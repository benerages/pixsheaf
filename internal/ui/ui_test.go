package ui

import (
	"io/fs"
	"testing"
)

func TestIndexIsEmbedded(t *testing.T) {
	b, err := fs.ReadFile(FS(), "index.html")
	if err != nil {
		t.Fatalf("index.html not embedded: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("index.html is empty")
	}
}
