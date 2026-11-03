// Package version exposes build-time identity for the Warden binary.
//
// All three vars are set by ldflags at build time. The Taskfile's `build`
// target (and Dockerfile.warden) sets them; running `go build .` directly
// leaves the dev defaults below.
//
//	go build -ldflags="\
//	  -X warden/internal/version.Version=v0.24.0 \
//	  -X warden/internal/version.Commit=abc1234 \
//	  -X warden/internal/version.BuildDate=2026-05-09T12:00:00Z" \
//	  ./warden
//
// At runtime: /version returns these as JSON, /healthz includes them, and
// the UI footer shows {Version}+{Commit}.
package version

// Build-time identity. Defaults indicate an un-stamped local `go build`.
var (
	Version   = "v0.0.0-dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Info bundles the three vars for marshaling to JSON or templates.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
}

// Get returns the current build identity. Cheap; safe to call in hot paths.
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
	}
}
