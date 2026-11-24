package hostd

import (
	"context"
	"fmt"
	"strings"
)

// innerInst is one instance inside a tenant VM, decoded from the inner Incus API.
// Guest output is untrusted: callers keep only names that classify as ours.
type innerInst struct {
	Name    string            `json:"name"`
	Project string            `json:"project"`
	Status  string            `json:"status"`
	Config  map[string]string `json:"config"`
	State   vmState           `json:"state"`
}

// listInner lists instances in every inner project. M1 workspaces live in pj-<uuid>,
// not in the legacy "ws" project place.query would pin.
func listInner(ctx context.Context, at place, recursion int) ([]innerInst, error) {
	var list []innerInst
	err := at.queryAll(ctx, fmt.Sprintf("/1.0/instances?recursion=%d", recursion), &list)
	return list, err
}

type innerItem struct {
	Kind    string // "workspace" or "utility"
	ID      string
	Project string
	Name    string
}

// classifyInner accepts only w-<uuid> workspaces and u-<uuid> activation containers.
func classifyInner(name string) (kind, id string, ok bool) {
	var prefix string
	switch {
	case strings.HasPrefix(name, "w-"):
		kind, prefix = "workspace", "w-"
	case strings.HasPrefix(name, "u-"):
		kind, prefix = "utility", "u-"
	default:
		return "", "", false
	}
	id = strings.TrimPrefix(name, prefix)
	if ValidUUID(id) != nil {
		return "", "", false
	}
	return kind, id, true
}

// inventory classifies a guest instance list. One unexpected name fails the whole list
// so a delete cannot proceed while an unknown instance still has an edge route.
func inventory(list []innerInst) ([]innerItem, error) {
	out := make([]innerItem, 0, len(list))
	for _, row := range list {
		kind, id, ok := classifyInner(row.Name)
		if !ok {
			return nil, fmt.Errorf("unexpected instance %q in tenant VM", row.Name)
		}
		out = append(out, innerItem{Kind: kind, ID: id, Project: row.Project, Name: row.Name})
	}
	return out, nil
}

// atProject scopes a VM place to the inner project a guest reported. Only "ws" and
// pj-<uuid> are accepted.
func (p place) atProject(project string) (place, error) {
	if !p.vm {
		return p, nil
	}
	switch {
	case project == InnerProject:
		return p.in(""), nil
	case strings.HasPrefix(project, "pj-") && ValidUUID(project[3:]) == nil:
		return p.in(project[3:]), nil
	default:
		return place{}, fmt.Errorf("unexpected inner project %q", project)
	}
}
