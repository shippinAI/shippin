// Command mcplgen writes the MCPL action registry from the action files under internal/<domain>/.
// With -check it changes nothing and fails if the registry is out of date.
//
// mcplgen new is the factory: it starts an action where it belongs, with its test, then regenerates.
// mcplgen react starts a reaction in an existing entity, following events its actions emit.
//
//	go run ./cmd/mcplgen new tenancy/plan/list -access staff -perm catalog.plans.read -summary "List the plans."
//	go run ./cmd/mcplgen react identity/ssh_key/push_to_workspaces -on shippin.identity.ssh_key.created -summary "..."
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/mcpl/gen"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "new" {
		scaffold(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "react" {
		react(os.Args[2:])
		return
	}
	check := flag.Bool("check", false, "fail if the generated files are out of date, without writing")
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	if stale := generate(*root, *check); stale {
		os.Exit(1)
	}
}

func generate(root string, check bool) (stale bool) {
	files, err := gen.Files(root)
	if err != nil {
		fail(err)
	}
	for path, want := range files {
		full := filepath.Join(root, path)
		have, _ := os.ReadFile(full)
		if bytes.Equal(have, want) {
			continue
		}
		if check {
			fmt.Fprintln(os.Stderr, "mcplgen: out of date:", path)
			stale = true
			continue
		}
		if err := os.WriteFile(full, want, 0o644); err != nil {
			fail(err)
		}
		fmt.Println("wrote", path)
	}
	return stale
}

func scaffold(args []string) {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	access := fs.String("access", "", "who may call it: public, signed-in, staff or member")
	perm := fs.String("perm", "", `staff: the catalog permission; member: "", "manage" or "decide"`)
	summary := fs.String("summary", "", "one plain sentence, ending with a full stop, that a person or a model reads to choose it")
	singleton := fs.Bool("singleton", false, "the entity is one per parent (an organization's coverage): no ID, no list")
	collection := fs.Bool("collection", false, "a custom verb that acts on the collection, not one item")
	root := fs.String("root", ".", "repository root")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: mcplgen new <domain>/<entity>[/<child>]/<verb> -access ... [-perm ...] -summary \"...\"")
		fs.PrintDefaults()
	}
	path := ""
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		path, args = args[0], args[1:]
	}
	_ = fs.Parse(args)
	if path == "" && fs.NArg() > 0 {
		path = fs.Arg(0)
	}
	if path == "" {
		fs.Usage()
		os.Exit(2)
	}
	n := gen.New{Path: path, Collection: *collection,
		Spec: mcpl.Spec{Summary: *summary, Access: mcpl.Access(*access), Perm: *perm, Singleton: *singleton}}
	written, err := gen.Scaffold(*root, n)
	for _, f := range written {
		fmt.Println("wrote", f)
	}
	if err != nil {
		fail(err)
	}
	regenerate(*root)
	fmt.Println("\nNext: write the action and its test, then call it from the panel with callAction. See docs/mcpl.md.")
}

func react(args []string) {
	fs := flag.NewFlagSet("react", flag.ExitOnError)
	on := fs.String("on", "", "the events it follows, comma separated: shippin.<domain>.<entity>.<past verb>")
	summary := fs.String("summary", "", "one plain sentence, ending with a full stop, saying what it does")
	root := fs.String("root", ".", "repository root")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: mcplgen react <domain>/<entity>[/<child>]/<name> -on <event>[,<event>] -summary \"...\"")
		fs.PrintDefaults()
	}
	path := ""
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		path, args = args[0], args[1:]
	}
	_ = fs.Parse(args)
	if path == "" {
		fs.Usage()
		os.Exit(2)
	}
	var events []string
	for _, e := range strings.Split(*on, ",") {
		if e = strings.TrimSpace(e); e != "" {
			events = append(events, e)
		}
	}
	written, err := gen.ScaffoldReaction(*root, gen.NewReaction{Path: path, On: events, Summary: *summary})
	for _, f := range written {
		fmt.Println("wrote", f)
	}
	if err != nil {
		fail(err)
	}
	generate(*root, false)
	fmt.Println("\nNext: write the reaction and its test. See docs/mcpl.md.")
}

// regenerate writes the registry, then the typed map and OpenAPI from the compiled registry.
func regenerate(root string) {
	generate(root, false)
	spec := exec.Command("go", "run", "./cmd/mcplspec", "-root", ".")
	spec.Dir, spec.Stdout, spec.Stderr = root, os.Stdout, os.Stderr
	if err := spec.Run(); err != nil {
		fail(fmt.Errorf("the typed map and OpenAPI: %v", err))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "mcplgen:", err)
	os.Exit(1)
}
