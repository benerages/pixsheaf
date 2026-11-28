package imaging

import "testing"

func TestLoaderDisabled(t *testing.T) {
	t.Cleanup(func() { disabledFormats = map[string]bool{} })
	DisableFormats([]string{"heif", "gif"})
	if !loaderDisabled([]string{"gif"}) || loaderDisabled([]string{"heif", "avif"}) || loaderDisabled([]string{"png"}) || loaderDisabled(nil) {
		t.Error("only loaders whose formats are all disabled stay blocked")
	}
	DisableFormats([]string{"avif"})
	if !loaderDisabled([]string{"heif", "avif"}) {
		t.Error("the HEIF loader stays blocked when HEIC and AVIF are disabled")
	}
}
