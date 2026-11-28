package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/benerages/pixsheaf/internal/api"
	"github.com/benerages/pixsheaf/internal/jobs"
	"github.com/benerages/pixsheaf/internal/pipeline"
	"github.com/benerages/pixsheaf/internal/version"
)

// maxCreateBytes bounds a POST /jobs body: the pipeline with an image
// watermark of up to 1 MiB in base64 plus the rest of the document.
const maxCreateBytes = 2 << 20

// multipartOverhead is allowed on top of the file size for the multipart
// framing of an upload.
const multipartOverhead = 64 << 10

// eventPing is the interval of SSE keep-alive comments.
const eventPing = 15 * time.Second

// Options configure the job API.
type Options struct {
	Jobs *jobs.Manager
	// MaxSteps bounds pipeline steps (0: the hard limit).
	MaxSteps int
	// MaxWatermarkBytes bounds an image watermark; 0 disables them.
	MaxWatermarkBytes int
	// TrustedProxies may set X-Forwarded-For (ADR 0022).
	TrustedProxies []netip.Prefix
	// RequestsPerMinute limits API requests per client; 0 disables the
	// limit (tests only).
	RequestsPerMinute int
	// Requests, if set, counts API responses for the metrics endpoint.
	Requests *RequestStats
}

type jobAPI struct {
	m       *jobs.Manager
	opts    Options
	logger  *slog.Logger
	presets []json.RawMessage
}

func (a *jobAPI) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/capabilities", a.capabilities)
	mux.HandleFunc("GET /api/v1/presets", a.listPresets)
	mux.HandleFunc("POST /api/v1/jobs", csrf(a.createJob))
	mux.HandleFunc("GET /api/v1/jobs/{jobId}", a.withJob(false, a.getJob))
	mux.HandleFunc("DELETE /api/v1/jobs/{jobId}", csrf(a.withJob(false, a.deleteJob)))
	mux.HandleFunc("POST /api/v1/jobs/{jobId}/files", csrf(a.withJob(false, a.uploadFile)))
	mux.HandleFunc("GET /api/v1/jobs/{jobId}/files/{fileId}", a.withJob(false, a.getFile))
	mux.HandleFunc("DELETE /api/v1/jobs/{jobId}/files/{fileId}", csrf(a.withJob(false, a.cancelFile)))
	mux.HandleFunc("GET /api/v1/jobs/{jobId}/events", a.withJob(true, a.events))
	mux.HandleFunc("GET /api/v1/jobs/{jobId}/download", a.withJob(true, a.download))
	mux.HandleFunc("GET /api/v1/jobs/{jobId}/files/{fileId}/download", a.withJob(true, a.downloadFile))
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, _ *http.Request) {
		writeProblem(w, http.StatusNotFound, codeNotFound, "")
	})
}

// withJob checks the job ID and token before calling next with the job ID.
func (a *jobAPI) withJob(allowCookie bool, next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("jobId")
		if !validID(id) {
			writeProblem(w, http.StatusNotFound, codeNotFound, "")
			return
		}
		switch err := a.m.Authorize(id, requestToken(r, id, allowCookie)); {
		case errors.Is(err, jobs.ErrNotFound):
			writeProblem(w, http.StatusNotFound, codeNotFound, "")
			return
		case err != nil:
			w.Header().Set("WWW-Authenticate", `Bearer realm="pixsheaf"`)
			writeProblem(w, http.StatusUnauthorized, codeUnauthorized, "the job token is missing or wrong")
			return
		}
		next(w, r, id)
	}
}

