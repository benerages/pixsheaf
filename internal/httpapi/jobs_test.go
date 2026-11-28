package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/benerages/pixsheaf/internal/api"
	"github.com/benerages/pixsheaf/internal/jobproto"
	"github.com/benerages/pixsheaf/internal/jobs"
)

var pngHeader = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00")

const emptyPipeline = `{"version":1,"steps":[]}`

type apiEnv struct {
	ts  *httptest.Server
	m   *jobs.Manager
	dir string
}

// newAPI starts a test server; an optional logger replaces the silent one.
func newAPI(t *testing.T, opts Options, logger ...*slog.Logger) *apiEnv {
	return newAPIWith(t, opts, nil, logger...)
}

// newAPIWith starts a test server with job limits changed by limits.
func newAPIWith(t *testing.T, opts Options, limits func(*jobs.Limits), logger ...*slog.Logger) *apiEnv {
	t.Helper()
	dir := t.TempDir()
	lim := jobs.Limits{
		MaxUploadBytes: 1000, MaxJobBytes: 5000, MaxFiles: 10, MaxOutputBytes: 1000,
		MaxInputPixels: 100, MaxOutputPixels: 100, MaxFrames: 2,
		MaxConcurrency: 1, MaxQueued: 10, MaxJobs: 10, MaxJobsPerClient: 5, MaxStoredBytes: 1 << 20,
		TTL: time.Minute, LeaseTimeout: time.Minute,
	}
	if limits != nil {
		limits(&lim)
	}
	m, err := jobs.New(dir, lim)
	if err != nil {
		t.Fatal(err)
	}
	opts.Jobs = m
	ui := fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if len(logger) > 0 {
		log = logger[0]
	}
	ts := httptest.NewServer(New("127.0.0.1:0", ui, log, opts).Handler)
	t.Cleanup(func() {
		ts.Close()
		_ = m.Close()
	})
	return &apiEnv{ts: ts, m: m, dir: dir}
}

