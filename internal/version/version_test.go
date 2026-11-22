package version

import (
	"runtime/debug"
	"testing"
)

func TestFromBuildSettings(t *testing.T) {
	got := FromBuildSettings([]debug.BuildSetting{
		{Key: "vcs.revision", Value: "abcdef0123456789"},
		{Key: "vcs.time", Value: "2026-11-03T00:00:00Z"},
		{Key: "vcs.modified", Value: "true"},
		{Key: "GOOS", Value: "linux"},
	})
	if got.Commit != "abcdef0123456789" || got.BuiltAt != "2026-11-03T00:00:00Z" || !got.Modified {
		t.Fatalf("%+v", got)
	}
	if got.Short() != "abcdef0" {
		t.Fatalf("short %q", got.Short())
	}
	if none := FromBuildSettings(nil); none.Commit != "" || none.Short() != "" {
		t.Fatalf("no stamp: %+v", none)
	}
}

// A plain build, like this test binary, is the local development build; a build sets both with -ldflags.
func TestCurrentIsTheLocalBuildUnlessTheBuildSaysOtherwise(t *testing.T) {
	if got := Current(); got.Version != "0.0.0-dev" || got.Channel != Local {
		t.Fatalf("%+v", got)
	}
}
