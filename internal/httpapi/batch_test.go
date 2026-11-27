package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/benerages/pixsheaf/internal/api"
	"github.com/benerages/pixsheaf/internal/jobproto"
)

func TestBatchFlow(t *testing.T) {
	e := newAPI(t, Options{})
	body := `{"pipeline":` + emptyPipeline + `,"expectedFiles":3,"naming":{"pattern":"{index}-{name}","keepFolders":true}}`
	resp := e.do(t, "POST", "/api/v1/jobs", "", "application/json", strings.NewReader(body))
	expect(t, resp, http.StatusCreated, "")
	var jc api.JobCreated
	_ = json.NewDecoder(resp.Body).Decode(&jc)

	for _, name := range []string{"Holiday/Beach.png", "Holiday/Beach.png", "broken.png"} {
		expect(t, e.upload(t, jc.Job.Id, jc.Token, name, pngHeader), http.StatusCreated, "")
	}
	expect(t, e.upload(t, jc.Job.Id, jc.Token, "extra.png", pngHeader), http.StatusConflict, codeJobNotOpen)
	e.work(t, jobproto.Response{Output: &jobproto.ImageInfo{Format: "webp", Width: 2, Height: 2, Pages: 1}}, "first")
	e.work(t, jobproto.Response{Output: &jobproto.ImageInfo{Format: "webp", Width: 2, Height: 2, Pages: 1}}, "second")
	e.work(t, jobproto.Response{Error: &jobproto.Error{Code: "decode_failed", Message: "the image could not be decoded"}}, "")

	resp = e.do(t, "GET", "/api/v1/jobs/"+jc.Job.Id, jc.Token, "", nil)
	var job api.Job
	_ = json.NewDecoder(resp.Body).Decode(&job)
	if job.Status != "done" || *job.Files[0].Source != "Holiday/Beach.png" || job.Files[0].Kind != api.Image {
		t.Fatalf("job %+v", job)
	}

	resp = e.do(t, "GET", "/api/v1/jobs/"+jc.Job.Id+"/download", jc.Token, "", nil)
	expect(t, resp, http.StatusOK, "")
	if resp.Header.Get("Content-Type") != "application/zip" ||
		!strings.HasPrefix(resp.Header.Get("Content-Disposition"), `attachment; filename="pixsheaf-`) {
		t.Fatalf("headers %v", resp.Header)
	}
	data, _ := io.ReadAll(resp.Body)
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	var report api.Report
	for _, f := range zr.File {
		names = append(names, f.Name)
		rc, _ := f.Open()
		content, _ := io.ReadAll(rc)
		_ = rc.Close()
		switch f.Name {
		case "report.json":
			if err := json.Unmarshal(content, &report); err != nil {
				t.Fatal(err)
			}
		case "Holiday/1-Beach.webp":
			if string(content) != "first" || f.Method != zip.Store {
				t.Errorf("first entry %q, method %d", content, f.Method)
			}
		}
	}
	if strings.Join(names, "|") != "Holiday/1-Beach.webp|Holiday/2-Beach.webp|report.json" {
		t.Errorf("archive entries %v", names)
	}
	if len(report.Files) != 3 || report.JobId != jc.Job.Id || report.Files[2].Status != "failed" ||
		report.Files[2].Error.Code != "decode_failed" || *report.Files[1].Output != "Holiday/2-Beach.webp" ||
		report.Files[0].SavedPercent == nil {
		t.Errorf("report %+v", report)
	}
	expect(t, e.do(t, "GET", "/api/v1/jobs/"+jc.Job.Id+"/download", jc.Token, "", nil), http.StatusNotFound, codeNotFound)
}

