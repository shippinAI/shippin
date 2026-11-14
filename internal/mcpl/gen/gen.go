// Package gen finds every MCPL action, reaction and entity service provider by where it is and
// writes the registry from them, reading the source only, so it runs even when the registry it replaces no longer compiles. Everything else
// (the panel's typed map, OpenAPI) is derived from the compiled registry by package contract. Run both
// with `go generate ./internal/actions`; internal/actions fails its tests when either is stale.
package gen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"shippin.cloud/console/internal/mcpl"
)

// Module is the Go module path of the console.
const Module = "shippin.cloud/console"

// Found is one action file.
type Found struct {
	ID      string
	Dir     string // module-relative package directory, e.g. internal/tenancy/coverage_tier
	Var     string // the exported Def, e.g. Update
	Spec    mcpl.Spec
	Route   string
	File    string
	Package string
}

// FoundReaction is one reaction file.
type FoundReaction struct {
	ID   string
	Dir  string
	Var  string
	On   []string
	File string
}

// FoundProvider is an entity package's `var Services = mcpl.Provider(...)`.
type FoundProvider struct {
	Dir  string
	File string
}

// Tree is everything found under internal/<domain>/.
type Tree struct {
	Actions   []Found
	Reactions []FoundReaction
	Providers []FoundProvider
}

// Find is the actions of Scan.
func Find(root string) ([]Found, error) {
	t, err := Scan(root)
	return t.Actions, err
}

// Scan walks internal/<domain>/... under root. A file named <verb>.go that declares
// `var <Verb> = mcpl.Def[...]{...}` is an action with ID shippin.<domain>.<entity dirs...>.<verb>; a
// file named <name>.go that declares `var <Name> = mcpl.Reaction{...}` is a reaction with ID
// shippin.<domain>.<entity dirs...>.<name>; `var Services = mcpl.Provider(...)` is the entity's own
// services. A reaction must follow events that the actions found here emit.
func Scan(root string) (Tree, error) {
	var t Tree
	for _, domain := range mcpl.Domains {
		base := filepath.Join(root, "internal", domain)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
				return nil
			}
			return inspect(root, base, domain, path, &t)
		})
		if err != nil {
			return t, err
		}
	}
	out := t.Actions
	slices.SortFunc(out, func(a, b Found) int { return strings.Compare(a.ID, b.ID) })
	for i := 1; i < len(out); i++ {
		if out[i].ID == out[i-1].ID {
			return t, fmt.Errorf("%s is declared twice (%s, %s)", out[i].ID, out[i-1].File, out[i].File)
		}
	}
	slices.SortFunc(t.Reactions, func(a, b FoundReaction) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(t.Providers, func(a, b FoundProvider) int { return strings.Compare(a.Dir, b.Dir) })
	events, files := map[string]bool{}, map[string]string{}
	for _, a := range out {
		files[a.ID] = a.File
		if verb := a.ID[strings.LastIndex(a.ID, ".")+1:]; mcpl.VerbOf(verb, a.Spec).Risk != mcpl.Read {
			events[mcpl.EventFor(a.ID)] = true
		}
	}
	for _, r := range t.Reactions {
		if f, ok := files[r.ID]; ok {
			return t, fmt.Errorf("%s is declared twice (%s, %s)", r.ID, f, r.File)
		}
		files[r.ID] = r.File
		if len(r.On) == 0 {
			return t, fmt.Errorf("%s: a reaction follows at least one event: On: []string{\"shippin.<domain>.<entity>.<past verb>\"}", r.File)
		}
		for _, e := range r.On {
			if !events[e] {
				return t, fmt.Errorf("%s: %s is not emitted by any action (an event is a change's ID in the past tense)", r.File, e)
			}
		}
	}
	return t, nil
}

