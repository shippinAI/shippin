package contract

import (
	"encoding/json"
	"strings"

	"shippin.cloud/console/internal/mcpl"
)

type obj = map[string]any

// openAPI is the OpenAPI 3.1 document of every action. The hand-written api/openapi/openapi.yaml
// covers only the routes that are not actions yet; a path is in one or the other, never both.
func openAPI(m *model, ops []op) ([]byte, error) {
	paths := obj{}
	for _, o := range ops {
		method, route, _ := strings.Cut(o.g.Route, " ")
		p := strings.TrimPrefix(route, "/v1")
		item, _ := paths[p].(obj)
		if item == nil {
			item = obj{}
			paths[p] = item
		}
		item[strings.ToLower(method)] = operation(m, o)
	}
	schemas := obj{"Problem": obj{
		"type": "object", "description": "Every refusal and failure, as application/problem+json; message is plain words for a person.",
		"properties": obj{"type": obj{"type": "string"}, "title": obj{"type": "string"}, "status": obj{"type": "integer"}, "message": obj{"type": "string"}},
		"required":   []string{"status", "message"},
	}}
	for _, n := range sortedNames(m) {
		schemas[n] = schema(m.defs[n])
	}
	doc := obj{
		"openapi": "3.1.0",
		"info": obj{
			"title":   "Shippin actions",
			"version": mcpl.Version,
			"description": "Generated from internal/<domain>/<entity>/<verb>.go by cmd/mcplspec. Every operation is an MCPL action: " +
				"the panel, this API and MCP (POST /v1/mcp) run the same handler, permission check, audit entry and event. " +
				"A staff change takes a reason, kept in the audit log with it.",
		},
		"servers": []obj{{"url": "/v1"}},
		"paths":   paths,
		"components": obj{
			"schemas": schemas,
			"securitySchemes": obj{
				"session":   obj{"type": "apiKey", "in": "cookie", "name": "shippin_session", "description": "A panel sign-in."},
				"accessKey": obj{"type": "http", "scheme": "bearer", "description": "An access key (shp_...) from Account > AI access, or a sign-in token."},
			},
			"responses": obj{"Problem": obj{"description": "Refused or failed.", "content": obj{"application/problem+json": obj{"schema": ref("Problem")}}}},
		},
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	return append(b, '\n'), err
}

func operation(m *model, o op) obj {
	g := o.g
	x := obj{"id": g.ID, "access": g.Access, "risk": g.Risk, "reason": g.Reason}
	if g.Perm != "" {
		x["perm"] = g.Perm
	}
	if g.Event != "" {
		x["event"] = g.Event
	}
	op := obj{
		"operationId": g.ID,
		"summary":     g.Summary,
		"tags":        []string{g.Domain()},
		"x-mcpl":      x,
		"security":    []obj{{"session": []string{}}, {"accessKey": []string{}}},
	}
	if g.Access == mcpl.Public {
		op["security"] = []obj{}
	}
	var params []obj
	for _, p := range o.path {
		params = append(params, obj{"name": p, "in": "path", "required": true, "schema": obj{"type": "string"}})
	}
	for _, q := range o.query {
		params = append(params, obj{"name": q, "in": "query", "schema": obj{"type": "string"}})
	}
	if params != nil {
		op["parameters"] = params
	}
	if o.reason {
		reason := obj{"type": "object", "properties": obj{"reason": obj{"type": "string", "description": "Why this change is being made; kept in the audit log with it."}}}
		if g.Reason {
			reason["required"] = []string{"reason"}
		}
		body := reason
		if o.body != nil {
			body = obj{"allOf": []obj{schema(o.body), reason}}
		}
		op["requestBody"] = obj{"required": m.bodyRequired(o), "content": obj{"application/json": obj{"schema": body}}}
	}
	ok := obj{"description": "Done."}
	status := "200"
	switch {
	case o.output == nil:
		status = "204"
	case verb(g.ID) == "create":
		status = "201"
		ok = obj{"description": "Created.", "content": obj{"application/json": obj{"schema": schema(o.output)}}}
	default:
		ok["content"] = obj{"application/json": obj{"schema": schema(o.output)}}
	}
	op["responses"] = obj{status: ok, "default": obj{"$ref": "#/components/responses/Problem"}}
	return op
}

func verb(id string) string { return id[strings.LastIndex(id, ".")+1:] }

func ref(n string) obj { return obj{"$ref": "#/components/schemas/" + n} }

func schema(n *node) obj {
	switch n.kind {
	case "string":
		return obj{"type": "string"}
	case "enum":
		return obj{"type": "string", "enum": n.values}
	case "date-time":
		return obj{"type": "string", "format": "date-time"}
	case "bytes":
		return obj{"type": "string", "contentEncoding": "base64"}
	case "integer", "number", "boolean":
		return obj{"type": n.kind}
	case "array":
		return obj{"type": "array", "items": schema(n.elem)}
	case "map":
		return obj{"type": "object", "additionalProperties": schema(n.elem)}
	case "ref":
		return ref(n.ref)
	case "object":
		props, required := obj{}, []string{}
		for _, f := range n.fields {
			s := schema(f.n)
			if f.nullable {
				s = obj{"anyOf": []obj{s, {"type": "null"}}}
			}
			props[f.name] = s
			if !f.optional {
				required = append(required, f.name)
			}
		}
		out := obj{"type": "object", "properties": props, "additionalProperties": false}
		if len(required) > 0 {
			out["required"] = required
		}
		return out
	}
	return obj{}
}