func (a *jobAPI) capabilities(w http.ResponseWriter, _ *http.Request) {
	lim := a.m.Limits()
	maxSteps := pipeline.MaxSteps
	if a.opts.MaxSteps > 0 && a.opts.MaxSteps < maxSteps {
		maxSteps = a.opts.MaxSteps
	}
	inputs := make([]api.InputFormat, 0, 10)
	for _, f := range []api.InputFormat{"jpeg", "png", "webp", "gif", "tiff", "bmp", "avif", "heif", "jxl", "zip"} {
		if !slices.Contains(lim.DisabledFormats, string(f)) {
			inputs = append(inputs, f)
		}
	}
	writeJSON(w, http.StatusOK, api.Capabilities{
		Version:         version.Version,
		InputFormats:    inputs,
		OutputFormats:   []api.OutputFormat{"jpeg", "png", "webp", "gif", "tiff", "bmp", "avif", "jxl", "pdf"},
		PipelineVersion: pipeline.Version,
		Limits: api.Limits{
			MaxUploadBytes:    int(lim.MaxUploadBytes),
			MaxFilesPerJob:    lim.MaxFiles,
			MaxJobBytes:       int(lim.MaxJobBytes),
			MaxInputPixels:    int(lim.MaxInputPixels),
			MaxOutputPixels:   int(lim.MaxOutputPixels),
			MaxFrames:         lim.MaxFrames,
			MaxSteps:          maxSteps,
			MaxWatermarkBytes: a.opts.MaxWatermarkBytes,
			MaxArchiveBytes:   int(lim.MaxArchiveBytes),
			MaxArchiveEntries: lim.MaxArchiveEntries,
			MaxJobsPerClient:  lim.MaxJobsPerClient,
			RequestsPerMinute: a.opts.RequestsPerMinute,
		},
	})
}

func (a *jobAPI) createJob(w http.ResponseWriter, r *http.Request) {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		writeProblem(w, http.StatusUnsupportedMediaType, codeUnsupportedMedia, "send the job as application/json")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCreateBytes))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeProblem(w, http.StatusRequestEntityTooLarge, codeBodyTooLarge, fmt.Sprintf("the request body is limited to %d bytes", maxCreateBytes))
			return
		}
		writeProblem(w, http.StatusBadRequest, codeInvalidJSON, "the request body could not be read")
		return
	}
	var req struct {
		Pipeline      json.RawMessage `json:"pipeline"`
		ExpectedFiles *int            `json:"expectedFiles"`
		Naming        *struct {
			Pattern     *string `json:"pattern"`
			KeepFolders bool    `json:"keepFolders"`
		} `json:"naming"`
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || dec.More() {
		writeProblem(w, http.StatusBadRequest, codeInvalidJSON, "the request body is not a valid job document")
		return
	}
	var verrs []api.ValidationError
	if req.ExpectedFiles == nil {
		verrs = append(verrs, api.ValidationError{Pointer: "/expectedFiles", Message: "is required"})
	}
	if len(req.Pipeline) == 0 {
		verrs = append(verrs, api.ValidationError{Pointer: "/pipeline", Message: "is required"})
	} else {
		verrs = append(verrs, a.checkPipeline(req.Pipeline)...)
	}
	naming := jobs.Naming{Pattern: "{name}"}
	if req.Naming != nil {
		naming.KeepFolders = req.Naming.KeepFolders
		if req.Naming.Pattern != nil {
			naming.Pattern = *req.Naming.Pattern
			if err := jobs.ValidatePattern(naming.Pattern); err != nil {
				verrs = append(verrs, api.ValidationError{Pointer: "/naming/pattern", Message: strings.TrimPrefix(err.Error(), "jobs: ")})
			}
		}
	}
	if len(verrs) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, codePipelineInvalid, "", verrs...)
		return
	}

	job, token, err := a.m.Create(req.Pipeline, *req.ExpectedFiles, naming, clientKeyFrom(r.Context()))
	switch {
	case errors.Is(err, jobs.ErrJobTooLarge):
		writeProblem(w, http.StatusUnprocessableEntity, codeRequestInvalid, "",
			api.ValidationError{Pointer: "/expectedFiles", Message: fmt.Sprintf("must be between 1 and %d", a.m.Limits().MaxFiles)})
		return
	case errors.Is(err, jobs.ErrClientJobs):
		w.Header().Set("Retry-After", "60")
		writeProblem(w, http.StatusTooManyRequests, codeClientJobs,
			fmt.Sprintf("at most %d open jobs per client; download or delete one first", a.m.Limits().MaxJobsPerClient))
		return
	case errors.Is(err, jobs.ErrTooManyJobs):
		writeBusy(w, codeTooManyJobs, "too many open jobs", 10)
		return
	case err != nil:
		writeBusy(w, codeUnavailable, "the server is shutting down", 10)
		return
	}
	http.SetCookie(w, jobCookie(job.ID, token, int(time.Until(job.ExpiresAt).Seconds())))
	w.Header().Set("Location", "/api/v1/jobs/"+job.ID)
	writeJSON(w, http.StatusCreated, api.JobCreated{Job: toAPIJob(job), Token: token})
}

