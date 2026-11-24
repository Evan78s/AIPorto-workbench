package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"aiporto/internal/auth"
	"aiporto/internal/metering"
	"aiporto/internal/metrics"
	"aiporto/internal/quota"
	"aiporto/internal/routing"
	"aiporto/internal/util"
)

var usageRe = regexp.MustCompile(`"usage"\s*:\s*(\{[^}]*\})`)

type Handler struct {
	authn    *auth.Authenticator
	limiter  *quota.Limiter
	router   *routing.Router
	reporter *metering.Reporter
	client   *http.Client
}

func New(a *auth.Authenticator, l *quota.Limiter, r *routing.Router, rep *metering.Reporter, c *http.Client) *Handler {
	return &Handler{authn: a, limiter: l, router: r, reporter: rep, client: c}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	metrics.Requests.Add(1)

	identity, err := h.authn.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read body")
		return
	}

	model, stream, inputText, err := parseRequest(body)
	if err != nil || model == "" {
		writeError(w, http.StatusBadRequest, "invalid body or missing model")
		return
	}
	inputTokens := util.EstimateTokens(inputText)

	candidates, err := h.router.Resolve(r.Context(), identity.TenantID, model)
	if err != nil {
		writeError(w, http.StatusNotFound, "no route for model")
		return
	}

	requestID := util.NewID()
	estimate := inputTokens + outputPreEstimate(body)
	decision, err := h.limiter.Acquire(r.Context(), identity, requestID, estimate)
	if err != nil {
		metrics.QuotaErrors.Add(1)
		writeError(w, http.StatusServiceUnavailable, "quota service unavailable")
		return
	}
	if !decision.Allowed {
		metrics.Rejected429.Add(1)
		w.Header().Set("Retry-After", fmt.Sprintf("%d", decision.RetryAfter))
		writeError(w, http.StatusTooManyRequests, "quota exceeded: "+decision.Reason)
		return
	}

	var promptTokens, completionTokens, statusCode, ttftMS int
	used := candidates[0]

	// 无论成功、失败还是客户端断连，都释放并发占位并把预扣校正为真实用量。
	defer func() {
		metrics.Observe(time.Since(started))
		bg := context.Background()
		h.limiter.Settle(bg, identity, requestID, estimate, promptTokens+completionTokens)
		h.reporter.Enqueue(metering.Record{
			RequestID:        requestID,
			TenantID:         identity.TenantID,
			KeyID:            identity.KeyID,
			ProviderID:       used.Provider.ID,
			UpstreamKeyID:    used.KeyID,
			ModelAlias:       model,
			ModelResolved:    used.Model,
			Endpoint:         r.URL.Path,
			Stream:           stream,
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			StatusCode:       statusCode,
			LatencyMS:        int(time.Since(started).Milliseconds()),
			TTFTMS:           ttftMS,
			CreatedAt:        time.Now().UTC(),
		})
	}()

	upstreamBody := rewriteModel(body, candidates[0].Model)
	committed := false
	attempts := len(candidates)
	if attempts > 2 {
		attempts = 2 // 最多失败重试一次
	}

	for i := 0; i < attempts; i++ {
		c := candidates[i]
		used = c

		upReq, err := http.NewRequestWithContext(r.Context(), r.Method, c.Provider.BaseURL+r.URL.Path, bytes.NewReader(upstreamBody))
		if err != nil {
			clear(c.PlainKey)
			statusCode = 502
			continue
		}
		copyRequestHeaders(upReq.Header, r.Header)
		upReq.Header.Set("Authorization", "Bearer "+string(c.PlainKey))
		upReq.Header.Set("X-Request-Id", requestID)
		upReq.Host = ""

		resp, err := h.client.Do(upReq)
		if err != nil {
			metrics.UpstreamFailures.Add(1)
			clear(c.PlainKey)
			statusCode = 502
			continue
		}

		// 只有尚未向客户端写出任何内容时才允许重试；SSE 一旦开始输出就不可重试。
		if resp.StatusCode >= 500 && !committed && i+1 < attempts {
			metrics.UpstreamFailures.Add(1)
			resp.Body.Close()
			clear(c.PlainKey)
			statusCode = resp.StatusCode
			continue
		}

		copyResponseHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		statusCode = resp.StatusCode
		committed = true

		captured := newCapture(256 << 10)
		tee := io.TeeReader(resp.Body, captured)
		fw := &flushWriter{w: w, rc: http.NewResponseController(w)}
		buf := make([]byte, 16<<10)
		for {
			n, readErr := tee.Read(buf)
			if n > 0 {
				if ttftMS == 0 {
					ttftMS = int(time.Since(started).Milliseconds())
				}
				if _, writeErr := fw.Write(buf[:n]); writeErr != nil {
					break
				}
			}
			if readErr != nil {
				break
			}
		}
		resp.Body.Close()
		clear(c.PlainKey)

		promptTokens, completionTokens = tokensFromCapture(captured.Bytes(), inputTokens, stream)
		return
	}

	if !committed {
		if statusCode == 0 {
			statusCode = 502
		}
		writeError(w, statusCode, "upstream unavailable")
	}
}

