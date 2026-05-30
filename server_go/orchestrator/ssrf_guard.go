package orchestrator

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"
)

// isBlockedIP reports whether ip is in a loopback, private, link-local,
// unique-local, unspecified, or multicast range — an address an outbound agent
// request must never reach (SSRF defense). Link-local (169.254.0.0/16) covers
// the cloud metadata endpoint 169.254.169.254, which on Cloud Run returns the
// service-account access token.
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

// ssrfDialControl is a net.Dialer.Control hook. It runs after DNS resolution
// with the concrete IP about to be dialed, so it is robust against DNS
// rebinding (a hostname that resolves to an internal IP is still caught).
func ssrfDialControl(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("ssrf guard: cannot parse dial address %q", address)
	}
	if isBlockedIP(ip) {
		return fmt.Errorf("ssrf guard: blocked outbound connection to internal address %s", ip)
	}
	return nil
}

// ssrfProtectionActive reports whether SSRF protection should be enforced.
// Enabled in production — Cloud Run sets K_SERVICE, and the app's environment
// variable ("n") is "production". Disabled in local/dev so agents pointed at
// localhost / private hosts keep working.
func ssrfProtectionActive() bool {
	if os.Getenv("K_SERVICE") != "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(os.Getenv("n")), "production")
}

// guardedHTTPTransport returns a RoundTripper that refuses connections to
// internal/reserved addresses when SSRF protection is active; otherwise it
// returns the default transport unchanged.
func guardedHTTPTransport() http.RoundTripper {
	if !ssrfProtectionActive() {
		return http.DefaultTransport
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return http.DefaultTransport
	}
	t := base.Clone()
	t.DialContext = (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   ssrfDialControl,
	}).DialContext
	return t
}
