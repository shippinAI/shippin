package mcpl

import (
	"errors"
	"slices"
	"strings"
)

// RouteFor derives an action's route from its ID and spec. The rules, in order:
//
//   - The audience gives the prefix: staff /v1/operator, member /v1/tenants/{tenantId}, signed-in
//     /v1/me, public /v1. Then the domain.
//   - Each entity is its plural in kebab case; an outer entity is followed by its own {<entity>Id}.
//   - list GET the collection; create POST it; get GET, update PUT and delete/remove/revoke DELETE one
//     item; any other verb POSTs to the item, or to the collection when the spec says Collection.
//   - A Singleton entity (one per parent) is singular with no item ID: get GET, update PUT and
//     delete DELETE it, and any other verb POSTs to it.
//
// shippin.tenancy.coverage_tier.update for staff is PUT /v1/operator/tenancy/coverage-tiers/{coverageTierId}.
func RouteFor(id string, s Spec) (string, error) {
	parts := strings.Split(id, ".")
	if len(parts) < 4 || parts[0] != "shippin" || !idPattern.MatchString(id) {
		return "", errors.New("an action ID is shippin.<domain>.<entity...>.<verb>")
	}
	domain, entities, verb := parts[1], parts[2:len(parts)-1], parts[len(parts)-1]
	if !slices.Contains(Domains, domain) {
		return "", errors.New("unknown domain " + domain)
	}
	var b strings.Builder
	switch s.Access {
	case Staff:
		b.WriteString("/v1/operator")
	case Member:
		b.WriteString("/v1/tenants/{tenantId}")
	case SignedIn:
		b.WriteString("/v1/me")
	case Public:
		b.WriteString("/v1")
	default:
		return "", errors.New("access must be public, signed-in, staff or member")
	}
	b.WriteString("/" + domain)
	for i, e := range entities {
		if i == len(entities)-1 && s.Singleton {
			b.WriteString("/" + kebab(e))
			break
		}
		b.WriteString("/" + plural(kebab(e)))
		if i < len(entities)-1 {
			b.WriteString("/{" + param(e) + "}")
		}
	}
	item := "/{" + param(entities[len(entities)-1]) + "}"
	if s.Singleton {
		if VerbOf(verb, s).Collection {
			return "", errors.New("a singleton has no collection: it is read with get and changed with update")
		}
		item = ""
	}
	v, standard := VerbOf(verb, s), Verbs[verb] != (Verb{})
	switch {
	case standard && s.Collection:
		return "", errors.New("Collection is for a verb of your own; " + verb + " already says what it acts on")
	case standard && v.Collection:
		return v.Method + " " + b.String(), nil
	case standard:
		return v.Method + " " + b.String() + item, nil
	case v.Collection:
		return v.Method + " " + b.String() + "/" + kebab(verb), nil
	}
	return v.Method + " " + b.String() + item + "/" + kebab(verb), nil
}

// Verb is what a verb decides about its action: the method, whether it acts on the collection rather
// than one item, and the risk.
type Verb struct {
	Method     string
	Collection bool
	Risk       Risk
}

// Verbs are the verbs with a method of their own. Routes, risks and validation all read this table.
var Verbs = map[string]Verb{
	"list":   {Method: "GET", Collection: true, Risk: Read},
	"get":    {Method: "GET", Risk: Read},
	"create": {Method: "POST", Collection: true, Risk: Write},
	"update": {Method: "PUT", Risk: Write},
	"delete": {Method: "DELETE", Risk: Destructive},
	"remove": {Method: "DELETE", Risk: Destructive},
	"revoke": {Method: "DELETE", Risk: Destructive},
}

// VerbOf is the verb's row in Verbs. Any other verb is a write POSTed to one item, or to the
// collection when the spec says Collection: POST .../coverage-tiers/{coverageTierId}/publish.
func VerbOf(verb string, s Spec) Verb {
	if v, ok := Verbs[verb]; ok {
		return v
	}
	return Verb{Method: "POST", Collection: s.Collection, Risk: Write}
}

func kebab(s string) string { return strings.ReplaceAll(s, "_", "-") }

// param is the path parameter for one item of an entity: coverage_tier -> coverageTierId.
func param(entity string) string {
	words := strings.Split(entity, "_")
	for i := 1; i < len(words); i++ {
		words[i] = strings.ToUpper(words[i][:1]) + words[i][1:]
	}
	return strings.Join(words, "") + "Id"
}

// plural is English plural for an entity's last word.
func plural(s string) string {
	switch {
	case strings.HasSuffix(s, "y") && len(s) > 1 && !strings.ContainsAny(s[len(s)-2:len(s)-1], "aeiou"):
		return s[:len(s)-1] + "ies"
	case strings.HasSuffix(s, "s"), strings.HasSuffix(s, "x"), strings.HasSuffix(s, "z"), strings.HasSuffix(s, "ch"), strings.HasSuffix(s, "sh"):
		return s + "es"
	}
	return s + "s"
}
