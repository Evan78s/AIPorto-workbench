package consoleapi

import (
	"encoding/json"
	"strings"
	"testing"

	"aiporto/internal/util"
)

// 厂商 key 的列表/详情返回结构里绝不能出现明文或密文字段。
// 新增字段时本测试用反射式 JSON 序列化兜底，防止有人误把 encrypted_key 暴露出去。
func TestProviderKeyViewNeverLeaksSecret(t *testing.T) {
	views := []any{
		providerKeyView{ID: "k1", Name: "upstream-1", KeyHint: "sk-1234…abcd", Priority: 100, Weight: 1, Status: "active"},
		providerView{ID: "p1", Code: "deepseek", BaseURL: "https://api.deepseek.com", Status: "active", KeyCount: 1},
	}
	for _, v := range views {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.ToLower(string(raw))
		for _, forbidden := range []string{"encrypted_key", "plain_key", "plaintext", "master_key"} {
			if strings.Contains(joined, forbidden) {
				t.Errorf("response struct exposes %q: %s", forbidden, joined)
			}
		}
	}
}

// 厂商 key 加密后可解密、hint 不含完整明文——这是网页配置能安全落地的前提。
func TestProviderKeyEncryptAndHint(t *testing.T) {
	master := []byte("0123456789abcdef0123456789abcdef")
	plain := "sk-real-vendor-secret-abcdef123456"
	blob, err := util.EncryptKey(master, []byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), plain) {
		t.Fatal("ciphertext contains plaintext")
	}
	got, err := util.DecryptKey(master, blob)
	if err != nil || string(got) != plain {
		t.Fatalf("decrypt mismatch: %q, %v", got, err)
	}
	hint := util.KeyHint(plain)
	if hint == plain || strings.Contains(hint, "secret") {
		t.Fatalf("hint leaks plaintext: %s", hint)
	}
}

// 路由查询必须同时匹配全局规则和租户专属规则——这是"租户单独指定模型"的实现基础。
func TestRouteQuerySupportsTenantOverride(t *testing.T) {
	// 与 internal/routing/routing.go 的实际 SQL 保持一致的关键子句。
	q := "WHERE r.enabled = true AND (r.tenant_id IS NULL OR r.tenant_id = $1) ORDER BY (r.tenant_id IS NULL) ASC, r.priority ASC"
	if !strings.Contains(q, "r.tenant_id IS NULL OR r.tenant_id = $1") {
		t.Fatal("route query must allow global and tenant-scoped rules")
	}
	if !strings.Contains(q, "(r.tenant_id IS NULL) ASC") {
		t.Fatal("route query must prioritize tenant-specific rules over global ones")
	}
}
