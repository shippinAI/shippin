package actions

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/mcpl/contract"
	"shippin.cloud/console/internal/mcpl/gen"
)

// The registry is generated from the action files, and the panel's typed map and the OpenAPI document
// from the registry; this fails when any is out of date, so a moved or added action can't ship without
// them.
func TestGeneratedFilesAreCurrent(t *testing.T) {
	root := filepath.Join("..", "..")
	files, err := gen.Files(root)
	if err != nil {
		t.Fatal(err)
	}
	derived, err := contract.Files(All)
	if err != nil {
		t.Fatal(err)
	}
	for p, b := range derived {
		files[p] = b
	}
	for path, want := range files {
		have, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || !bytes.Equal(have, want) {
			t.Errorf("%s is out of date: run go generate ./internal/actions", path)
		}
	}
}

// Every generated action follows the MCPL rules.
func TestEveryActionFollowsTheRules(t *testing.T) {
	var all []mcpl.Action
	for _, a := range All {
		all = append(all, a.Action)
		if a.Runner.Input().Kind().String() != "struct" {
			t.Errorf("%s: input must be a struct", a.ID)
		}
	}
	for _, err := range mcpl.Validate(all) {
		t.Error(err)
	}
	if len(All) == 0 {
		t.Fatal("no actions found")
	}
}
