package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/benerages/pixsheaf/internal/api"
)

// Problem codes (api/openapi.yaml, RFC 9457 problem details).
const (
	codeInvalidJSON       = "invalid_json"
	codePipelineInvalid   = "pipeline_invalid"
	codeRequestInvalid    = "request_invalid"
	codeUnsupportedMedia  = "unsupported_media_type"
	codeUnsupportedFormat = "unsupported_format"
	codeFileTooLarge      = "file_too_large"
	codeJobTooLarge       = "job_too_large"
	codeBodyTooLarge      = "body_too_large"
	codeJobNotOpen        = "job_not_open"
	codeJobNotDone        = "job_not_done"
	codeNoResult          = "no_result"
	codeQueueFull         = "queue_full"
	codeTooManyJobs       = "too_many_jobs"
	codeClientJobs        = "too_many_client_jobs"
	codeRateLimited       = "rate_limited"
	codeNamingInvalid     = "naming_invalid"
	codeUnavailable       = "unavailable"
	codeNotFound          = "not_found"
	codeUnauthorized      = "unauthorized"
	codeCrossOrigin       = "cross_origin"
	codeInternal          = "internal"
)

// writeProblem sends an RFC 9457 problem. detail must not contain file
// contents, file names, or paths.
func writeProblem(w http.ResponseWriter, status int, code, detail string, errs ...api.ValidationError) {
	p := api.Problem{Type: "about:blank", Title: http.StatusText(status), Status: status, Code: code}
	if detail != "" {
		p.Detail = &detail
	}
	if len(errs) > 0 {
		p.Errors = &errs
	}
	h := w.Header()
	h.Set("Content-Type", "application/problem+json")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}

// writeBusy sends 503 with Retry-After.
func writeBusy(w http.ResponseWriter, code, detail string, retryAfter int) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	writeProblem(w, http.StatusServiceUnavailable, code, detail)
}

// writeJSON sends a JSON body with Cache-Control: no-store.
func writeJSON(w http.ResponseWriter, status int, v any) {
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
