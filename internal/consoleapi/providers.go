package consoleapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"aiporto/internal/util"
	"github.com/go-chi/chi/v5"
)

// ============================================================================
// 厂商管理 / 模型路由管理
//
// 安全红线（与 seed 命令一致）：
//   1. 厂商 key 明文只在 createProviderKey 的请求体里出现一次，立刻 AES-256-GCM
//      加密入库；任何列表/详情接口都只返回 key_hint，绝不返回明文或密文。
//   2. auditWrite 中间件只记录 path/query/status，不记录 body，明文不会进审计表。
//   3. 删除一律软删除（status=archived / enabled=false），保留历史关联与审计链。
//   4. 所有 SQL 均参数化，租户级路由的 tenant_id 来自服务端校验，不接受越权值。
// ============================================================================

type providerView struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	BaseURL  string `json:"base_url"`
	Protocol string `json:"protocol"`
	Status   string `json:"status"`
	KeyCount int    `json:"key_count"`
}

// listProviders 列出所有厂商（含每个厂商的活跃 key 数量，不含 key 内容）。
func (a *API) listProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := a.pg.Query(r.Context(), `
		SELECT p.id::text, p.code, p.base_url, p.protocol, p.status,
		       (SELECT COUNT(*)::int FROM upstream_keys k
		        WHERE k.provider_id = p.id AND k.status = 'active')
		FROM upstream_providers p
		ORDER BY p.code`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []providerView{}
	for rows.Next() {
		var p providerView
		if err := rows.Scan(&p.ID, &p.Code, &p.BaseURL, &p.Protocol, &p.Status, &p.KeyCount); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		out = append(out, p)
	}
	writeJSON(w, 200, out)
}

// createProvider 新增厂商档案（code 唯一，重复则 409）。
func (a *API) createProvider(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code     string `json:"code"`
		BaseURL  string `json:"base_url"`
		Protocol string `json:"protocol"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}
	req.Code = strings.ToLower(strings.TrimSpace(req.Code))
	req.BaseURL = strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if req.Code == "" || req.BaseURL == "" {
		writeJSON(w, 400, map[string]string{"error": "code and base_url required"})
		return
	}
	if req.Protocol == "" {
		req.Protocol = "openai_compatible"
	}
	id := util.NewID()
	_, err := a.pg.Exec(r.Context(), `
		INSERT INTO upstream_providers(id, code, base_url, protocol, status)
		VALUES($1,$2,$3,$4,'active')`, id, req.Code, req.BaseURL, req.Protocol)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeJSON(w, 409, map[string]string{"error": "provider code already exists"})
			return
		}
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "status": "created"})
}

// updateProvider 修改厂商档案（base_url / protocol / status）。
func (a *API) updateProvider(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		BaseURL  string `json:"base_url"`
		Protocol string `json:"protocol"`
		Status   string `json:"status"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}
	req.BaseURL = strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if req.BaseURL == "" {
		writeJSON(w, 400, map[string]string{"error": "base_url required"})
		return
	}
	if req.Protocol == "" {
		req.Protocol = "openai_compatible"
	}
	if req.Status == "" {
		req.Status = "active"
	}
	tag, err := a.pg.Exec(r.Context(), `
		UPDATE upstream_providers SET base_url=$1, protocol=$2, status=$3 WHERE id=$4`,
		req.BaseURL, req.Protocol, req.Status, id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, 404, map[string]string{"error": "provider not found"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

// deleteProvider 软删除厂商：归档厂商本身，并停用其所有 key 与路由，避免流量打到已删厂商。
func (a *API) deleteProvider(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tx, err := a.pg.Begin(r.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), `UPDATE upstream_providers SET status='archived' WHERE id=$1`, id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE upstream_keys SET status='archived' WHERE provider_id=$1`, id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE routing_rules SET enabled=false WHERE provider_id=$1`, id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "archived"})
}

