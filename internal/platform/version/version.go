// Package version says which build this binary is: its version and channel (docs/release.md), set by the
// build with -ldflags, and the commit, which Go stamps from the checkout so no build step has to pass it.
package version

import (
	"runtime/debug"
	"sync"
)

// Set at build time, for example:
//
//	go build -ldflags "-X shippin.cloud/console/internal/platform/version.version=0.4.0 -X shippin.cloud/console/internal/platform/version.channel=release"
//
// A plain build is the local development build.
var (
	version = "0.0.0-dev"
	channel = "local"
)

// Channels a build can come from (docs/release.md).
const (
	Local   = "local"   // built from a working tree
	Edge    = "edge"    // built from a commit on main
	Release = "release" // built from a vX.Y.Z tag
)

// Info describes one build. Commit is empty for a build with no version control data, such as one
// made from a source archive.
type Info struct {
	Version  string `json:"version"`
	Channel  string `json:"channel"`
	Commit   string `json:"commit"`
	BuiltAt  string `json:"built_at,omitempty"`
	Modified bool   `json:"modified,omitempty"`
}

// Short is the abbreviated commit, or "" with no commit.
func (i Info) Short() string {
	if len(i.Commit) > 7 {
		return i.Commit[:7]
	}
	return i.Commit
}

// FromBuildSettings reads the settings Go records in a binary.
func FromBuildSettings(settings []debug.BuildSetting) Info {
	var i Info
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			i.Commit = s.Value
		case "vcs.time":
			i.BuiltAt = s.Value
		case "vcs.modified":
			i.Modified = s.Value == "true"
		}
	}
	return i
}

// Current is this binary's build.
var Current = sync.OnceValue(func() Info {
	var i Info
	if bi, ok := debug.ReadBuildInfo(); ok {
		i = FromBuildSettings(bi.Settings)
	}
	i.Version, i.Channel = version, channel
	switch channel {
	case Local, Edge, Release:
	default:
		i.Channel = Local
	}
	return i
})
