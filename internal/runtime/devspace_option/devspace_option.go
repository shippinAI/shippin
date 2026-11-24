// Package devspace_option is what a person may choose when making a development space: the templates
// that are turned on, the sizes on offer and what the server accepts. The create page draws its form
// from this answer and holds no rule of its own.
package devspace_option

import (
	"shippin.cloud/console/internal/platform/config"
	"shippin.cloud/console/internal/runtime/template"
)

// Options are the choices and the rules they are checked against.
type Options struct {
	Templates []template.Template `json:"templates"`
	Sizes     []config.Size       `json:"sizes"`
	Rules     Rules               `json:"rules"`
}

// Rules are what the server accepts, so a form can say so before it sends.
type Rules struct {
	RepoHosts   []string              `json:"repo_hosts"`
	ImagePrefix string                `json:"image_prefix"`
	PreviewMin  int                   `json:"preview_min"`
	PreviewMax  int                   `json:"preview_max"`
	Template    config.TemplateLimits `json:"template"`
}
