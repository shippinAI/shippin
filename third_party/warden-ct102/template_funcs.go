package main

// Template helper functions used by the component partials per Spec 17.
// Kept in a single file so the dependency surface stays visible (warden/AGENTS.md
// "Dependencies" rule — no sprig, no new deps).
//
// Wire into main.go's template root via:
//
//   tmpl, err := template.New("").Funcs(funcMap).ParseFS(
//       templatesFS, "templates/*.html", "components/*.html")
//
// Companion to: customers.go + customers_handlers.go + warden/components/*.html.

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"strings"
)

// componentFuncMap is the component template helper registry. Wire into ParseFS via
// template.New("").Funcs(funcMap).
var componentFuncMap = template.FuncMap{
	"dict":            tfDict,
	"list":            tfList,
	"append":          tfAppend,
	"lower":           strings.ToLower,
	"upper":           strings.ToUpper,
	"hasPrefix":       strings.HasPrefix,
	"hasSuffix":       strings.HasSuffix,
	"contains":        strings.Contains,
	"trim":            strings.TrimSpace,
	"toString":        tfToString,
	"sparkPoints":     tfSparkPoints,
	"renderShellBody": tfRenderShellBody,
	// Existing helpers in main.go remain registered alongside these.
	// totalDiskPerc + levelTextClass are already declared on the existing
	// template root; merging both maps happens in main.go init.
}

// tfDict builds a map[string]interface{} from alternating key/value args.
// Use: {{ template "components.status_pill" (dict "State" "active" "Label" "Active") }}
func tfDict(pairs ...interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		k, ok := pairs[i].(string)
		if !ok {
			continue
		}
		m[k] = pairs[i+1]
	}
	return m
}

// tfList builds an []interface{} from variadic args.
// Use: {{ template "components.toolbar" (dict "Actions" (list (dict "Label" "Save"))) }}
func tfList(items ...interface{}) []interface{} {
	out := make([]interface{}, len(items))
	copy(out, items)
	return out
}

// tfAppend appends one item to a list. Returns a new list (immutable-style)
// so chained appends in templates don't surprise downstream uses of the
// original slice.
func tfAppend(list []interface{}, item interface{}) []interface{} {
	out := make([]interface{}, 0, len(list)+1)
	out = append(out, list...)
	out = append(out, item)
	return out
}

// tfToString stringifies any value for template interpolation. Equivalent
// to fmt.Sprint but explicit so authors know what they're calling.
func tfToString(v interface{}) string {
	return fmt.Sprint(v)
}

// tfSparkPoints renders an SVG polyline points string from a []float64.
// Normalizes input to a 100x24 viewBox (the c-metric-spark__svg dimensions).
// Returns "" on empty input.
func tfSparkPoints(values []float64) string {
	if len(values) == 0 {
		return ""
	}
	min, max := values[0], values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	rng := max - min
	if rng == 0 {
		rng = 1 // flat line at mid-height
	}
	var sb strings.Builder
	step := 100.0 / float64(len(values)-1)
	if len(values) == 1 {
		step = 100
	}
	for i, v := range values {
		x := float64(i) * step
		y := 24.0 - ((v-min)/rng)*24.0
		if i > 0 {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%.1f,%.1f", x, y)
	}
	return sb.String()
}

// tfRenderShellBody renders a named page body template inside components.shell.
// Each page owns a unique body template (for example "customers.body") so the
// global template set cannot accidentally reuse another page's shell slot.
func tfRenderShellBody(name string, data interface{}) template.HTML {
	if strings.TrimSpace(name) == "" {
		return ""
	}
	if templates == nil {
		log.Printf("template error: template root not initialised")
		return ""
	}
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("template error: %v", err)
		return ""
	}
	return template.HTML(buf.String())
}