// checkPipeline validates a pipeline with the operator limits. Pointers
// are relative to the request body.
func (a *jobAPI) checkPipeline(raw json.RawMessage) []api.ValidationError {
	lim := pipeline.Limits{MaxSteps: a.opts.MaxSteps, MaxWatermarkBytes: a.opts.MaxWatermarkBytes}
	p, err := pipeline.Parse(raw, lim)
	var perr *pipeline.Error
	if errors.As(err, &perr) {
		out := make([]api.ValidationError, 0, len(perr.Problems))
		for _, pr := range perr.Problems {
			out = append(out, api.ValidationError{Pointer: "/pipeline" + pr.Pointer, Message: pr.Message})
		}
		return out
	}
	if err != nil {
		return []api.ValidationError{{Pointer: "/pipeline", Message: "is invalid"}}
	}
	if a.opts.MaxWatermarkBytes == 0 {
		var out []api.ValidationError
		for i, s := range p.Steps {
			if s.Watermark != nil && s.Watermark.Image != "" {
				out = append(out, api.ValidationError{
					Pointer: fmt.Sprintf("/pipeline/steps/%d/params/image", i),
					Message: "image watermarks are disabled on this server",
				})
			}
		}
		return out
	}
	return nil
}

func (a *jobAPI) getJob(w http.ResponseWriter, _ *http.Request, id string) {
	job, err := a.m.Get(id)
	if err != nil {
		writeProblem(w, http.StatusNotFound, codeNotFound, "")
		return
	}
	writeJSON(w, http.StatusOK, toAPIJob(job))
}

