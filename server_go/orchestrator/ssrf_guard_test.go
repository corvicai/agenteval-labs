package orchestrator

import (
	"net"
	"testing"
)

// SSRF defense: outbound agent requests must never be allowed to reach
// internal/reserved addresses (esp. the cloud metadata endpoint
// 169.254.169.254, which on Cloud Run yields the service-account token).
func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"169.254.169.254",        // cloud metadata (link-local)
		"127.0.0.1",              // loopback
		"10.1.2.3",               // RFC1918
		"172.16.5.4",             // RFC1918
		"192.168.1.1",            // RFC1918
		"0.0.0.0",                // unspecified
		"::1",                    // loopback (v6)
		"fc00::1",                // unique-local (v6)
		"fe80::1",                // link-local (v6)
		"::ffff:169.254.169.254", // v4-mapped metadata
	}
	for _, s := range blocked {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("bad test IP %q", s)
		}
		if !isBlockedIP(ip) {
			t.Errorf("expected %s to be BLOCKED", s)
		}
	}

	allowed := []string{
		"8.8.8.8",
		"1.1.1.1",
		"2606:4700:4700::1111", // public v6
	}
	for _, s := range allowed {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("bad test IP %q", s)
		}
		if isBlockedIP(ip) {
			t.Errorf("expected %s to be ALLOWED", s)
		}
	}
}

// ssrfDialControl runs at dial time with the resolved IP, so it is robust
// against DNS rebinding (a hostname that resolves to a private IP is caught).
func TestSSRFDialControl(t *testing.T) {
	if err := ssrfDialControl("tcp", "169.254.169.254:80", nil); err == nil {
		t.Error("expected dial to metadata IP to be blocked")
	}
	if err := ssrfDialControl("tcp", "10.0.0.5:8080", nil); err == nil {
		t.Error("expected dial to RFC1918 IP to be blocked")
	}
	if err := ssrfDialControl("tcp", "8.8.8.8:443", nil); err != nil {
		t.Errorf("expected dial to public IP to be allowed, got %v", err)
	}
}

func TestSSRFProtectionActiveUsesAppEnv(t *testing.T) {
	t.Setenv("K_SERVICE", "")
	t.Setenv("APP_ENV", "production")
	t.Setenv("n", "")

	if !ssrfProtectionActive() {
		t.Fatal("expected SSRF protection to be active when APP_ENV=production")
	}
}