// do sends a request; token "" sends none.
func (e *apiEnv) do(t *testing.T, method, path, token, contentType string, body io.Reader, hdr ...string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, e.ts.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func (e *apiEnv) create(t *testing.T, pipeline string, n int) (api.JobCreated, *http.Response) {
	t.Helper()
	body := `{"pipeline":` + pipeline + `,"expectedFiles":` + itoa(n) + `}`
	resp := e.do(t, "POST", "/api/v1/jobs", "", "application/json", strings.NewReader(body))
	var jc api.JobCreated
	if resp.StatusCode == http.StatusCreated {
		if err := json.NewDecoder(resp.Body).Decode(&jc); err != nil {
			t.Fatal(err)
		}
	}
	return jc, resp
}

func (e *apiEnv) upload(t *testing.T, jobID, token, name string, data []byte, hdr ...string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	_ = mw.Close()
	return e.do(t, "POST", "/api/v1/jobs/"+jobID+"/files", token, mw.FormDataContentType(), &buf, hdr...)
}

// work completes the next task like a worker would.
func (e *apiEnv) work(t *testing.T, resp jobproto.Response, output string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	task, err := e.m.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = task.Input.Close()
	resp.OutputBytes = int64(len(output))
	if err := e.m.Complete(task.ID, resp, strings.NewReader(output)); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func problemCode(t *testing.T, resp *http.Response) string {
	t.Helper()
	var p api.Problem
	if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json; charset=utf-8" {
		t.Fatalf("content type %q for status %d", ct, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	return p.Code
}

func expect(t *testing.T, resp *http.Response, status int, code string) {
	t.Helper()
	if resp.StatusCode != status {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s: status %d (%s), want %d", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, body, status)
	}
	if code != "" {
		if got := problemCode(t, resp); got != code {
			t.Errorf("%s %s: code %q, want %q", resp.Request.Method, resp.Request.URL.Path, got, code)
		}
	}
}

func TestSingleImageFlow(t *testing.T) {
	e := newAPI(t, Options{MaxWatermarkBytes: 1024})
	resp := e.do(t, "GET", "/api/v1/capabilities", "", "", nil)
	var caps api.Capabilities
	if err := json.NewDecoder(resp.Body).Decode(&caps); err != nil || caps.Limits.MaxUploadBytes != 1000 || caps.PipelineVersion != 1 || caps.Version == "" {
		t.Fatalf("capabilities %+v, %v", caps, err)
	}

	jc, resp := e.create(t, emptyPipeline, 1)
	expect(t, resp, http.StatusCreated, "")
	if resp.Header.Get("Location") != "/api/v1/jobs/"+jc.Job.Id || len(jc.Token) != 43 {
		t.Errorf("location %q, token %d chars", resp.Header.Get("Location"), len(jc.Token))
	}
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == cookiePrefix+jc.Job.Id {
			cookie = c
		}
	}
	if cookie == nil || cookie.Value != jc.Token || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode ||
		cookie.Path != "/" || cookie.MaxAge < 50 {
		t.Fatalf("cookie %+v", cookie)
	}

	resp = e.upload(t, jc.Job.Id, jc.Token, "../Ferien Foto.heic", pngHeader)
	expect(t, resp, http.StatusCreated, "")
	var f api.JobFile
	_ = json.NewDecoder(resp.Body).Decode(&f)
	if f.Status != "queued" || f.Input.Format != "png" {
		t.Errorf("file %+v", f)
	}

	// Follow the job with the cookie, as EventSource would.
	evReq, _ := http.NewRequest("GET", e.ts.URL+"/api/v1/jobs/"+jc.Job.Id+"/events", nil)
	evReq.AddCookie(cookie)
	evResp, err := http.DefaultClient.Do(evReq)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = evResp.Body.Close() }()
	if evResp.Header.Get("Content-Type") != "text/event-stream; charset=utf-8" {
		t.Fatalf("events: %d %s", evResp.StatusCode, evResp.Header.Get("Content-Type"))
	}
	done := make(chan []string)
	go func() {
		var events []string
		sc := bufio.NewScanner(evResp.Body)
		for sc.Scan() {
			if ev, ok := strings.CutPrefix(sc.Text(), "event: "); ok {
				events = append(events, ev)
			}
		}
		done <- events
	}()

	expect(t, e.do(t, "GET", "/api/v1/jobs/"+jc.Job.Id+"/download", jc.Token, "", nil), http.StatusConflict, codeJobNotDone)
	e.work(t, jobproto.Response{Output: &jobproto.ImageInfo{Format: "webp", Width: 2, Height: 3, Pages: 1}}, "RIFFdata")

	select {
	case events := <-done:
		if len(events) < 3 || events[len(events)-1] != "job" {
			t.Errorf("events %v; want file and job events ending with job", events)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("event stream did not end after the job was done")
	}

	var job api.Job
	resp = e.do(t, "GET", "/api/v1/jobs/"+jc.Job.Id, jc.Token, "", nil)
	_ = json.NewDecoder(resp.Body).Decode(&job)
	if job.Status != "done" || job.Files[0].Output == nil || job.Files[0].Output.Name != "Ferien Foto.webp" {
		t.Fatalf("job %+v", job)
	}

	dlReq, _ := http.NewRequest("GET", e.ts.URL+"/api/v1/jobs/"+jc.Job.Id+"/download", nil)
	dlReq.AddCookie(cookie)
	dl, err := http.DefaultClient.Do(dlReq)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(dl.Body)
	_ = dl.Body.Close()
	if dl.StatusCode != 200 || string(body) != "RIFFdata" || dl.Header.Get("Content-Type") != "image/webp" ||
		dl.Header.Get("Content-Disposition") != `attachment; filename="Ferien Foto.webp"; filename*=UTF-8''Ferien%20Foto.webp` {
		t.Fatalf("download %d %q %v", dl.StatusCode, body, dl.Header)
	}
	expect(t, e.do(t, "GET", "/api/v1/jobs/"+jc.Job.Id+"/download", jc.Token, "", nil), http.StatusNotFound, codeNotFound)
	if entries, _ := os.ReadDir(e.dir); len(entries) != 0 {
		t.Errorf("%d files left after the download", len(entries))
	}
}

func TestJobAccess(t *testing.T) {
	e := newAPI(t, Options{})
	jc, _ := e.create(t, emptyPipeline, 1)
	path := "/api/v1/jobs/" + jc.Job.Id
	cookie := cookiePrefix + jc.Job.Id + "=" + jc.Token

	expect(t, e.do(t, "GET", path, "", "", nil), http.StatusUnauthorized, codeUnauthorized)
	expect(t, e.do(t, "GET", path, "wrong", "", nil), http.StatusUnauthorized, codeUnauthorized)
	// The cookie is accepted only for events and download.
	expect(t, e.do(t, "GET", path, "", "", nil, "Cookie", cookie), http.StatusUnauthorized, codeUnauthorized)
	expect(t, e.do(t, "DELETE", path, "", "", nil, "Cookie", cookie), http.StatusUnauthorized, codeUnauthorized)
	expect(t, e.upload(t, jc.Job.Id, "", "a.png", pngHeader, "Cookie", cookie), http.StatusUnauthorized, codeUnauthorized)
	expect(t, e.do(t, "GET", path+"/download", "", "", nil, "Cookie", cookie), http.StatusConflict, codeJobNotDone)
	// The cookie of another job does not open this one.
	other, _ := e.create(t, emptyPipeline, 1)
	foreign := cookiePrefix + jc.Job.Id + "=" + other.Token
	expect(t, e.do(t, "GET", path+"/download", "", "", nil, "Cookie", foreign), http.StatusUnauthorized, codeUnauthorized)
	misnamed := cookiePrefix + other.Job.Id + "=" + jc.Token
	expect(t, e.do(t, "GET", path+"/download", "", "", nil, "Cookie", misnamed), http.StatusUnauthorized, codeUnauthorized)

	expect(t, e.do(t, "GET", "/api/v1/jobs/AAAAAAAAAAAAAAAAAAAAAA", jc.Token, "", nil), http.StatusNotFound, codeNotFound)
	expect(t, e.do(t, "GET", "/api/v1/jobs/..%2f..", jc.Token, "", nil), http.StatusNotFound, "")
	expect(t, e.do(t, "GET", "/api/v1/nothing", "", "", nil), http.StatusNotFound, codeNotFound)

	expect(t, e.do(t, "DELETE", path, jc.Token, "", nil), http.StatusNoContent, "")
	expect(t, e.do(t, "GET", path, jc.Token, "", nil), http.StatusNotFound, codeNotFound)
}

func TestCrossOriginRejected(t *testing.T) {
	e := newAPI(t, Options{})
	body := `{"pipeline":` + emptyPipeline + `,"expectedFiles":1}`
	for name, hdr := range map[string][]string{
		"sec-fetch-site cross": {"Sec-Fetch-Site", "cross-site"},
		"sec-fetch-site same":  {"Sec-Fetch-Site", "same-site"},
		"foreign origin":       {"Origin", "https://evil.example"},
		"null origin":          {"Origin", "null"},
	} {
		resp := e.do(t, "POST", "/api/v1/jobs", "", "application/json", strings.NewReader(body), hdr...)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", name, resp.StatusCode)
		}
	}
	host := strings.TrimPrefix(e.ts.URL, "http://")
	resp := e.do(t, "POST", "/api/v1/jobs", "", "application/json", strings.NewReader(body),
		"Origin", "https://"+host, "Sec-Fetch-Site", "same-origin")
	expect(t, resp, http.StatusCreated, "")
	var jc api.JobCreated
	_ = json.NewDecoder(resp.Body).Decode(&jc)
	expect(t, e.upload(t, jc.Job.Id, jc.Token, "a.png", pngHeader, "Sec-Fetch-Site", "cross-site"), http.StatusForbidden, codeCrossOrigin)
	expect(t, e.do(t, "DELETE", "/api/v1/jobs/"+jc.Job.Id, jc.Token, "", nil, "Origin", "https://evil.example"), http.StatusForbidden, codeCrossOrigin)
}

func TestCreateRejected(t *testing.T) {
	e := newAPI(t, Options{})
	post := func(ct, body string) *http.Response {
		return e.do(t, "POST", "/api/v1/jobs", "", ct, strings.NewReader(body))
	}
	expect(t, post("text/plain", `{}`), http.StatusUnsupportedMediaType, codeUnsupportedMedia)
	expect(t, post("application/json", `{`), http.StatusBadRequest, codeInvalidJSON)
	expect(t, post("application/json", `{"pipeline":{},"expectedFiles":1,"x":1}`), http.StatusBadRequest, codeInvalidJSON)
	expect(t, post("application/json", `{"expectedFiles":1}`), http.StatusUnprocessableEntity, codePipelineInvalid)
	expect(t, post("application/json", `{"pipeline":`+emptyPipeline+`,"expectedFiles":0}`), http.StatusUnprocessableEntity, codeRequestInvalid)
	expect(t, post("application/json", `{"pipeline":`+emptyPipeline+`,"expectedFiles":1,"naming":{"pattern":"{nope}"}}`), http.StatusUnprocessableEntity, codePipelineInvalid)
	expect(t, post("application/json", `{"pipeline":`+emptyPipeline+`,"expectedFiles":1,"naming":{"x":1}}`), http.StatusBadRequest, codeInvalidJSON)
	expect(t, post("application/json", strings.Repeat(" ", maxCreateBytes+1)), http.StatusRequestEntityTooLarge, codeBodyTooLarge)

	resp := post("application/json", `{"pipeline":{"version":2,"steps":[]},"expectedFiles":1}`)
	expect(t, resp, http.StatusUnprocessableEntity, "")
	var p api.Problem
	_ = json.NewDecoder(resp.Body).Decode(&p)
	if p.Errors == nil || (*p.Errors)[0].Pointer != "/pipeline/version" {
		t.Errorf("problem %+v", p)
	}

	// Image watermarks are disabled with MaxWatermarkBytes 0.
	wm := `{"version":1,"steps":[{"op":"watermark","params":{"image":"iVBORw0KGgo="}}]}`
	resp = post("application/json", `{"pipeline":`+wm+`,"expectedFiles":1}`)
	expect(t, resp, http.StatusUnprocessableEntity, "")
	p = api.Problem{}
	_ = json.NewDecoder(resp.Body).Decode(&p)
	if p.Errors == nil || (*p.Errors)[0].Pointer != "/pipeline/steps/0/params/image" {
		t.Errorf("watermark problem %+v", p)
	}
}

func TestUploadRejected(t *testing.T) {
	e := newAPI(t, Options{})
	jc, _ := e.create(t, emptyPipeline, 1)
	expect(t, e.upload(t, jc.Job.Id, jc.Token, "a.svg", []byte("<svg/>")), http.StatusUnsupportedMediaType, codeUnsupportedFormat)
	// A PNG with a name that is not an image file name is refused too
	// (ASVS 5.0 5.2.2, ADR 0023).
	for _, name := range []string{"a.txt", "a", "a.png.php", ""} {
		expect(t, e.upload(t, jc.Job.Id, jc.Token, name, pngHeader), http.StatusUnsupportedMediaType, codeUnsupportedFormat)
	}
	expect(t, e.upload(t, jc.Job.Id, jc.Token, "a.png", append(bytes.Clone(pngHeader), make([]byte, 2000)...)), http.StatusRequestEntityTooLarge, codeFileTooLarge)
	expect(t, e.do(t, "POST", "/api/v1/jobs/"+jc.Job.Id+"/files", jc.Token, "image/png", bytes.NewReader(pngHeader)), http.StatusUnsupportedMediaType, codeUnsupportedMedia)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormField("other")
	_, _ = fw.Write(pngHeader)
	_ = mw.Close()
	expect(t, e.do(t, "POST", "/api/v1/jobs/"+jc.Job.Id+"/files", jc.Token, mw.FormDataContentType(), &buf), http.StatusBadRequest, codeRequestInvalid)

	expect(t, e.upload(t, jc.Job.Id, jc.Token, "a.png", pngHeader), http.StatusCreated, "")
	expect(t, e.upload(t, jc.Job.Id, jc.Token, "b.png", pngHeader), http.StatusConflict, codeJobNotOpen)
}

func TestFailedFileAndCancel(t *testing.T) {
	e := newAPI(t, Options{})
	jc, _ := e.create(t, emptyPipeline, 1)
	resp := e.upload(t, jc.Job.Id, jc.Token, "a.png", pngHeader)
	var f api.JobFile
	_ = json.NewDecoder(resp.Body).Decode(&f)
	e.work(t, jobproto.Response{Error: &jobproto.Error{Code: "decode_failed", Message: "the image could not be decoded"}}, "")

	resp = e.do(t, "GET", "/api/v1/jobs/"+jc.Job.Id+"/files/"+f.Id, jc.Token, "", nil)
	var got api.JobFile
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got.Status != "failed" || got.Error == nil || got.Error.Code != "decode_failed" {
		t.Errorf("file %+v", got)
	}
	expect(t, e.do(t, "GET", "/api/v1/jobs/"+jc.Job.Id+"/download", jc.Token, "", nil), http.StatusConflict, codeNoResult)

	jc2, _ := e.create(t, emptyPipeline, 1)
	resp = e.upload(t, jc2.Job.Id, jc2.Token, "a.png", pngHeader)
	_ = json.NewDecoder(resp.Body).Decode(&f)
	expect(t, e.do(t, "DELETE", "/api/v1/jobs/"+jc2.Job.Id+"/files/"+f.Id, jc2.Token, "", nil), http.StatusNoContent, "")
	resp = e.do(t, "GET", "/api/v1/jobs/"+jc2.Job.Id, jc2.Token, "", nil)
	var job api.Job
	_ = json.NewDecoder(resp.Body).Decode(&job)
	if job.Status != "done" || job.Files[0].Status != "canceled" {
		t.Errorf("job after cancel %+v", job)
	}
}

func TestContentDisposition(t *testing.T) {
	for in, want := range map[string]string{
		"a.png":          `attachment; filename="a.png"; filename*=UTF-8''a.png`,
		"Ärger 100%.jpg": `attachment; filename="_rger 100_.jpg"; filename*=UTF-8''%C3%84rger%20100%25.jpg`,
		`a"b\c.gif`:      `attachment; filename="a_b_c.gif"; filename*=UTF-8''a%22b%5Cc.gif`,
	} {
		if got := contentDisposition(in); got != want {
			t.Errorf("contentDisposition(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestDisabledFormat(t *testing.T) {
	e := newAPIWith(t, Options{}, func(l *jobs.Limits) { l.DisabledFormats = []string{"heif", "zip"} })
	resp := e.do(t, "GET", "/api/v1/capabilities", "", "", nil)
	var caps api.Capabilities
	_ = json.NewDecoder(resp.Body).Decode(&caps)
	for _, f := range caps.InputFormats {
		if f == "heif" || f == "zip" {
			t.Errorf("capabilities list disabled format %q", f)
		}
	}
	jc, _ := e.create(t, emptyPipeline, 1)
	heic := []byte("\x00\x00\x00\x18ftypheic")
	expect(t, e.upload(t, jc.Job.Id, jc.Token, "a.heic", heic), http.StatusUnsupportedMediaType, codeFormatDisabled)
	expect(t, e.upload(t, jc.Job.Id, jc.Token, "a.png", pngHeader), http.StatusCreated, "")
}