func (a *jobAPI) deleteJob(w http.ResponseWriter, _ *http.Request, id string) {
	if err := a.m.Delete(id); err != nil {
		writeProblem(w, http.StatusNotFound, codeNotFound, "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *jobAPI) uploadFile(w http.ResponseWriter, r *http.Request, id string) {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "multipart/form-data" {
		writeProblem(w, http.StatusUnsupportedMediaType, codeUnsupportedMedia, "upload the file as multipart/form-data")
		return
	}
	lim := a.m.Limits()
	maxFile := max(lim.MaxUploadBytes, lim.MaxArchiveBytes)
	r.Body = http.MaxBytesReader(w, r.Body, maxFile+multipartOverhead)
	mr, err := r.MultipartReader()
	if err != nil {
		writeProblem(w, http.StatusBadRequest, codeRequestInvalid, "the multipart body could not be read")
		return
	}
	part, err := mr.NextPart()
	if err != nil || part.FormName() != "file" {
		writeProblem(w, http.StatusBadRequest, codeRequestInvalid, `the body must contain one part named "file"`)
		return
	}
	defer func() { _ = part.Close() }()

	name := uploadName(part)
	if !jobs.AllowedExtension(name) {
		writeProblem(w, http.StatusUnsupportedMediaType, codeUnsupportedFormat,
			"the file name must end in an image extension such as .jpg or .png, or in .zip")
		return
	}
	f, err := a.m.AddFile(id, name, part)
	var mbe *http.MaxBytesError
	switch {
	case err == nil:
	case errors.Is(err, jobs.ErrTooLarge), errors.As(err, &mbe):
		writeProblem(w, http.StatusRequestEntityTooLarge, codeFileTooLarge,
			fmt.Sprintf("images are limited to %d bytes and ZIP archives to %d bytes", lim.MaxUploadBytes, lim.MaxArchiveBytes))
		return
	case errors.Is(err, jobs.ErrJobTooLarge):
		writeProblem(w, http.StatusRequestEntityTooLarge, codeJobTooLarge, fmt.Sprintf("all files of a job are limited to %d bytes", lim.MaxJobBytes))
		return
	case errors.Is(err, jobs.ErrUnsupported):
		writeProblem(w, http.StatusUnsupportedMediaType, codeUnsupportedFormat, "the file is not an allowed image format")
		return
	case errors.Is(err, jobs.ErrFormatDisabled):
		writeProblem(w, http.StatusUnsupportedMediaType, codeFormatDisabled, "this image format is disabled on this server")
		return
	case errors.Is(err, jobs.ErrNotOpen):
		writeProblem(w, http.StatusConflict, codeJobNotOpen, "the job accepts no more files")
		return
	case errors.Is(err, jobs.ErrBusy):
		writeBusy(w, codeQueueFull, "the server is busy; retry later", 5)
		return
	case errors.Is(err, jobs.ErrNotFound):
		writeProblem(w, http.StatusNotFound, codeNotFound, "")
		return
	default:
		a.logger.Error("storing upload", "err", err)
		writeProblem(w, http.StatusInternalServerError, codeInternal, "")
		return
	}
	w.Header().Set("Location", "/api/v1/jobs/"+id+"/files/"+f.ID)
	writeJSON(w, http.StatusCreated, toAPIFile(f))
}

func (a *jobAPI) getFile(w http.ResponseWriter, r *http.Request, id string) {
	fileID := r.PathValue("fileId")
	f, err := a.m.GetFile(id, fileID)
	if !validID(fileID) || err != nil {
		writeProblem(w, http.StatusNotFound, codeNotFound, "")
		return
	}
	writeJSON(w, http.StatusOK, toAPIFile(f))
}

func (a *jobAPI) cancelFile(w http.ResponseWriter, r *http.Request, id string) {
	fileID := r.PathValue("fileId")
	if !validID(fileID) || a.m.CancelFile(id, fileID) != nil {
		writeProblem(w, http.StatusNotFound, codeNotFound, "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *jobAPI) download(w http.ResponseWriter, _ *http.Request, id string) {
	d, err := a.m.TakeDownload(id)
	switch {
	case errors.Is(err, jobs.ErrNotReady):
		writeProblem(w, http.StatusConflict, codeJobNotDone, "the job is not done yet")
		return
	case errors.Is(err, jobs.ErrNoResult):
		writeProblem(w, http.StatusConflict, codeNoResult, "the file could not be processed; see the job for the reason")
		return
	case errors.Is(err, jobs.ErrNotFound):
		writeProblem(w, http.StatusNotFound, codeNotFound, "")
		return
	case err != nil:
		a.logger.Error("opening result", "err", err)
		writeProblem(w, http.StatusInternalServerError, codeInternal, "")
		return
	}
	defer d.Release()
	if d.IsArchive() {
		a.sendArchive(w, d)
		return
	}
	a.sendFile(w, d)
}

func (a *jobAPI) downloadFile(w http.ResponseWriter, r *http.Request, id string) {
	fileID := r.PathValue("fileId")
	if !validID(fileID) {
		writeProblem(w, http.StatusNotFound, codeNotFound, "")
		return
	}
	d, err := a.m.TakeFile(id, fileID)
	switch {
	case errors.Is(err, jobs.ErrNotReady):
		writeProblem(w, http.StatusConflict, codeFileNotDone, "the file is not done, or it failed")
		return
	case errors.Is(err, jobs.ErrNotFound):
		writeProblem(w, http.StatusNotFound, codeNotFound, "")
		return
	case err != nil:
		a.logger.Error("opening result", "err", err)
		writeProblem(w, http.StatusInternalServerError, codeInternal, "")
		return
	}
	defer d.Release()
	a.sendFile(w, d)
}

// sendFile streams one result.
func (a *jobAPI) sendFile(w http.ResponseWriter, d jobs.Download) {
	defer func() { _ = d.File.Close() }()
	h := w.Header()
	h.Set("Content-Type", d.ContentType)
	h.Set("Content-Length", strconv.FormatInt(d.Size, 10))
	h.Set("Content-Disposition", contentDisposition(d.Name))
	h.Set("Cache-Control", "no-store")
	// A result opened in the browser runs in a sandbox (relevant for PDF).
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, d.File); err != nil {
		a.logger.Warn("download interrupted", "err", err)
	}
}

// uploadName returns the file name of a multipart part including its
// folder path. Part.FileName drops the folders (filepath.Base), but folder
// uploads send them on purpose; jobs.SourceName sanitizes every segment.
func uploadName(p *multipart.Part) string {
	_, params, err := mime.ParseMediaType(p.Header.Get("Content-Disposition"))
	if err != nil {
		return ""
	}
	return params["filename"]
}

// contentDisposition builds an RFC 6266 attachment header with an ASCII
// fallback and the UTF-8 name (RFC 8187).
func contentDisposition(name string) string {
	var ascii strings.Builder
	for _, r := range name {
		if r >= 0x20 && r < 0x7f && r != '"' && r != '\\' && r != '%' {
			ascii.WriteRune(r)
		} else {
			ascii.WriteByte('_')
		}
	}
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii.String(), url.PathEscape(name))
}

// sendArchive streams the results and report.json as a ZIP archive.
// Images are stored without compression (they are compressed already);
// nothing is buffered beyond the copy buffer.
func (a *jobAPI) sendArchive(w http.ResponseWriter, d jobs.Download) {
	h := w.Header()
	h.Set("Content-Type", "application/zip")
	h.Set("Content-Disposition", contentDisposition("pixsheaf-"+d.Created.UTC().Format("2006-01-02")+".zip"))
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)

	zw := zip.NewWriter(w)
	for _, e := range d.Entries {
		_ = rc.SetWriteDeadline(time.Now().Add(2 * time.Minute))
		if err := a.addEntry(zw, e, d.Created); err != nil {
			a.logger.Warn("archive download interrupted", "err", err)
			return
		}
	}
	report, err := json.MarshalIndent(toReport(d), "", "  ")
	if err == nil {
		var fw io.Writer
		fw, err = zw.CreateHeader(&zip.FileHeader{Name: "report.json", Method: zip.Deflate, Modified: d.Created})
		if err == nil {
			_, err = fw.Write(report)
		}
	}
	if err == nil {
		err = zw.Close()
	}
	if err != nil {
		a.logger.Warn("archive download interrupted", "err", err)
	}
}

