package gen

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/template"

	"shippin.cloud/console/internal/mcpl"
)

// New is what the factory is asked for: an action at <domain>/<entity...>/<verb> and its spec.
type New struct {
	Path       string // tenancy/plan/list, or tenancy/plan/service/update for a nested entity
	Spec       mcpl.Spec
	Collection bool
}

var word = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Scaffold writes a new action that compiles and follows the rules, a test beside it that fails
// until the action is written, and the entity's file when the entity is new. It refuses to overwrite
// anything, and checks the spec against docs/mcpl.md before writing. It returns the files written.
func Scaffold(root string, n New) ([]string, error) {
	parts := strings.Split(strings.Trim(filepath.ToSlash(n.Path), "/"), "/")
	if len(parts) < 3 {
		return nil, errors.New("give <domain>/<entity>/<verb>, such as tenancy/plan/list")
	}
	for _, p := range parts {
		if !word.MatchString(p) {
			return nil, fmt.Errorf("%q: names are lower case with underscores, such as coverage_tier", p)
		}
	}
	domain, entities, verb := parts[0], parts[1:len(parts)-1], parts[len(parts)-1]
	if !slices.Contains(mcpl.Domains, domain) {
		return nil, fmt.Errorf("unknown domain %q; the domains are %s", domain, strings.Join(mcpl.Domains, ", "))
	}
	n.Spec.Collection = n.Spec.Collection || n.Collection
	id := "shippin." + strings.Join(parts, ".")
	route, err := mcpl.RouteFor(id, n.Spec)
	if err != nil {
		return nil, err
	}
	a := mcpl.Action{ID: id, Route: route, Access: n.Spec.Access, Perm: n.Spec.Perm, Risk: mcpl.RiskOf(route), Summary: n.Spec.Summary}
	if a.Risk != mcpl.Read {
		a.Event = mcpl.EventFor(id)
	}
	if errs := mcpl.Validate([]mcpl.Action{a}); len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	entity := entities[len(entities)-1]
	dir := filepath.Join(append([]string{root, "internal", domain}, entities...)...)
	d := data{ID: id, Route: route, Pkg: entity, Verb: camel(verb), T: camel(lastWord(entity)), Words: strings.ReplaceAll(entity, "_", " "),
		Spec: n.Spec, Access: map[mcpl.Access]string{mcpl.Public: "Public", mcpl.SignedIn: "SignedIn", mcpl.Staff: "Staff", mcpl.Member: "Member"}[n.Spec.Access]}
	for _, outer := range entities[:len(entities)-1] {
		d.Parents = append(d.Parents, param{Field: camel(outer) + "ID", Name: paramName(outer)})
	}
	if !n.Spec.Singleton {
		d.ItemParam = paramName(entity)
	}
	d.shape(verb, a.Risk, n.Spec)

	var written []string
	write := func(name string, tmpl *template.Template) error {
		file := filepath.Join(dir, name)
		if _, err := os.Stat(file); err == nil {
			return fmt.Errorf("%s exists; the factory never overwrites", file)
		}
		var b bytes.Buffer
		if err := tmpl.Execute(&b, d); err != nil {
			return err
		}
		src, err := format.Source(b.Bytes())
		if err != nil {
			return fmt.Errorf("%s: %v\n%s", name, err, b.Bytes())
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(file, src, 0o644); err != nil {
			return err
		}
		written = append(written, filepath.ToSlash(file))
		return nil
	}
	for _, f := range []string{verb + ".go", verb + "_db_test.go"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return nil, fmt.Errorf("%s exists; the factory never overwrites", filepath.Join(dir, f))
		}
	}
	if _, err := os.Stat(filepath.Join(dir, entity+".go")); errors.Is(err, os.ErrNotExist) {
		if err := write(entity+".go", entityTmpl); err != nil {
			return written, err
		}
	}
	if err := write(verb+".go", actionTmpl); err != nil {
		return written, err
	}
	return written, write(verb+"_db_test.go", testTmpl)
}

type param struct{ Field, Name string }

type data struct {
	ID, Route, Pkg, Verb, T, Words, Access string
	Spec                                   mcpl.Spec
	Parents                                []param
	ItemParam                              string
	// What the action takes and returns, and how its test calls it.
	In, InDecl, Out, Zero, Method, Path, Body, Status string
}

