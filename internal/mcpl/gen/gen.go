// Package gen finds every MCPL action by where it is and writes the registry and the panel's action
// map from them. Run it with `go generate ./internal/actions`; internal/actions fails its tests when
// the generated files are out of date.
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

// Find walks internal/<domain>/... under root for action files. A file named <verb>.go that declares
// `var <Verb> = mcpl.Def[...]{...}` is an action with ID shippin.<domain>.<entity dirs...>.<verb>.
func Find(root string) ([]Found, error) {
	var out []Found
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
			found, err := inspect(root, base, domain, path)
			if err != nil {
				return err
			}
			out = append(out, found...)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	slices.SortFunc(out, func(a, b Found) int { return strings.Compare(a.ID, b.ID) })
	for i := 1; i < len(out); i++ {
		if out[i].ID == out[i-1].ID {
			return nil, fmt.Errorf("%s is declared twice (%s, %s)", out[i].ID, out[i-1].File, out[i].File)
		}
	}
	return out, nil
}

func inspect(root, base, domain, path string) ([]Found, error) {
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, err
	}
	rel, _ := filepath.Rel(root, path)
	rel = filepath.ToSlash(rel)
	var out []Found
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
			lit, ok := vs.Values[0].(*ast.CompositeLit)
			if !ok || !isDef(lit.Type) {
				continue
			}
			verb := strings.TrimSuffix(filepath.Base(path), ".go")
			if want := camel(verb); vs.Names[0].Name != want {
				return nil, fmt.Errorf("%s: the action in %s.go is named %s, not %s", rel, verb, want, vs.Names[0].Name)
			}
			entDir, _ := filepath.Rel(base, filepath.Dir(path))
			entities := strings.Split(filepath.ToSlash(entDir), "/")
			if entDir == "." {
				return nil, fmt.Errorf("%s: an action sits in an entity folder, internal/%s/<entity>/%s.go", rel, domain, verb)
			}
			spec, err := specOf(lit)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", rel, err)
			}
			id := "shippin." + domain + "." + strings.Join(entities, ".") + "." + verb
			route, err := mcpl.RouteFor(id, spec)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", rel, err)
			}
			dir, _ := filepath.Rel(root, filepath.Dir(path))
			out = append(out, Found{ID: id, Dir: filepath.ToSlash(dir), Var: vs.Names[0].Name, Spec: spec, Route: route, File: rel, Package: f.Name.Name})
		}
	}
	return out, nil
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
	sel, ok := t.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "mcpl" && sel.Sel.Name == "Def"
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

// Go is the registry: every action, registered under its derived ID.
func Go(found []Found) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by cmd/mcplgen from internal/<domain>/<entity>/<verb>.go. DO NOT EDIT.\n\npackage actions\n\nimport (\n\t\"shippin.cloud/console/internal/mcpl\"\n")
	aliases := map[string]string{}
	var dirs []string
	for _, f := range found {
		if _, ok := aliases[f.Dir]; !ok {
			aliases[f.Dir] = strings.ReplaceAll(strings.TrimPrefix(f.Dir, "internal/"), "/", "_")
			dirs = append(dirs, f.Dir)
		}
	}
	slices.Sort(dirs)
	for _, d := range dirs {
		fmt.Fprintf(&b, "\t%s %q\n", aliases[d], Module+"/"+d)
	}
	b.WriteString(")\n\n// All is every action, in ID order.\nvar All = []mcpl.Registered{\n")
	for _, f := range found {
		fmt.Fprintf(&b, "\tmcpl.Register(%q, %s.%s),\n", f.ID, aliases[f.Dir], f.Var)
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}

// TS is the panel's action map: the same IDs with their method, path and rules.
func TS(found []Found) []byte {
	var b bytes.Buffer
	b.WriteString("// Code generated by cmd/mcplgen from internal/<domain>/<entity>/<verb>.go. DO NOT EDIT.\n\nexport const actions = {\n")
	for _, f := range found {
		method, path, _ := strings.Cut(f.Route, " ")
		risk := mcpl.RiskOf(f.Route)
		reason := f.Spec.Access == mcpl.Staff && risk != mcpl.Read
		fmt.Fprintf(&b, "  %q: { method: %q, path: %q, access: %q, perm: %q, risk: %q, reason: %t, summary: %q },\n",
			f.ID, method, path, f.Spec.Access, f.Spec.Perm, risk, reason, f.Spec.Summary)
	}
	b.WriteString("} as const;\n\nexport type ActionId = keyof typeof actions;\n")
	return b.Bytes()
}

// Files are the generated files, by module-relative path.
func Files(root string) (map[string][]byte, error) {
	found, err := Find(root)
	if err != nil {
		return nil, err
	}
	goSrc, err := Go(found)
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		"internal/actions/registry_gen.go": goSrc,
		"web/src/lib/api/actions.gen.ts":   TS(found),
	}, nil
}
