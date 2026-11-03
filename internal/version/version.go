// Package version says which commit this binary was built from. Go stamps that from the checkout at
// build time, so no build step has to remember to pass it.
package version

import (
	"runtime/debug"
	"sync"
)

// Info describes one build. Commit is empty for a build with no version control data, such as one
// made from a source archive.
type Info struct {
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
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return Info{}
	}
	return FromBuildSettings(bi.Settings)
})
