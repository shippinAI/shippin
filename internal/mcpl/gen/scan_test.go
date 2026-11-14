package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shippin.cloud/console/internal/mcpl"
)

func write(t *testing.T, root, path, src string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A reaction and an entity's services are found where they are, like actions, and the registry
// lists them; a reaction to an event no action emits stops generation.
func TestScanFindsReactionsAndServices(t *testing.T) {
	root := t.TempDir()
	if _, err := Scaffold(root, New{Path: "identity/key/create", Spec: mcpl.Spec{Summary: "Add a key.", Access: mcpl.SignedIn}}); err != nil {
		t.Fatal(err)
	}
	files, err := ScaffoldReaction(root, NewReaction{Path: "identity/key/push", On: []string{"shippin.identity.key.created"}, Summary: "Push the keys."})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || !strings.HasSuffix(files[1], "push_db_test.go") {
		t.Fatalf("written: %v", files)
	}
	write(t, root, "internal/identity/key/services.go", "package key\n\nimport \"shippin.cloud/console/internal/mcpl\"\n\nvar Services = mcpl.Provider(func(s mcpl.Services) {})\n")

	tree, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Reactions) != 1 || tree.Reactions[0].ID != "shippin.identity.key.push" || tree.Reactions[0].On[0] != "shippin.identity.key.created" {
		t.Fatalf("reactions: %+v", tree.Reactions)
	}
	if len(tree.Providers) != 1 || tree.Providers[0].Dir != "internal/identity/key" {
		t.Fatalf("providers: %+v", tree.Providers)
	}
	src, err := Go(tree)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`mcpl.React("shippin.identity.key.push", identity_key.Push)`, "identity_key.Services,"} {
		if !strings.Contains(string(src), want) {
			t.Errorf("registry lacks %s:\n%s", want, src)
		}
	}
	if _, err := ScaffoldReaction(root, NewReaction{Path: "identity/key/push", On: []string{"shippin.identity.key.created"}, Summary: "Push the keys."}); err == nil {
		t.Error("the factory overwrote a reaction")
	}

	for name, src := range map[string]string{
		"unknown event": "package key\n\nimport \"shippin.cloud/console/internal/mcpl\"\n\nvar Pull = mcpl.Reaction{On: []string{\"shippin.identity.key.renamed\"}, Summary: \"Pull.\"}\n",
		"misnamed":      "package key\n\nimport \"shippin.cloud/console/internal/mcpl\"\n\nvar Other = mcpl.Reaction{On: []string{\"shippin.identity.key.created\"}, Summary: \"Pull.\"}\n",
		"not literal":   "package key\n\nimport \"shippin.cloud/console/internal/mcpl\"\n\nvar events = []string{}\n\nvar Pull = mcpl.Reaction{On: events, Summary: \"Pull.\"}\n",
	} {
		write(t, root, "internal/identity/key/pull.go", src)
		if _, err := Scan(root); err == nil {
			t.Errorf("%s: generation went ahead", name)
		}
	}
}

// The reaction factory refuses what the rules refuse, before writing anything.
func TestScaffoldReactionRefuses(t *testing.T) {
	root := t.TempDir()
	if _, err := Scaffold(root, New{Path: "identity/key/list", Spec: mcpl.Spec{Summary: "List keys.", Access: mcpl.SignedIn}}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadDir(filepath.Join(root, "internal", "identity", "key"))
	for _, n := range []NewReaction{
		{Path: "identity/key/push", On: []string{"shippin.identity.key.listed"}, Summary: "Push."}, // a read emits nothing
		{Path: "identity/key/push", Summary: "Push."},
		{Path: "identity/key/push", On: []string{"shippin.identity.key.created"}, Summary: "no stop"},
		{Path: "identity/nothing/push", On: []string{"shippin.identity.key.created"}, Summary: "Push."},
		{Path: "billing/key/push", On: []string{"shippin.identity.key.created"}, Summary: "Push."},
		{Path: "identity/key/list", On: []string{"shippin.identity.key.created"}, Summary: "Push."},
	} {
		if _, err := ScaffoldReaction(root, n); err == nil {
			t.Errorf("%+v was accepted", n)
		}
	}
	if after, _ := os.ReadDir(filepath.Join(root, "internal", "identity", "key")); len(after) != len(before) {
		t.Errorf("a refused reaction wrote files: %d before, %d after", len(before), len(after))
	}
}
