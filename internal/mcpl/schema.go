package mcpl

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"
)

// Enum is a string type with a fixed set of values, which it lists. The type is the one owner of its
// values: the action checks them with Valid, and the panel's types and OpenAPI list them.
//
//	type Hours string
//	func (Hours) Values() []string { return []string{"business", "critical_always", "always"} }
type Enum interface{ Values() []string }

// Valid is whether v is one of its type's values.
func Valid[T interface {
	~string
	Enum
}](v T) bool {
	for _, x := range v.Values() {
		if x == string(v) {
			return true
		}
	}
	return false
}

// Schema is a JSON Schema, derived from an action's Go types so MCP tools, OpenAPI and the panel's
// types never drift from what the action decodes.
type Schema = map[string]any

var timeType = reflect.TypeFor[time.Time]()
var rawType = reflect.TypeFor[json.RawMessage]()

// SchemaOf is the JSON Schema of t as encoding/json reads and writes it. Fields tagged path or query
// are left out: they are parameters, not body.
func SchemaOf(t reflect.Type) Schema {
	return schemaOf(t, map[reflect.Type]bool{})
}

func schemaOf(t reflect.Type, seen map[reflect.Type]bool) Schema {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case t == timeType:
		return Schema{"type": "string", "format": "date-time"}
	case t == rawType:
		return Schema{}
	}
	if t.Kind() == reflect.String && t.Implements(reflect.TypeFor[Enum]()) {
		return Schema{"type": "string", "enum": reflect.Zero(t).Interface().(Enum).Values()}
	}
	switch t.Kind() {
	case reflect.String:
		return Schema{"type": "string"}
	case reflect.Bool:
		return Schema{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return Schema{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return Schema{"type": "number"}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return Schema{"type": "string", "contentEncoding": "base64"}
		}
		return Schema{"type": "array", "items": schemaOf(t.Elem(), seen)}
	case reflect.Map:
		return Schema{"type": "object", "additionalProperties": schemaOf(t.Elem(), seen)}
	case reflect.Struct:
		if seen[t] {
			return Schema{"type": "object"}
		}
		seen[t] = true
		defer delete(seen, t)
		props := Schema{}
		JSONFields(t, func(name string, f reflect.StructField) { props[name] = schemaOf(f.Type, seen) })
		return Schema{"type": "object", "properties": props, "additionalProperties": false}
	}
	return Schema{}
}

// JSONFields calls fn for each JSON field of struct t, flattening embedded structs as encoding/json does.
func JSONFields(t reflect.Type, fn func(name string, f reflect.StructField)) {
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" || f.Tag.Get("path") != "" || f.Tag.Get("query") != "" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if f.Anonymous && name == "" {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				JSONFields(ft, fn)
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		fn(name, f)
	}
}

// Params are the path and query parameters an input type takes, by name.
func Params(t reflect.Type) (path, query []string) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, nil
	}
	for i := range t.NumField() {
		f := t.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			p, q := Params(f.Type)
			path, query = append(path, p...), append(query, q...)
			continue
		}
		if n := f.Tag.Get("path"); n != "" {
			path = append(path, n)
		} else if n := f.Tag.Get("query"); n != "" {
			query = append(query, n)
		}
	}
	return path, query
}

// RouteParams are the {names} in a route, in order.
func RouteParams(route string) []string {
	var out []string
	for _, part := range strings.Split(route, "/") {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			out = append(out, strings.Trim(part, "{}"))
		}
	}
	return out
}

// InputSchema is everything a caller passes to run g, as one object: the route's parameters, the
// query, the body's fields and, for a staff change, the reason. MCP tools take exactly this.
func InputSchema(g Registered) Schema {
	in := g.Runner.Input()
	body := Schema{"type": "object", "properties": Schema{}}
	if g.Risk != Read {
		body = SchemaOf(in)
	}
	props := Schema{}
	for k, v := range body["properties"].(Schema) {
		props[k] = v
	}
	required := []string{}
	for _, p := range RouteParams(g.Route) {
		props[p] = Schema{"type": "string", "description": "The " + paramWords(p) + "."}
		required = append(required, p)
	}
	_, query := Params(in)
	for _, q := range query {
		props[q] = Schema{"type": "string"}
	}
	if g.Risk != Read {
		props["reason"] = Schema{"type": "string", "description": "Why this change is being made; kept in the audit log with it."}
		if g.Reason {
			required = append(required, "reason")
		}
	}
	return Schema{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

// paramWords is a route parameter in words: coverageTierId -> coverage tier ID.
func paramWords(p string) string {
	p = strings.TrimSuffix(p, "Id")
	var b strings.Builder
	for i, r := range p {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte(' ')
			}
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	if b.String() == "tenant" {
		return "organization ID"
	}
	return b.String() + " ID"
}
