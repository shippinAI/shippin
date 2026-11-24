package mail

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	htmltpl "html/template"
	"strings"
	texttpl "text/template"
)

//go:embed templates/*
var files embed.FS

// Notice is the data for one notice. Vars fill the copy in templates/notices.json.
type Notice struct {
	Kind   string
	Vars   map[string]string // Brand, Tenant, Workspace, Admin, Actor, ...
	Facts  [][2]string
	URL    string
	Footer string
	Alert  bool
}

type copyText struct{ Subject, Heading, Intro, Action string }

// Renderer turns notices into subject + text + HTML using the embedded templates.
type Renderer struct {
	copy map[string]copyText
	html *htmltpl.Template
	text *texttpl.Template
}

func NewRenderer() (*Renderer, error) {
	raw, err := files.ReadFile("templates/notices.json")
	if err != nil {
		return nil, err
	}
	r := &Renderer{}
	if err := json.Unmarshal(raw, &r.copy); err != nil {
		return nil, err
	}
	if r.html, err = htmltpl.ParseFS(files, "templates/notice.html.tmpl"); err != nil {
		return nil, err
	}
	if r.text, err = texttpl.ParseFS(files, "templates/notice.txt.tmpl"); err != nil {
		return nil, err
	}
	return r, nil
}

// Kinds lists the notice kinds the copy file defines.
func (r *Renderer) Kinds() []string {
	out := make([]string, 0, len(r.copy))
	for k := range r.copy {
		out = append(out, k)
	}
	return out
}

func fill(s string, vars map[string]string) (string, error) {
	t, err := texttpl.New("").Option("missingkey=error").Parse(s)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	err = t.Execute(&b, vars)
	return strings.ReplaceAll(b.String(), "\n", " "), err
}

func (r *Renderer) Render(n Notice) (Rendered, error) {
	c, ok := r.copy[n.Kind]
	if !ok {
		return Rendered{}, fmt.Errorf("mail: unknown notice kind %q", n.Kind)
	}
	data := map[string]any{"Brand": n.Vars["Brand"], "Facts": n.Facts, "URL": n.URL, "Footer": n.Footer, "Alert": n.Alert}
	for k, s := range map[string]string{"Subject": c.Subject, "Heading": c.Heading, "Intro": c.Intro, "Action": c.Action} {
		v, err := fill(s, n.Vars)
		if err != nil {
			return Rendered{}, fmt.Errorf("mail: %s %s: %w", n.Kind, k, err)
		}
		data[k] = v
	}
	var h, t bytes.Buffer
	if err := r.html.Execute(&h, data); err != nil {
		return Rendered{}, err
	}
	if err := r.text.Execute(&t, data); err != nil {
		return Rendered{}, err
	}
	return Rendered{Subject: data["Subject"].(string), Text: t.String(), HTML: h.String()}, nil
}