func inspect(root, base, domain, path string, t *Tree) error {
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return err
	}
	rel, _ := filepath.Rel(root, path)
	rel = filepath.ToSlash(rel)
	dir, _ := filepath.Rel(root, filepath.Dir(path))
	dir = filepath.ToSlash(dir)
	entDir, _ := filepath.Rel(base, filepath.Dir(path))
	entities := strings.Split(filepath.ToSlash(entDir), "/")
	name := strings.TrimSuffix(filepath.Base(path), ".go")
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, sp := range gd.Specs {
			vs := sp.(*ast.ValueSpec)
			if len(vs.Names) != 1 || len(vs.Values) != 1 {
				continue
			}
			if call, ok := vs.Values[0].(*ast.CallExpr); ok && isMCPL(call.Fun, "Provider") {
				if vs.Names[0].Name != "Services" || entDir == "." {
					return fmt.Errorf("%s: an entity's services are `var Services = mcpl.Provider(...)` in its folder", rel)
				}
				t.Providers = append(t.Providers, FoundProvider{Dir: dir, File: rel})
				continue
			}
			lit, ok := vs.Values[0].(*ast.CompositeLit)
			if !ok {
				continue
			}
			kind := ""
			switch {
			case isDef(lit.Type):
				kind = "action"
			case isMCPL(lit.Type, "Reaction"):
				kind = "reaction"
			default:
				continue
			}
			if want := camel(name); vs.Names[0].Name != want {
				return fmt.Errorf("%s: the %s in %s.go is named %s, not %s", rel, kind, name, want, vs.Names[0].Name)
			}
			if entDir == "." {
				return fmt.Errorf("%s: a %s sits in an entity folder, internal/%s/<entity>/%s.go", rel, kind, domain, name)
			}
			id := "shippin." + domain + "." + strings.Join(entities, ".") + "." + name
			if kind == "reaction" {
				on, err := onOf(lit)
				if err != nil {
					return fmt.Errorf("%s: %v", rel, err)
				}
				t.Reactions = append(t.Reactions, FoundReaction{ID: id, Dir: dir, Var: vs.Names[0].Name, On: on, File: rel})
				continue
			}
			spec, err := specOf(lit)
			if err != nil {
				return fmt.Errorf("%s: %v", rel, err)
			}
			route, err := mcpl.RouteFor(id, spec)
			if err != nil {
				return fmt.Errorf("%s: %v", rel, err)
			}
			t.Actions = append(t.Actions, Found{ID: id, Dir: dir, Var: vs.Names[0].Name, Spec: spec, Route: route, File: rel, Package: f.Name.Name})
		}
	}
	return nil
}

// onOf reads On: []string{...} from a reaction. Its events must be string literals, so they can be
// checked here against the actions.
func onOf(lit *ast.CompositeLit) ([]string, error) {
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok || ident(kv.Key) != "On" {
			continue
		}
		list, ok := kv.Value.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("On must be written out as []string{...}")
		}
		var on []string
		for _, e := range list.Elts {
			bl, ok := e.(*ast.BasicLit)
			if !ok || bl.Kind != token.STRING {
				return nil, fmt.Errorf("On lists events as string literals")
			}
			v, _ := strconv.Unquote(bl.Value)
			on = append(on, v)
		}
		return on, nil
	}
	return nil, nil
}

// isMCPL reports whether e is mcpl.<name>.
func isMCPL(e ast.Expr, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "mcpl" && sel.Sel.Name == name
}

func isDef(t ast.Expr) bool {
	switch x := t.(type) {
	case *ast.IndexListExpr:
		t = x.X
	case *ast.IndexExpr:
		t = x.X
	default:
		return false
	}
	return isMCPL(t, "Def")
}

