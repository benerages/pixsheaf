package httpapi

import (
	"net/http"
	"net/url"
	"strings"
)

// Job access and CSRF protection (ADR 0019).

// cookiePrefix starts the name of a job cookie; the job ID completes it.
// "__Host-" makes browsers require Secure and Path=/ and reject the cookie
// from other hosts (ASVS 5.0 3.3.1, 3.3.3); the ID in the name keeps the
// cookies of parallel jobs apart.
const cookiePrefix = "__Host-pixsheaf-"

// sameOrigin rejects state-changing requests from other sites: by
// Sec-Fetch-Site where the browser sends it, and by Origin otherwise.
// Requests without either header (API clients, curl) pass; they cannot
// carry the job token without the client knowing it.
func sameOrigin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "cross-site", "same-site":
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		// Includes "null" (sandboxed frames, some redirects).
		return false
	}
	// The scheme is not compared: behind a TLS-terminating proxy the
	// request arrives over HTTP while the page origin is https. Host names
	// are compared case-insensitively in ASCII only; strings.EqualFold
	// would treat any two invalid UTF-8 bytes as equal.
	return lowerASCII(u.Host) == lowerASCII(r.Host)
}

func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// csrf wraps state-changing handlers with the same-origin check.
func csrf(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			writeProblem(w, http.StatusForbidden, codeCrossOrigin, "cross-origin requests are not allowed")
			return
		}
		next(w, r)
	}
}

// requestToken returns the job token from the Authorization header, or,
// if allowCookie is set, from the cookie of job jobID.
func requestToken(r *http.Request, jobID string, allowCookie bool) string {
	if h := r.Header.Get("Authorization"); h != "" {
		scheme, token, ok := strings.Cut(h, " ")
		if ok && strings.EqualFold(scheme, "Bearer") {
			return strings.TrimSpace(token)
		}
		return ""
	}
	if allowCookie {
		if c, err := r.Cookie(cookiePrefix + jobID); err == nil {
			return c.Value
		}
	}
	return ""
}

// validID reports whether s looks like a job or file ID (22 base64url
// characters), so that other strings never reach the job store.
func validID(s string) bool {
	if len(s) != 22 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// jobCookie carries the token of one job for EventSource and download
// links.
func jobCookie(jobID, token string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     cookiePrefix + jobID,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		// Always Secure: browsers also accept Secure cookies from
		// http://localhost and 127.0.0.1, and everything else must be
		// served over TLS (docs/deployment.md).
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	}
}
