// Package contract derives everything outside Go from the compiled action registry: the panel's typed
// action map and the OpenAPI document. The types come from each action's In and Out, walked once, so
// TypeScript, OpenAPI and the server never disagree. cmd/mcplspec writes the files, internal/actions
// fails its tests when they are stale, and the image build regenerates them from source.
package contract

import (
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode"

	"shippin.cloud/console/internal/mcpl"
)

// Files are the generated files, by repository-relative path.
func Files(all []mcpl.Registered) (map[string][]byte, error) {
	m := newModel()
	ops := make([]op, 0, len(all))
	for _, g := range all {
		ops = append(ops, m.op(g))
	}
	if m.err != nil {
		return nil, m.err
	}
	api, err := openAPI(m, ops)
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		"web/src/lib/api/actions.gen.ts": typeScript(m, ops),
		"api/openapi/actions.gen.json":   api,
	}, nil
}

// node is one JSON shape.
type node struct {
	kind   string   // string, enum, date-time, bytes, integer, number, boolean, any, array, map, object, ref
	ref    string   // a named struct or enum, by its contract name
	values []string // enum
	elem   *node    // array and map
	fields []field
}

type field struct {
	name     string
	n        *node
	optional bool // omitempty: may be absent
	nullable bool // a pointer: may be null
}

// required is whether the object has a field a caller must send.
func (n *node) required() bool {
	return n.kind == "object" && slices.ContainsFunc(n.fields, func(f field) bool { return !f.optional })
}

var (
	timeType = reflect.TypeFor[time.Time]()
	rawType  = reflect.TypeFor[json.RawMessage]()
	noneType = reflect.TypeFor[mcpl.None]()
)

// model names every named struct the actions use, once.
type model struct {
	names  map[reflect.Type]string
	byName map[string]reflect.Type
	defs   map[string]*node
	err    error
}

func newModel() *model {
	return &model{names: map[reflect.Type]string{}, byName: map[string]reflect.Type{}, defs: map[string]*node{}}
}

// name is a named struct's contract name: its package's entity in Pascal case, then the type name
// unless the entity already ends with it. ssh_key.Key is SshKey, access_key.Created AccessKeyCreated.
func name(t reflect.Type) string {
	pkg := pascal(path.Base(t.PkgPath()))
	if strings.HasSuffix(pkg, t.Name()) {
		return pkg
	}
	return pkg + t.Name()
}

func pascal(s string) string {
	var b strings.Builder
	for _, w := range strings.Split(s, "_") {
		if w != "" {
			r := []rune(w)
			b.WriteRune(unicode.ToUpper(r[0]))
			b.WriteString(string(r[1:]))
		}
	}
	return b.String()
}

var enumType = reflect.TypeFor[mcpl.Enum]()

// enumOf is the values of a string type that lists them with Values() []string (mcpl.Enum).
func enumOf(t reflect.Type) []string {
	if t.Kind() != reflect.String || t.Name() == "" || !t.Implements(enumType) {
		return nil
	}
	return reflect.Zero(t).Interface().(mcpl.Enum).Values()
}

func named(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && t.Name() != "" && !strings.Contains(t.Name(), "[") && t.PkgPath() != "" && t != timeType && t != noneType
}

func (m *model) of(t reflect.Type) *node {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case t == timeType:
		return &node{kind: "date-time"}
	case t == rawType:
		return &node{kind: "any"}
	}
	if vs := enumOf(t); vs != nil {
		return m.define(t, func() *node { return &node{kind: "enum", values: vs} })
	}
	switch t.Kind() {
	case reflect.String:
		return &node{kind: "string"}
	case reflect.Bool:
		return &node{kind: "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &node{kind: "integer"}
	case reflect.Float32, reflect.Float64:
		return &node{kind: "number"}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return &node{kind: "bytes"}
		}
		return &node{kind: "array", elem: m.of(t.Elem())}
	case reflect.Map:
		return &node{kind: "map", elem: m.of(t.Elem())}
	case reflect.Struct:
		if !named(t) {
			return m.object(t)
		}
		return m.define(t, func() *node { return m.object(t) })
	}
	return &node{kind: "any"}
}

// define names t once and returns a reference to it.
func (m *model) define(t reflect.Type, body func() *node) *node {
	if n, ok := m.names[t]; ok {
		return &node{kind: "ref", ref: n}
	}
	n := name(t)
	if other, ok := m.byName[n]; ok && other != t {
		m.err = fmt.Errorf("contract: %s and %s are both named %s; rename one", other, t, n)
		return &node{kind: "any"}
	}
	m.names[t], m.byName[n] = n, t
	m.defs[n] = body()
	return &node{kind: "ref", ref: n}
}

func (m *model) object(t reflect.Type) *node {
	o := &node{kind: "object"}
	mcpl.JSONFields(t, func(name string, f reflect.StructField) {
		_, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		o.fields = append(o.fields, field{name: name, n: m.of(f.Type), optional: slices.Contains(strings.Split(opts, ","), "omitempty"), nullable: f.Type.Kind() == reflect.Pointer})
	})
	return o
}

// op is one action with its shapes.
type op struct {
	g      mcpl.Registered
	path   []string // route parameters, in order
	query  []string
	body   *node // nil for a read
	reason bool  // the body takes a reason; required when g.Reason
	output *node // nil for mcpl.None
}

func (m *model) op(g mcpl.Registered) op {
	o := op{g: g, path: mcpl.RouteParams(g.Route)}
	in, out := g.Runner.Input(), g.Runner.Output()
	_, o.query = mcpl.Params(in)
	if g.Risk != mcpl.Read {
		o.reason = true
		if hasFields(in) {
			o.body = m.of(in)
		}
	}
	if out != noneType {
		o.output = m.of(out)
	}
	return o
}

// hasFields is whether t carries anything in a JSON body; ByID, whose ID is in the path, does not.
func hasFields(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return true
	}
	n := 0
	mcpl.JSONFields(t, func(string, reflect.StructField) { n++ })
	return n > 0
}

// bodyRequired is whether a caller must send a body: a staff change's reason, or a required field.
func (m *model) bodyRequired(o op) bool {
	if o.g.Reason {
		return true
	}
	if o.body == nil {
		return false
	}
	if o.body.kind == "ref" {
		return m.defs[o.body.ref].required()
	}
	return o.body.required()
}

func sortedNames(m *model) []string {
	out := make([]string, 0, len(m.defs))
	for n := range m.defs {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}