// specOf reads Spec: mcpl.Spec{...} from the literal. Its values must be literals, so they can be read here.
func specOf(lit *ast.CompositeLit) (mcpl.Spec, error) {
	var s mcpl.Spec
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok || ident(kv.Key) != "Spec" {
			continue
		}
		inner, ok := kv.Value.(*ast.CompositeLit)
		if !ok {
			return s, fmt.Errorf("Spec must be written out as mcpl.Spec{...}")
		}
		for _, f := range inner.Elts {
			fkv, ok := f.(*ast.KeyValueExpr)
			if !ok {
				return s, fmt.Errorf("Spec fields are named")
			}
			switch ident(fkv.Key) {
			case "Summary", "Perm":
				bl, ok := fkv.Value.(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					return s, fmt.Errorf("Spec.%s must be a string literal", ident(fkv.Key))
				}
				v, _ := strconv.Unquote(bl.Value)
				if ident(fkv.Key) == "Summary" {
					s.Summary = v
				} else {
					s.Perm = v
				}
			case "Access":
				sel, ok := fkv.Value.(*ast.SelectorExpr)
				if !ok {
					return s, fmt.Errorf("Spec.Access must be mcpl.Public, mcpl.SignedIn, mcpl.Staff or mcpl.Member")
				}
				s.Access = map[string]mcpl.Access{"Public": mcpl.Public, "SignedIn": mcpl.SignedIn, "Staff": mcpl.Staff, "Member": mcpl.Member}[sel.Sel.Name]
			case "Collection":
				s.Collection = ident(fkv.Value) == "true"
			case "Singleton":
				s.Singleton = ident(fkv.Value) == "true"
			}
		}
		return s, nil
	}
	return s, fmt.Errorf("an action declares Spec: mcpl.Spec{...}")
}

func ident(e ast.Expr) string {
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// camel is the exported Go name of a verb: take_over -> TakeOver.
func camel(verb string) string {
	var b strings.Builder
	for _, w := range strings.Split(verb, "_") {
		if w != "" {
			r := []rune(w)
			b.WriteRune(unicode.ToUpper(r[0]))
			b.WriteString(string(r[1:]))
		}
	}
	return b.String()
}

// Go is the registry: every action and reaction under its derived ID, and every entity's services.
func Go(t Tree) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by cmd/mcplgen from internal/<domain>/<entity>/. DO NOT EDIT.\n\npackage actions\n\nimport (\n\t\"shippin.cloud/console/internal/mcpl\"\n")
	aliases := map[string]string{}
	var dirs []string
	use := func(dir string) {
		if _, ok := aliases[dir]; !ok {
			aliases[dir] = strings.ReplaceAll(strings.TrimPrefix(dir, "internal/"), "/", "_")
			dirs = append(dirs, dir)
		}
	}
	found := t.Actions
	for _, f := range found {
		use(f.Dir)
	}
	for _, r := range t.Reactions {
		use(r.Dir)
	}
	for _, p := range t.Providers {
		use(p.Dir)
	}
	slices.Sort(dirs)
	for _, d := range dirs {
		fmt.Fprintf(&b, "\t%s %q\n", aliases[d], Module+"/"+d)
	}
	b.WriteString(")\n\n// All is every action, in ID order.\nvar All = []mcpl.Registered{\n")
	for _, f := range found {
		fmt.Fprintf(&b, "\tmcpl.Register(%q, %s.%s),\n", f.ID, aliases[f.Dir], f.Var)
	}
	b.WriteString("}\n\n// Reactions is every reaction, in ID order.\nvar Reactions = []mcpl.RegisteredReaction{\n")
	for _, r := range t.Reactions {
		fmt.Fprintf(&b, "\tmcpl.React(%q, %s.%s),\n", r.ID, aliases[r.Dir], r.Var)
	}
	b.WriteString("}\n\n// Providers are the entities' own services, by package.\nvar Providers = []mcpl.Provider{\n")
	for _, p := range t.Providers {
		fmt.Fprintf(&b, "\t%s.Services,\n", aliases[p.Dir])
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}

// Files are the generated files, by module-relative path.
func Files(root string) (map[string][]byte, error) {
	t, err := Scan(root)
	if err != nil {
		return nil, err
	}
	goSrc, err := Go(t)
	if err != nil {
		return nil, err
	}
	return map[string][]byte{"internal/actions/registry_gen.go": goSrc}, nil
}
