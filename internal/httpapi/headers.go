// Package httpapi contains the public HTTP server of the API.
package httpapi

import "net/http"

// ContentSecurityPolicy is sent with every response. It allows no inline
// scripts or styles and no third-party origins.
const ContentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; " +
	"img-src 'self' blob: data:; connect-src 'self'; font-src 'self'; manifest-src 'self'; " +
	"base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// securityHeaders adds the security headers required by the project to
// every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", ContentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=(), interest-cohort=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		if r.TLS != nil {
			// Behind a TLS-terminating proxy, HSTS is set by the proxy
			// (documented in docs/deployment.md).
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}
