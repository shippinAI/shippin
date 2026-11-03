package api

import (
	"fmt"

	"shippin.cloud/console/internal/managed"
)

// vmRRFlowCheck: the tenant VM's one shared rrflow/state instance (Notion D36).
func vmRRFlowCheck(vms []managed.Instance) setupCheck {
	c := setupCheck{ID: "rrflow_vm", Label: "rrflow on the tenant VM (shared by its projects)", State: "todo",
		Detail: "starts with the first project that attaches rrflow"}
	for _, x := range vms {
		if x.Kind != managed.KindRRFlow {
			continue
		}
		switch x.State {
		case "ready":
			c.State, c.Detail = "ok", fmt.Sprintf("ready, %d devspace binding(s)", len(x.Bindings))
		case "error":
			c.State, c.Detail = "error", "instance error"
			return c
		default:
			c.Detail = "instance " + x.State
		}
	}
	return c
}

// rrflowCheck: the project's namespace inside the tenant VM's shared rrflow, bound to at least one
// of its devspaces. A pre-D36 per-project instance still being moved shows as in progress.
func rrflowCheck(p setupProject, insts []managed.Instance, on bool) setupCheck {
	c := setupCheck{ID: "rrflow:" + p.ID, Label: "rrflow namespace for " + p.Name, Project: p.ID, State: "todo", Fix: "rrflow_attach"}
	if !on {
		c.State, c.Detail, c.Fix = "off", "rrflow not enabled for this tenant", ""
		return c
	}
	if p.Workspaces == 0 {
		c.Detail, c.Fix = "open the project once so it has a devspace", "devspace"
		return c
	}
	for _, x := range insts {
		if x.Kind == managed.KindRRFlow && x.State == "migrating" {
			c.Detail, c.Fix = "moving this project's rrflow state into the tenant VM's shared instance", ""
			return c
		}
	}
	for _, x := range insts {
		if x.Kind != managed.KindRRFlow {
			continue
		}
		if x.State == "error" {
			c.State, c.Detail = "error", "instance "+x.State
			return c
		}
		bound := 0
		for _, b := range x.Bindings {
			if b.State == "bound" {
				bound++
			}
		}
		if x.State == "ready" && bound > 0 {
			c.State, c.Detail, c.Fix = "ok", "namespace ready in the shared instance, bound to devspace", ""
		} else {
			c.Detail, c.Fix = "instance "+x.State+", waiting for binding", ""
		}
		return c
	}
	c.Detail = "no rrflow namespace yet"
	return c
}

// autoAttachCheck shows whether services.rrflow.auto_attach binds rrflow to new project devspaces.
func autoAttachCheck(on, auto bool) setupCheck {
	c := setupCheck{ID: "rrflow_auto", Label: "rrflow auto-attach on devspace creation", State: "off", Detail: "services.rrflow.auto_attach is off; attach per project below"}
	switch {
	case !on:
		c.Detail = "rrflow not enabled for this tenant"
	case auto:
		c.State, c.Detail = "ok", "new project devspaces get a namespace in the tenant VM's rrflow, bound automatically"
	}
	return c
}
