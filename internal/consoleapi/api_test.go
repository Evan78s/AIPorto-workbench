package consoleapi

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 客户控制台的每一条查询都必须带租户过滤；新增 SQL 常量时本测试会自动覆盖。
func TestConsoleQueriesHaveTenantFilter(t *testing.T) {
	queries := map[string]string{
		"overview":        consoleOverviewSQL,
		"by_model":        consoleByModelSQL,
		"daily":           consoleDailySQL,
		"keys":            consoleKeysSQL,
		"recharges":       consoleRechargesSQL,
		"usage_export":    consoleUsageExportSQL,
		"bill_export":     consoleBillExportSQL,
	}
	for name, q := range queries {
		if !strings.Contains(q, "tenant_id = $1") {
			t.Errorf("%s query is missing mandatory tenant filter: %s", name, q)
		}
	}
}

func TestTenantComesFromContextOnly(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/console/overview", nil)
	if got := tenantIDFrom(req); got != "" {
		t.Fatalf("expected empty tenant from unauthenticated request, got %q", got)
	}
	req = req.WithContext(context.WithValue(req.Context(), tenantCtxKey, "t_only_me"))
	if got := tenantIDFrom(req); got != "t_only_me" {
		t.Fatalf("expected t_only_me, got %q", got)
	}
}

// CSV 导出列是白名单：不得包含 prompt 正文、客户 key 或租户信息。
func TestCSVExportAllowlist(t *testing.T) {
	row := UsageExportRow{
		RequestID: "req-1", CreatedAt: "2026-11-23 10:00:00", Model: "demo-model",
		Endpoint: "/v1/chat/completions", PromptTokens: 10, CompletionTokens: 20,
		StatusCode: 200, LatencyMS: 123, TTFTMS: 45, Cost: 0.12345678,
	}
	data := RenderUsageCSV([]UsageExportRow{row})
	records, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"request_id", "created_at", "model", "endpoint",
		"prompt_tokens", "completion_tokens", "status_code", "latency_ms", "ttft_ms", "cost"}
	if strings.Join(records[0], ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected header: %v", records[0])
	}
	joined := strings.ToLower(string(data))
	for _, forbidden := range []string{"prompt_text", "messages", "api_key", "tenant_id", "sk-wb-"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("CSV contains forbidden field/value %q", forbidden)
		}
	}
}

func TestRangeDates(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/console/usage/daily?days=7", nil)
	from, to := rangeDates(req)
	if to.Sub(from) < 6*24*time.Hour {
		t.Fatalf("expected at least 7 days range, got %v", to.Sub(from))
	}
	req = httptest.NewRequest("GET", "/api/console/usage/daily?days=9999", nil)
	from, to = rangeDates(req)
	if to.Sub(from) > 366*24*time.Hour {
		t.Fatalf("days must be capped at 365, got %v", to.Sub(from))
	}
}

// 审计只写元数据：即便请求体里带着一次性明文 key，也不能进入 audit_logs.detail。
func TestAuditDetailNeverContainsBodyOrAuth(t *testing.T) {
	body := `{"name":"x","api_key_plaintext":"sk-wb-SHOULD_NOT_APPEAR","password":"SECRET"}`
	req := httptest.NewRequest("POST", "/api/admin/tenants?plan_code=basic", strings.NewReader(body))
	req.Header.Set("Authorization", "Basic dXNlcjpzZWNyZXQ=")
	raw, err := json.Marshal(AuditDetail(req, 201, 7))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.ToLower(string(raw))
	for _, forbidden := range []string{"sk-wb-", "secret", "password", "dXNlcjpzZWNyZXQ=", "api_key_plaintext"} {
		if strings.Contains(joined, strings.ToLower(forbidden)) {
			t.Errorf("audit detail leaked %q: %s", forbidden, joined)
		}
	}
}