func (a *jobAPI) addEntry(zw *zip.Writer, e jobs.ZipEntry, modified time.Time) error {
	f, err := a.m.OpenEntry(e)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: e.Path, Method: zip.Store, Modified: modified})
	if err != nil {
		return err
	}
	_, err = io.Copy(fw, f)
	return err
}

// toReport builds report.json (schema Report).
func toReport(d jobs.Download) api.Report {
	r := api.Report{JobId: d.JobID, CreatedAt: d.Created.UTC(), Files: make([]api.ReportEntry, 0, len(d.Files))}
	inZip := map[int]bool{}
	for _, e := range d.Entries {
		inZip[e.Index] = true
	}
	for _, f := range d.Files {
		e := api.ReportEntry{
			Index: f.Index, Source: f.Source, Status: api.FileStatus(f.Status), InputBytes: int(f.InputBytes),
		}
		if f.InputFormat != "" {
			format := api.InputFormat(f.InputFormat)
			e.InputFormat = &format
		}
		if f.Output != nil && inZip[f.Index] {
			name, n, w, h := f.OutputName, int(f.OutputBytes), f.Output.Width, f.Output.Height
			e.Output, e.OutputBytes, e.Width, e.Height = &name, &n, &w, &h
			if f.InputBytes > 0 {
				saved := float32(math.Round(1000*(1-float64(f.OutputBytes)/float64(f.InputBytes))) / 10)
				e.SavedPercent = &saved
			}
		}
		if f.ErrorCode != "" {
			e.Error = &api.FileError{Code: api.FileErrorCode(f.ErrorCode), Message: f.ErrorMsg}
		}
		r.Files = append(r.Files, e)
	}
	return r
}

