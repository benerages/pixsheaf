package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func FuzzContentDisposition(f *testing.F) {
	for _, s := range []string{"a.png", "Ärger 100%.jpg", `a"b\c.gif`, "x\r\nSet-Cookie: a=b.png", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		h := contentDisposition(name)
		if strings.ContainsAny(h, "\r\n\x00") {
			t.Fatalf("header %q contains a line break or NUL", h)
		}
		ascii, ok := strings.CutPrefix(h, `attachment; filename="`)
		if !ok {
			t.Fatalf("header %q", h)
		}
		ascii, _, ok = strings.Cut(ascii, `"; filename*=UTF-8''`)
		if !ok {
			t.Fatalf("header %q has no RFC 8187 name", h)
		}
		for _, r := range ascii {
			if r < 0x20 || r >= 0x7f || r == '"' || r == '\\' {
				t.Fatalf("fallback name %q contains %U", ascii, r)
			}
		}
		// net/http must accept it as a header value.
		w := http.Header{}
		w.Set("Content-Disposition", h)
	})
}

func FuzzSameOrigin(f *testing.F) {
	f.Add("example.com", "https://example.com", "same-origin")
	f.Add("example.com", "https://evil.example", "")
	f.Add("example.com", "null", "")
	f.Add("example.com:8080", "http://EXAMPLE.com:8080", "none")
	f.Fuzz(func(t *testing.T, host, origin, site string) {
		r, err := http.NewRequest(http.MethodPost, "http://x/", nil)
		if err != nil {
			t.Skip()
		}
		r.Host = host
		r.Header.Set("Origin", origin)
		r.Header.Set("Sec-Fetch-Site", site)
		ok := sameOrigin(r)
		if ok && (site == "cross-site" || site == "same-site") {
			t.Fatalf("accepted Sec-Fetch-Site %q", site)
		}
		if ok && origin != "" && !strings.Contains(strings.ToLower(origin), strings.ToLower(host)) {
			t.Fatalf("accepted origin %q for host %q", origin, host)
		}
	})
}
