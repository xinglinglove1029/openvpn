package openvpnweb

import (
	"strings"
	"testing"
)

func TestSetDNSPushResolversReplacesLegacyAndDuplicateLines(t *testing.T) {
	cfg := &VPNConfig{Lines: []string{
		"port 1194",
		`# push "dhcp-option DNS 8.8.8.8"`,
		`push "dhcp-option DNS 9.9.9.9"`,
		`push "dhcp-option DNS 1.0.0.1"`,
		"keepalive 10 60",
	}}

	cfg.SetDNSPushResolvers("1.1.1.1", "2001:4860:4860::8888", "1.1.1.1", "invalid")
	joined := strings.Join(cfg.Lines, "\n")
	if strings.Contains(joined, "9.9.9.9") || strings.Contains(joined, "1.0.0.1") || strings.Contains(joined, "8.8.8.8") {
		t.Fatalf("legacy DNS push lines remain: %q", joined)
	}
	if got := strings.Count(joined, "dhcp-option DNS"); got != 2 {
		t.Fatalf("DNS push line count=%d, want 2: %q", got, joined)
	}
	if !strings.Contains(joined, `push "dhcp-option DNS 1.1.1.1"`) || !strings.Contains(joined, `push "dhcp-option DNS 2001:4860:4860::8888"`) {
		t.Fatalf("expected DNS push lines missing: %q", joined)
	}
	if !strings.Contains(joined, "port 1194") || !strings.Contains(joined, "keepalive 10 60") {
		t.Fatalf("unrelated server.conf lines were changed: %q", joined)
	}

	cfg.SetDNSPushResolvers()
	if strings.Contains(strings.Join(cfg.Lines, "\n"), "dhcp-option DNS") {
		t.Fatalf("DNS push lines remain after gateway disable: %q", cfg.Lines)
	}
}