// fetchProviderModels 从厂商的 OpenAI 兼容 /models 端点拉取模型 ID，
// 供后台「模型路由」下拉选择，避免手抄模型名出错。
// 只用该厂商的一把 active key，且密钥不出本机：解密后直接放到请求头。
func (a *API) fetchProviderModels(w http.ResponseWriter, r *http.Request) {
	pid := chi.URLParam(r, "id")
	var baseURL, protocol, encKey string
	err := a.pg.QueryRow(r.Context(), `
		SELECT p.base_url, p.protocol,
		       COALESCE((SELECT k.encrypted_key::text FROM upstream_keys k
		                 WHERE k.provider_id=p.id AND k.status='active'
		                 ORDER BY k.priority, k.id LIMIT 1), '')
		FROM upstream_providers p WHERE p.id=$1`, pid).Scan(&baseURL, &protocol, &encKey)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "provider not found"})
		return
	}
	if encKey == "" {
		writeJSON(w, 400, map[string]string{"error": "该厂商还没有可用的密钥，请先在「密钥」里添加"})
		return
	}
	plain, err := util.DecryptKey(a.masterKey, []byte(encKey))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "key decrypt failed"})
		return
	}
	defer clear(plain)

	// 无 /v1 协议（火山方舟）用 baseURL+"/models"；普通协议用 baseURL+"/v1/models"。
	path := "/v1/models"
	if protocol == "openai_compatible_no_v1" {
		path = "/models"
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
		strings.TrimRight(baseURL, "/")+path, nil)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid base_url"})
		return
	}
	req.Header.Set("Authorization", "Bearer "+string(plain))
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "上游请求失败: " + err.Error()})
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "读取上游响应失败"})
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeJSON(w, 502, map[string]string{"error": "上游返回 " + resp.Status + "：" + string(body[:min(len(body), 300)])})
		return
	}
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		writeJSON(w, 502, map[string]string{"error": "上游返回的 JSON 无法解析"})
		return
	}
	models := []string{}
	seen := map[string]bool{}
	for _, d := range parsed.Data {
		if d.ID != "" && !seen[d.ID] {
			seen[d.ID] = true
			models = append(models, d.ID)
		}
	}
	if len(models) == 0 {
		writeJSON(w, 502, map[string]string{"error": "上游没有返回任何模型，可能该厂商不支持 /models 接口"})
		return
	}
	writeJSON(w, 200, map[string]any{"models": models})
}

type providerKeyView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	KeyHint  string `json:"key_hint"`
	Priority int    `json:"priority"`
	Weight   int    `json:"weight"`
	Status   string `json:"status"`
	TenantID string `json:"tenant_id"`
}

// listProviderKeys 列出某厂商的 key——只返回 hint，绝不返回明文或密文。
func (a *API) listProviderKeys(w http.ResponseWriter, r *http.Request) {
	pid := chi.URLParam(r, "id")
	rows, err := a.pg.Query(r.Context(), `
		SELECT id::text, name, key_hint, priority, weight, status, COALESCE(tenant_id::text,'')
		FROM upstream_keys WHERE provider_id=$1 ORDER BY priority, id`, pid)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []providerKeyView{}
	for rows.Next() {
		var k providerKeyView
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyHint, &k.Priority, &k.Weight, &k.Status, &k.TenantID); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		out = append(out, k)
	}
	writeJSON(w, 200, out)
}