func parseRequest(body []byte) (model string, stream bool, inputText string, err error) {
	var obj map[string]json.RawMessage
	if err = json.Unmarshal(body, &obj); err != nil {
		return
	}
	if v, ok := obj["model"]; ok {
		_ = json.Unmarshal(v, &model)
	}
	if v, ok := obj["stream"]; ok {
		_ = json.Unmarshal(v, &stream)
	}
	inputText = string(obj["messages"])
	return
}

func outputPreEstimate(body []byte) int {
	var obj struct {
		MaxTokens int `json:"max_tokens"`
	}
	_ = json.Unmarshal(body, &obj)
	out := 512
	if obj.MaxTokens > 0 {
		out = obj.MaxTokens
	}
	if out > 4096 {
		out = 4096
	}
	return out
}

func rewriteModel(body []byte, resolved string) []byte {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	b, err := json.Marshal(resolved)
	if err != nil {
		return body
	}
	obj["model"] = b
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return out
}

func copyRequestHeaders(dst, src http.Header) {
	skip := map[string]bool{"authorization": true, "content-length": true, "transfer-encoding": true, "connection": true, "host": true}
	for k, values := range src {
		if skip[strings.ToLower(k)] {
			continue
		}
		for _, v := range values {
			dst.Add(k, v)
		}
	}
}

func copyResponseHeaders(dst, src http.Header) {
	skip := map[string]bool{"content-length": true, "transfer-encoding": true, "connection": true, "content-encoding": true}
	for k, values := range src {
		if skip[strings.ToLower(k)] {
			continue
		}
		for _, v := range values {
			dst.Add(k, v)
		}
	}
}

type flushWriter struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func (f *flushWriter) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	_ = f.rc.Flush()
	return n, err
}

type capture struct {
	buf []byte
	max int
}

func newCapture(max int) *capture { return &capture{max: max} }

func (c *capture) Write(p []byte) (int, error) {
	c.buf = append(c.buf, p...)
	if len(c.buf) > c.max {
		c.buf = append([]byte(nil), c.buf[len(c.buf)-c.max:]...)
	}
	return len(p), nil
}

func (c *capture) Bytes() []byte { return c.buf }

func tokensFromCapture(raw []byte, inputTokens int, stream bool) (int, int) {
	s := string(raw)
	if m := usageRe.FindStringSubmatch(s); len(m) > 1 {
		var u struct {
			Prompt     int `json:"prompt_tokens"`
			Completion int `json:"completion_tokens"`
		}
		if json.Unmarshal([]byte(m[1]), &u) == nil && u.Prompt+u.Completion > 0 {
			return u.Prompt, u.Completion
		}
	}
	return inputTokens, util.EstimateTokens(extractContent(s, stream))
}

func extractContent(raw string, stream bool) string {
	if !stream {
		var obj struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(raw), &obj) == nil && len(obj.Choices) > 0 {
			return obj.Choices[0].Message.Content
		}
		return ""
	}

	var sb strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" || payload == "" {
			continue
		}
		var obj struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(payload), &obj) == nil && len(obj.Choices) > 0 {
			sb.WriteString(obj.Choices[0].Delta.Content)
		}
	}
	return sb.String()
}

func writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
