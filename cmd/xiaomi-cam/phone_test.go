package main

import (
	"net/http/httptest"
	"testing"
)

// The phone page answers only itself: its own address or the Tailscale name, same origin.
func TestPhoneAllowed(t *testing.T) {
	for _, c := range []struct {
		host, origin, site string
		ok                 bool
	}{
		{"127.0.0.1:11985", "", "none", true},                                       // the address bar
		{"127.0.0.1:11985", "http://127.0.0.1:11985", "same-origin", true},          // the page's own POST / WebSocket
		{"localhost:11985", "", "", true},                                           // curl
		{"pc.tail1234.ts.net", "https://pc.tail1234.ts.net", "same-origin", true},   // tailscale serve
		{"PC.tail1234.ts.net.", "https://pc.tail1234.ts.net.", "same-origin", true}, // case, trailing dot
		{"127.0.0.1:11985", "https://example.com", "cross-site", false},             // another site's fetch / WebSocket
		{"127.0.0.1:11985", "http://localhost:3000", "same-site", false},            // another local port
		{"127.0.0.1:11985", "", "cross-site", false},                                // <img>/<video> of another site
		{"127.0.0.1:11985", "null", "cross-site", false},                            // sandboxed frame
		{"evil.example:11985", "http://evil.example:11985", "same-origin", false},   // DNS rebinding
		{"pc.tail1234.ts.net", "https://other.tail1234.ts.net", "same-site", false},
	} {
		r := httptest.NewRequest("GET", "/api/ws?src=x", nil)
		r.Host = c.host
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if c.site != "" {
			r.Header.Set("Sec-Fetch-Site", c.site)
		}
		if got := phoneAllowed(r); got != c.ok {
			t.Errorf("host %q origin %q site %q: %v, want %v", c.host, c.origin, c.site, got, c.ok)
		}
	}
}