// createProviderKey 新增厂商 key：明文只从请求体读取一次，加密后入库；响应不含明文。
func (a *API) createProviderKey(w http.ResponseWriter, r *http.Request) {
	pid := chi.URLParam(r, "id")
	var req struct {
		Plain    string `json:"plain_key"`
		Name     string `json:"name"`
		TenantID string `json:"tenant_id"`
		Priority int    `json:"priority"`
		Weight   int    `json:"weight"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}
	req.Plain = strings.TrimSpace(req.Plain)
	if req.Plain == "" {
		writeJSON(w, 400, map[string]string{"error": "plain_key required"})
		return
	}
	// 校验厂商存在且未归档
	var status string
	if err := a.pg.QueryRow(r.Context(), `SELECT status FROM upstream_providers WHERE id=$1`, pid).Scan(&status); err != nil || status != "active" {
		writeJSON(w, 400, map[string]string{"error": "provider not found or archived"})
		return
	}
	// 租户归属：填了必须存在；留空则全局共享
	var tenantArg any
	if strings.TrimSpace(req.TenantID) != "" {
		var exists string
		if err := a.pg.QueryRow(r.Context(), `SELECT id::text FROM tenants WHERE id=$1`, req.TenantID).Scan(&exists); err != nil {
			writeJSON(w, 400, map[string]string{"error": "tenant_id not found"})
			return
		}
		tenantArg = req.TenantID
	}
	if req.Priority == 0 {
		req.Priority = 100
	}
	if req.Weight <= 0 {
		req.Weight = 1
	}
	if req.Name == "" {
		req.Name = "upstream-key"
	}
	enc, err := util.EncryptKey(a.masterKey, []byte(req.Plain))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "encrypt failed"})
		return
	}
	id := util.NewID()
	_, err = a.pg.Exec(r.Context(), `
		INSERT INTO upstream_keys(id, provider_id, tenant_id, name, encrypted_key, key_hint, priority, weight, status)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,'active')`,
		id, pid, tenantArg, req.Name, enc, util.KeyHint(req.Plain), req.Priority, req.Weight)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	// 绝不回显明文，只回 hint
	writeJSON(w, 201, map[string]string{"id": id, "key_hint": util.KeyHint(req.Plain), "status": "created"})
}

// deleteProviderKey 软删除厂商 key。
func (a *API) deleteProviderKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tag, err := a.pg.Exec(r.Context(), `UPDATE upstream_keys SET status='archived' WHERE id=$1`, id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, 404, map[string]string{"error": "key not found"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "archived"})
}

type routeView struct {
	ID         string `json:"id"`
	TenantID   string `json:"tenant_id"`
	ModelAlias string `json:"model_alias"`
	ProviderID string `json:"provider_id"`
	Provider   string `json:"provider_code"`
	Model      string `json:"model"`
	Priority   int    `json:"priority"`
	Enabled    bool   `json:"enabled"`
}

// listRoutes 列出所有模型路由，带厂商代号便于阅读。
func (a *API) listRoutes(w http.ResponseWriter, r *http.Request) {
	rows, err := a.pg.Query(r.Context(), `
		SELECT rr.id::text, COALESCE(rr.tenant_id::text,''), rr.model_alias,
		       rr.provider_id::text, p.code, rr.model, rr.priority, rr.enabled
		FROM routing_rules rr JOIN upstream_providers p ON p.id = rr.provider_id
		ORDER BY rr.model_alias, (rr.tenant_id IS NULL) DESC, rr.priority`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []routeView{}
	for rows.Next() {
		var v routeView
		if err := rows.Scan(&v.ID, &v.TenantID, &v.ModelAlias, &v.ProviderID, &v.Provider, &v.Model, &v.Priority, &v.Enabled); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		out = append(out, v)
	}
	writeJSON(w, 200, out)
}

type routeReq struct {
	TenantID   string `json:"tenant_id"`
	ModelAlias string `json:"model_alias"`
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
	Priority   int    `json:"priority"`
}

// validateRouteReq 校验路由请求：别名/模型/厂商必填，tenant_id 留空=全局，填了必须存在。
// 返回 (tenantArg, errorMsg)；errorMsg 非空表示校验失败。
func (a *API) validateRouteReq(r *http.Request, req *routeReq) (any, string) {
	req.ModelAlias = strings.TrimSpace(req.ModelAlias)
	req.Model = strings.TrimSpace(req.Model)
	req.ProviderID = strings.TrimSpace(req.ProviderID)
	if req.ModelAlias == "" || req.Model == "" || req.ProviderID == "" {
		return nil, "model_alias, provider_id and model required"
	}
	var pstatus string
	if err := a.pg.QueryRow(r.Context(), `SELECT status FROM upstream_providers WHERE id=$1`, req.ProviderID).Scan(&pstatus); err != nil || pstatus != "active" {
		return nil, "provider not found or archived"
	}
	if req.Priority == 0 {
		req.Priority = 100
	}
	if strings.TrimSpace(req.TenantID) == "" {
		return nil, "" // 全局路由：tenant_id = NULL
	}
	var exists string
	if err := a.pg.QueryRow(r.Context(), `SELECT id::text FROM tenants WHERE id=$1`, req.TenantID).Scan(&exists); err != nil {
		return nil, "tenant_id not found"
	}
	return req.TenantID, ""
}

// createRoute 新增模型路由。同一 (tenant_id, model_alias, provider_id) 已存在则 409。
func (a *API) createRoute(w http.ResponseWriter, r *http.Request) {
	var req routeReq
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}
	tenantArg, errMsg := a.validateRouteReq(r, &req)
	if errMsg != "" {
		writeJSON(w, 400, map[string]string{"error": errMsg})
		return
	}
	id := util.NewID()
	_, err := a.pg.Exec(r.Context(), `
		INSERT INTO routing_rules(id, tenant_id, model_alias, provider_id, model, priority, enabled)
		VALUES($1,$2,$3,$4,$5,$6,true)`, id, tenantArg, req.ModelAlias, req.ProviderID, req.Model, req.Priority)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeJSON(w, 409, map[string]string{"error": "route already exists"})
			return
		}
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "status": "created"})
}

// updateRoute 修改路由的目标模型/厂商/优先级/启停。
func (a *API) updateRoute(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		ProviderID string `json:"provider_id"`
		Model      string `json:"model"`
		Priority   int    `json:"priority"`
		Enabled    *bool  `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}
	req.Model = strings.TrimSpace(req.Model)
	req.ProviderID = strings.TrimSpace(req.ProviderID)
	if req.Model == "" || req.ProviderID == "" {
		writeJSON(w, 400, map[string]string{"error": "provider_id and model required"})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.Priority == 0 {
		req.Priority = 100
	}
	tag, err := a.pg.Exec(r.Context(), `
		UPDATE routing_rules SET provider_id=$1, model=$2, priority=$3, enabled=$4 WHERE id=$5`,
		req.ProviderID, req.Model, req.Priority, enabled, id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, 404, map[string]string{"error": "route not found"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

// deleteRoute 硬删除路由（路由本身无敏感数据，且需即时从路由表消失）。
func (a *API) deleteRoute(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tag, err := a.pg.Exec(r.Context(), `DELETE FROM routing_rules WHERE id=$1`, id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, 404, map[string]string{"error": "route not found"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}
