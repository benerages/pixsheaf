// Package version reports the build version, set at link time with
// -ldflags "-X github.com/benerages/pixsheaf/internal/version.Version=...".
package version

// Version is the release version, or "dev" for local builds.
var Version = "dev"