func TestPresets(t *testing.T) {
	e := newAPI(t, Options{})
	resp := e.do(t, "GET", "/api/v1/presets", "", "", nil)
	var got struct {
		Presets []api.Preset `json:"presets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range got.Presets {
		ids = append(ids, p.Id)
		// Every bundled preset is accepted by POST /jobs unchanged.
		doc, _ := json.Marshal(map[string]any{"pipeline": p.Pipeline, "naming": p.Naming, "expectedFiles": 1})
		r := e.do(t, "POST", "/api/v1/jobs", "", "application/json", bytes.NewReader(doc))
		if r.StatusCode != http.StatusCreated {
			b, _ := io.ReadAll(r.Body)
			t.Errorf("preset %s: %d %s", p.Id, r.StatusCode, b)
		}
	}
	if strings.Join(ids, ",") != "web,thumbnail,social,archive" {
		t.Errorf("presets %v", ids)
	}
}

func TestClientLimits(t *testing.T) {
	e := newAPI(t, Options{RequestsPerMinute: 5})
	codes := map[int]int{}
	var retry string
	for range 7 {
		resp := e.do(t, "GET", "/api/v1/capabilities", "", "", nil)
		codes[resp.StatusCode]++
		if resp.StatusCode == http.StatusTooManyRequests {
			retry = resp.Header.Get("Retry-After")
		}
	}
	if codes[200] != 5 || codes[429] != 2 || retry == "" {
		t.Errorf("status counts %v, Retry-After %q", codes, retry)
	}
	// Health and UI are not limited.
	if resp := e.do(t, "GET", "/healthz", "", "", nil); resp.StatusCode != 200 {
		t.Errorf("healthz: %d", resp.StatusCode)
	}

	e2 := newAPI(t, Options{})
	// newAPI allows 5 open jobs per client.
	for i := range 6 {
		_, resp := e2.create(t, emptyPipeline, 1)
		if i < 5 && resp.StatusCode != http.StatusCreated {
			t.Fatalf("job %d: %d", i, resp.StatusCode)
		}
		if i == 5 {
			expect(t, resp, http.StatusTooManyRequests, codeClientJobs)
		}
	}
}

func TestClientIdentity(t *testing.T) {
	ids := newClientIdentifier([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	req := func(remote string, xff ...string) *http.Request {
		r := httptest.NewRequest("GET", "/api/v1/capabilities", nil)
		r.RemoteAddr = remote
		for _, v := range xff {
			r.Header.Add("X-Forwarded-For", v)
		}
		return r
	}
	for name, tc := range map[string]struct {
		r    *http.Request
		want string
	}{
		"direct":             {req("203.0.113.5:1234"), "203.0.113.5"},
		"xff from untrusted": {req("203.0.113.5:1234", "198.51.100.7"), "203.0.113.5"},
		"xff from proxy":     {req("10.1.2.3:1234", "198.51.100.7"), "198.51.100.7"},
		"spoofed left entry": {req("10.1.2.3:1234", "1.2.3.4, 198.51.100.7"), "198.51.100.7"},
		"chain of proxies":   {req("10.1.2.3:1234", "198.51.100.7, 10.9.9.9"), "198.51.100.7"},
		"malformed entry":    {req("10.1.2.3:1234", "junk, 10.9.9.9"), "10.9.9.9"},
		"mapped IPv4":        {req("[::ffff:203.0.113.5]:1"), "203.0.113.5"},
	} {
		if got := ids.address(tc.r).String(); got != tc.want {
			t.Errorf("%s: %s, want %s", name, got, tc.want)
		}
	}
	// One /64 is one client; the key does not contain the address.
	a := ids.key(req("[2001:db8:1:2::1]:1"))
	b := ids.key(req("[2001:db8:1:2:ffff::9]:1"))
	c := ids.key(req("[2001:db8:1:3::1]:1"))
	if a != b || a == c || strings.Contains(a, "2001") || len(a) != 32 {
		t.Errorf("IPv6 keys %s %s %s", a, b, c)
	}
}

func TestRateLimiterRefills(t *testing.T) {
	l := newRateLimiter(60)
	now := time.Unix(0, 0)
	l.now = func() time.Time { return now }
	for range 60 {
		if ok, _ := l.allow("k"); !ok {
			t.Fatal("burst exhausted early")
		}
	}
	if ok, wait := l.allow("k"); ok || wait != 1 {
		t.Fatalf("61st request: ok %v, wait %d", ok, wait)
	}
	now = now.Add(time.Second)
	if ok, _ := l.allow("k"); !ok {
		t.Error("no token after one second")
	}
}

func TestUploadKeepsFolders(t *testing.T) {
	e := newAPI(t, Options{})
	jc, _ := e.create(t, emptyPipeline, 1)
	resp := e.upload(t, jc.Job.Id, jc.Token, `../a/./b\c/..//photo.png`, pngHeader)
	var f api.JobFile
	_ = json.NewDecoder(resp.Body).Decode(&f)
	if f.Source == nil || *f.Source != "a/b/c/photo.png" {
		t.Errorf("source %v", f.Source)
	}
}
