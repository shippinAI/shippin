package gen

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"shippin.cloud/console/internal/mcpl"
)

// NewReaction is a reaction the factory is asked for: <domain>/<entity...>/<name> and what it follows.
type NewReaction struct {
	Path    string // identity/ssh_key/push_to_workspaces
	On      []string
	Summary string
}

// ScaffoldReaction writes a reaction in an existing entity's folder, following events the actions
// under root emit, and a test beside it that fails until the reaction is written. It checks
// everything before writing and never overwrites. It returns the files written.
func ScaffoldReaction(root string, n NewReaction) ([]string, error) {
	parts := strings.Split(strings.Trim(filepath.ToSlash(n.Path), "/"), "/")
	if len(parts) < 3 {
		return nil, errors.New("give <domain>/<entity>/<name>, such as identity/ssh_key/push_to_workspaces")
	}
	for _, p := range parts {
		if !word.MatchString(p) {
			return nil, fmt.Errorf("%q: names are lower case with underscores, such as push_to_workspaces", p)
		}
	}
	domain, entities, name := parts[0], parts[1:len(parts)-1], parts[len(parts)-1]
	if !slices.Contains(mcpl.Domains, domain) {
		return nil, fmt.Errorf("unknown domain %q; the domains are %s", domain, strings.Join(mcpl.Domains, ", "))
	}
	dir := filepath.Join(append([]string{root, "internal", domain}, entities...)...)
	if src, _ := filepath.Glob(filepath.Join(dir, "*.go")); len(src) == 0 {
		return nil, fmt.Errorf("%s has no entity yet; a reaction belongs to an entity, so start one with mcplgen new", filepath.ToSlash(dir))
	}
	t, err := Scan(root)
	if err != nil {
		return nil, err
	}
	var acts []mcpl.Registered
	for _, a := range t.Actions {
		x := mcpl.Action{ID: a.ID}
		if verb := a.ID[strings.LastIndex(a.ID, ".")+1:]; mcpl.VerbOf(verb, a.Spec).Risk != mcpl.Read {
			x.Event = mcpl.EventFor(a.ID)
		}
		acts = append(acts, mcpl.Registered{Action: x})
	}
	for _, r := range t.Reactions {
		acts = append(acts, mcpl.Registered{Action: mcpl.Action{ID: r.ID}})
	}
	id := "shippin." + strings.Join(parts, ".")
	r := mcpl.React(id, mcpl.Reaction{On: n.On, Summary: n.Summary, Run: func(*mcpl.Call, mcpl.Event) error { return nil }})
	if errs := mcpl.ValidateReactions([]mcpl.RegisteredReaction{r}, acts); len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	d := reactionData{ID: id, Pkg: entities[len(entities)-1], Var: camel(name), On: n.On, Summary: n.Summary,
		Import: Module + "/" + filepath.ToSlash(filepath.Join(append([]string{"internal", domain}, entities...)...))}
	files := map[string]*template.Template{name + ".go": reactionTmpl, name + "_db_test.go": reactionTestTmpl}
	for f := range files {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return nil, fmt.Errorf("%s exists; the factory never overwrites", filepath.ToSlash(filepath.Join(dir, f)))
		}
	}
	var written []string
	for _, f := range []string{name + ".go", name + "_db_test.go"} {
		var b bytes.Buffer
		if err := files[f].Execute(&b, d); err != nil {
			return written, err
		}
		src, err := format.Source(b.Bytes())
		if err != nil {
			return written, fmt.Errorf("%s: %v\n%s", f, err, b.Bytes())
		}
		if err := os.WriteFile(filepath.Join(dir, f), src, 0o644); err != nil {
			return written, err
		}
		written = append(written, filepath.ToSlash(filepath.Join(dir, f)))
	}
	return written, nil
}

type reactionData struct {
	ID, Pkg, Var, Summary, Import string
	On                            []string
}

var reactionTmpl = template.Must(template.New("reaction").Funcs(funcs).Parse(`package {{.Pkg}}

import "shippin.cloud/console/internal/mcpl"

// {{.Var}} is {{.ID}}. TODO: say what it does and why, in a sentence a newcomer understands.
var {{.Var}} = mcpl.Reaction{
	On:      []string{ {{- range $i, $e := .On}}{{if $i}}, {{end}}{{q $e}}{{end -}} },
	Summary: {{q .Summary}},
	Run: func(c *mcpl.Call, e mcpl.Event) error {
		// It runs after the change is committed: c.Pool is the database, e.Actor who acted, e.Tenant the
		// organization, e.Subject the IDs acted on. What only the panel has (its sealer, the workspace
		// runtime) comes from mcpl.Need[ports.X](c); see internal/ports.
		return mcpl.NotWritten({{q .ID}})
	},
}
`))

var reactionTestTmpl = template.Must(template.New("reactionTest").Funcs(funcs).Parse(`package {{.Pkg}}_test

import (
	"context"
	"testing"

	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/mcpl/mcpltest"
	{{q .Import}}
)

// Test{{.Var}} runs {{.ID}} on a fresh database. Check what it did. To run it after the real
// action instead, serve the actions with mcpltest.Serve and call h.React(actions.Reactions).
func Test{{.Var}}(t *testing.T) {
	c := &mcpl.Call{Ctx: context.Background(), Pool: mcpltest.DB(t), Actor: "ada", Via: "reaction", Services: mcpl.Services{}}
	if err := {{.Pkg}}.{{.Var}}.Run(c, mcpl.Event{ID: {{q (index .On 0)}}, Actor: "ada"}); err != nil {
		t.Fatal(err)
	}
}
`))
