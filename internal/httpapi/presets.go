package httpapi

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"path"

	"github.com/benerages/pixsheaf/internal/api"
	"github.com/benerages/pixsheaf/internal/jobs"
	"github.com/benerages/pixsheaf/internal/pipeline"
)

// Bundled presets (ADR 0022). The server stores no presets; these are
// compiled in and validated at start.
//
//go:embed presets/*.json
var presetFiles embed.FS

// presetOrder is the order in which presets are listed.
var presetOrder = []string{"web", "thumbnail", "social", "archive"}

// loadPresets reads and validates the bundled presets.
func loadPresets() ([]json.RawMessage, error) {
	out := make([]json.RawMessage, 0, len(presetOrder))
	for _, id := range presetOrder {
		data, err := presetFiles.ReadFile(path.Join("presets", id+".json"))
		if err != nil {
			return nil, err
		}
		var p struct {
			Format   string          `json:"format"`
			ID       string          `json:"id"`
			Title    string          `json:"title"`
			Pipeline json.RawMessage `json:"pipeline"`
			Naming   *struct {
				Pattern string `json:"pattern"`
			} `json:"naming"`
		}
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("preset %s: %w", id, err)
		}
		if p.Format != string(api.PixsheafPreset) || p.ID != id || p.Title == "" {
			return nil, fmt.Errorf("preset %s: wrong format, id, or title", id)
		}
		if _, err := pipeline.Parse(p.Pipeline, pipeline.Limits{}); err != nil {
			return nil, fmt.Errorf("preset %s: %w", id, err)
		}
		if p.Naming != nil {
			if err := jobs.ValidatePattern(p.Naming.Pattern); err != nil {
				return nil, fmt.Errorf("preset %s: %w", id, err)
			}
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, data); err != nil {
			return nil, err
		}
		out = append(out, compact.Bytes())
	}
	return out, nil
}

func (a *jobAPI) listPresets(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"presets": a.presets})
}
