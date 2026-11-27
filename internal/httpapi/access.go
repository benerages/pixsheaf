package httpapi

import (
	"net/http"
	"net/url"
	"strings"
)

// Job access and CSRF protection (ADR 0019).

const cookieName = "pixsheaf_job"

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
	if err != nil || u.Host == "" {
		// Includes "null" (sandboxed frames, some redirects).
		return false
	}
	// The scheme is not compared: behind a TLS-terminating proxy the
	// request arrives over HTTP while the page origin is https.
	return strings.EqualFold(u.Host, r.Host)
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
// if allowCookie is set, from the job cookie.
func requestToken(r *http.Request, allowCookie bool) string {
	if h := r.Header.Get("Authorization"); h != "" {
		scheme, token, ok := strings.Cut(h, " ")
		if ok && strings.EqualFold(scheme, "Bearer") {
			return strings.TrimSpace(token)
		}
		return ""
	}
	if allowCookie {
		if c, err := r.Cookie(cookieName); err == nil {
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

// jobCookie scopes the token to the job's own paths.
func jobCookie(r *http.Request, jobID, token string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/api/v1/jobs/" + jobID,
		MaxAge:   maxAge,
		HttpOnly: true,
		// Behind a TLS-terminating proxy the browser's Origin shows https.
		Secure:   r.TLS != nil || strings.HasPrefix(r.Header.Get("Origin"), "https://"),
		SameSite: http.SameSiteStrictMode,
	}
}
