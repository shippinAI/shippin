package devspace_option

import (
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/platform/config"
	"shippin.cloud/console/internal/runtime/template"
)

// Get is shippin.runtime.devspace_option.get: GET /v1/me/runtime/devspace-option. A template's start
// command is staff detail and is left out.
var Get = mcpl.Def[struct{}, Options]{
	Spec: mcpl.Spec{Summary: "Read what a development space can start from: the templates, the sizes and what the server accepts.", Access: mcpl.SignedIn, Singleton: true},
	Run: func(c *mcpl.Call, _ struct{}) (Options, error) {
		w, err := mcpl.Need[config.Workspaces](c)
		if err != nil {
			return Options{}, err
		}
		ts, err := template.Enabled(c.Ctx, c.DB)
		if err != nil {
			return Options{}, err
		}
		for i := range ts {
			ts[i].Startup = ""
		}
		sizes := w.Sizes
		if sizes == nil {
			sizes = []config.Size{}
		}
		hosts := w.RepoHosts
		if hosts == nil {
			hosts = []string{}
		}
		return Options{Templates: ts, Sizes: sizes, Rules: Rules{RepoHosts: hosts, ImagePrefix: w.ImagePrefix,
			PreviewMin: w.PreviewMin, PreviewMax: w.PreviewMax, Template: w.TemplateLimits}}, nil
	},
}
