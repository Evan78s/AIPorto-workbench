package proxy

import "testing"

func TestUpstreamPath(t *testing.T) {
	cases := []struct {
		name     string
		protocol string
		incoming string
		want     string
	}{
		{"default keeps v1", "openai_compatible", "/v1/chat/completions", "/v1/chat/completions"},
		{"empty keeps v1", "", "/v1/chat/completions", "/v1/chat/completions"},
		{"ark strips v1", "openai_compatible_no_v1", "/v1/chat/completions", "/chat/completions"},
		{"ark other endpoint", "openai_compatible_no_v1", "/v1/models", "/models"},
		{"ark idempotent", "openai_compatible_no_v1", "/chat/completions", "/chat/completions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := upstreamPath(tc.protocol, tc.incoming); got != tc.want {
				t.Fatalf("upstreamPath(%q, %q) = %q, want %q", tc.protocol, tc.incoming, got, tc.want)
			}
		})
	}
}
