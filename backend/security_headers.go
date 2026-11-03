package main

import (
	"net/http"
	"net/url"
	"strings"
)

// securityHeaders wraps h, setting baseline hardening headers on every
// response. csp is left empty for handlers that can't function under a
// same-origin-only policy (currently only the opt-in API docs page, which
// loads a third-party script).
func securityHeaders(csp string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if csp != "" {
			w.Header().Set("Content-Security-Policy", csp)
		}
		h(w, r)
	}
}

// buildContentSecurityPolicy locks the SPA and JSON API responses down to
// same-origin resources, plus the IRMA server that the frontend polls
// directly (via fetch/EventSource) while a disclosure session is running.
func buildContentSecurityPolicy(irmaServerURL string) string {
	connectSrc := "'self'"
	if u, err := url.Parse(irmaServerURL); err == nil && u.Scheme != "" && u.Host != "" {
		connectSrc += " " + u.Scheme + "://" + u.Host
	}
	directives := []string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self'",
		"img-src 'self' data:",
		"font-src 'self'",
		"connect-src " + connectSrc,
		"frame-ancestors 'self'",
		"base-uri 'self'",
		"form-action 'self'",
		"object-src 'none'",
	}
	return strings.Join(directives, "; ")
}