// events streams job and file changes as Server-Sent Events.
func (a *jobAPI) events(w http.ResponseWriter, r *http.Request, id string) {
	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	send := func(event string, v any) bool {
		data, err := json.Marshal(v)
		if err != nil {
			return false
		}
		_ = rc.SetWriteDeadline(time.Now().Add(30 * time.Second))
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	ping := time.NewTicker(eventPing)
	defer ping.Stop()

	lastJob := ""
	lastFiles := map[string]string{}
	for {
		changed := a.m.Changes()
		job, err := a.m.Get(id)
		if err != nil {
			// Deleted, downloaded, or expired.
			return
		}
		for _, f := range job.Files {
			if lastFiles[f.ID] != f.Status {
				lastFiles[f.ID] = f.Status
				if !send("file", toAPIFile(f)) {
					return
				}
			}
		}
		if job.Status != lastJob {
			lastJob = job.Status
			if !send("job", toAPIJob(job)) {
				return
			}
		}
		if job.Status == jobs.JobDone || job.Status == jobs.JobCanceled {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-changed:
		case <-ping.C:
			_ = rc.SetWriteDeadline(time.Now().Add(30 * time.Second))
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}

func toAPIJob(j jobs.Job) api.Job {
	files := make([]api.JobFile, 0, len(j.Files))
	for _, f := range j.Files {
		files = append(files, toAPIFile(f))
	}
	return api.Job{
		Id: j.ID, Status: api.JobStatus(j.Status), ExpectedFiles: j.ExpectedFiles,
		CreatedAt: j.CreatedAt.UTC(), ExpiresAt: j.ExpiresAt.UTC(), Files: files,
	}
}

func toAPIFile(f jobs.File) api.JobFile {
	source := f.Source
	out := api.JobFile{
		Id: f.ID, Index: f.Index, Kind: api.JobFileKind(f.Kind), Status: api.FileStatus(f.Status), Source: &source,
	}
	if f.Downloaded {
		downloaded := true
		out.Downloaded = &downloaded
	}
	if f.InputFormat != "" {
		out.Input = &api.ImageInfo{Bytes: int(f.InputBytes), Format: api.InputFormat(f.InputFormat)}
	}
	if f.ArchiveID != "" {
		archive := f.ArchiveID
		out.ArchiveId = &archive
	}
	if f.Output != nil {
		pages := f.Output.Pages
		out.Output = &api.OutputInfo{
			Bytes: int(f.OutputBytes), Format: api.OutputFormat(f.Output.Format),
			Width: f.Output.Width, Height: f.Output.Height, Pages: &pages, Name: f.OutputName,
		}
	}
	if f.ErrorCode != "" {
		out.Error = &api.FileError{Code: api.FileErrorCode(f.ErrorCode), Message: f.ErrorMsg}
	}
	return out
}
