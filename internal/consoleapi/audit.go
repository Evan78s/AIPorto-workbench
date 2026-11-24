package consoleapi

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"aiporto/internal/util"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(p []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(p)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// AuditDetail 只记录请求的元数据。
// 安全红线：这里绝不序列化请求体、Authorization 头或任何 key 字段，
// 因为创建租户/轮换 key 的请求体里会出现一次性明文。
func AuditDetail(r *http.Request, status int, durationMS int64) map[string]any {
	keys := make([]string, 0, len(r.URL.Query()))
	for k := range r.URL.Query() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return map[string]any{
		"path":         r.URL.Path,
		"query_fields": keys,
		"status_code":  status,
		"duration_ms":  durationMS,
	}
}

// auditActor 只写主体标识，不写令牌本身。
func (a *API) auditActor(r *http.Request) (actor, tenantID string) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if strings.HasPrefix(token, "cw-") {
		var tid string
		err := a.pg.QueryRow(context.Background(),
			`SELECT tenant_id::text FROM console_tokens WHERE token_hash=$1`,
			util.HashKey(a.pepper, "console:"+token)).Scan(&tid)
		if err == nil {
			return "console:" + tid, tid
		}
	}
	if user, ok := a.verifyToken(token, "admin"); ok {
		return user, ""
	}
	return "anonymous", ""
}

// auditWrite 挂在所有管理/控制台路由的最外层，覆盖鉴权失败的写尝试。
// 所有非幂等读方法（POST/PUT/PATCH/DELETE）都会落一条审计记录。
func (a *API) auditWrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		started := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}

		actor, tenantID := a.auditActor(r)
		detail := AuditDetail(r, rec.status, time.Since(started).Milliseconds())
		raw, _ := json.Marshal(detail)
		// 用 Background：请求被客户端取消时仍必须落审计。
		_, _ = a.pg.Exec(context.Background(), `
			INSERT INTO audit_logs(request_id, actor, actor_ip, user_agent, action,
			                      target_type, target_id, tenant_id, detail, status_code)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			r.Header.Get("X-Request-Id"), actor, clientIP(r), r.UserAgent(),
			r.Method+" "+r.URL.Path, "http", "", nullable(tenantID), string(raw), rec.status)
	})
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