// shape picks the action's input and output from its verb, the way docs/mcpl.md describes them.
func (d *data) shape(verb string, risk mcpl.Risk, s mcpl.Spec) {
	v := mcpl.VerbOf(verb, s)
	target := "ByID" // one item
	if v.Collection || s.Singleton {
		target = "Parent"
		if len(d.Parents) == 0 {
			target = "struct{}"
		}
	}
	d.In, d.Out, d.Zero = target, d.T, d.T+"{}"
	switch {
	case verb == "list":
		d.Out, d.Zero = "mcpl.Items["+d.T+"]", "mcpl.Items["+d.T+"]{Items: []"+d.T+"{}}"
	case risk == mcpl.Destructive:
		d.Out, d.Zero = "mcpl.None", "mcpl.None{}"
	case risk == mcpl.Write:
		d.In = camel(verb) + "Input"
		embed := target
		if embed == "struct{}" {
			embed = ""
		}
		d.InDecl = fmt.Sprintf("// %s is what %s takes. Name each field once, in snake case; the decoder refuses anything else.\ntype %s struct {\n%s\n}\n", d.In, verb, d.In, embed)
	}
	method, path, _ := strings.Cut(d.Route, " ")
	d.Method = method
	d.Path = regexp.MustCompile(`\{[A-Za-z]+\}`).ReplaceAllStringFunc(path, func(p string) string { return "example-" + strings.Trim(p, "{}") })
	d.Status = "200"
	switch {
	case d.Out == "mcpl.None":
		d.Status = "204"
	case verb == "create":
		d.Status = "201"
	}
	switch {
	case risk != mcpl.Read && s.Access == mcpl.Staff:
		d.Body = `{"reason":"Testing the factory's action."}`
	case risk != mcpl.Read:
		d.Body = `{}`
	}
}

func lastWord(entity string) string { return entity[strings.LastIndex(entity, "_")+1:] }

// paramName is the route parameter for one item: coverage_tier -> coverageTierId.
func paramName(entity string) string {
	c := camel(entity)
	return strings.ToLower(c[:1]) + c[1:] + "Id"
}

var funcs = template.FuncMap{"q": func(s string) string { return fmt.Sprintf("%q", s) }}

var entityTmpl = template.Must(template.New("entity").Funcs(funcs).Parse(`// Package {{.Pkg}} is TODO: say in one or two sentences what a {{.Words}} is, who changes it and who
// reads it. Its actions are the other files in this folder; see docs/mcpl.md.
package {{.Pkg}}

import "time"

// {{.T}} is one {{.Words}}, as its actions return it.
type {{.T}} struct {
	ID        string    ` + "`json:\"id\"`" + `
	CreatedAt time.Time ` + "`json:\"created_at\"`" + `
}
{{if .Parents}}
// Parent names what a {{.Words}} belongs to, from the route.
type Parent struct {
{{- range .Parents}}
	{{.Field}} string ` + "`path:\"{{.Name}}\" json:\"-\"`" + `
{{- end}}
}
{{end}}{{if .ItemParam}}
// ByID names one {{.Words}} from the route.
type ByID struct {
{{- if .Parents}}
	Parent
{{- end}}
	ID string ` + "`path:\"{{.ItemParam}}\" json:\"-\"`" + `
}
{{end}}`))

var actionTmpl = template.Must(template.New("action").Funcs(funcs).Parse(`package {{.Pkg}}

import "shippin.cloud/console/internal/mcpl"

{{.InDecl}}
// {{.Verb}} is {{.ID}}: {{.Route}}.
var {{.Verb}} = mcpl.Def[{{.In}}, {{.Out}}]{
	Spec: mcpl.Spec{Summary: {{q .Spec.Summary}}, Access: mcpl.{{.Access}}{{if .Spec.Perm}}, Perm: {{q .Spec.Perm}}{{end}}{{if .Spec.Singleton}}, Singleton: true{{end}}{{if .Spec.Collection}}, Collection: true{{end}}},
	Run: func(c *mcpl.Call, in {{.In}}) ({{.Out}}, error) {
		// c.DB is this call's database (one transaction for a change), c.Actor who it runs as{{if eq .Access "Member"}},
		// c.Tenant the organization{{end}}. Refuse with mcpl.Invalid, Forbidden, NotFound or Conflict, in plain words.
		return {{.Zero}}, mcpl.NotWritten({{q .ID}})
	},
}
`))

var testTmpl = template.Must(template.New("test").Funcs(funcs).Parse(`package {{.Pkg}}_test

import (
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl/mcpltest"
)

// Test{{.Verb}} runs {{.ID}} on its route against a fresh database. Add what it changed, who is
// refused, and its audit entry and event (h.Audits, h.Events).
func Test{{.Verb}}(t *testing.T) {
	h := mcpltest.Serve(t, actions.All, {{q .ID}}, mcpltest.As{Actor: "ada"{{if eq .Access "Member"}}, Tenant: "example-tenantId"{{end}}})
	h.Call({{q .Method}}, {{q .Path}}, {{q .Body}}, {{.Status}})
}
`))
