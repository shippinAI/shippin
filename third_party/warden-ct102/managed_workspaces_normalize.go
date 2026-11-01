package main

import (
	"fmt"
	"sort"
	"strings"
)

func (v *ManagedWorkspaceMap) normalize() {
	projectCounts := map[string]int{}
	connectionCounts := map[string]int{}
	planes := map[string]bool{}

	for _, p := range v.Projects {
		projectCounts[p.WorkspaceSlug]++
	}
	v.Connections = buildManagedWorkspaceConnections(*v)
	for _, c := range v.Connections {
		connectionCounts[c.From]++
		connectionCounts[c.To]++
	}
	for i := range v.Workspaces {
		w := &v.Workspaces[i]
		w.ProjectCount = projectCounts[w.WorkspaceSlug]
		w.ConnectionCount = connectionCounts[w.WorkspaceSlug]
		w.Role = managedWorkspaceRole(*w)
		for _, p := range w.ContributesToPlanes {
			planes[planeKey(p)] = true
		}
	}

	sort.Slice(v.Workspaces, func(i, j int) bool {
		return v.Workspaces[i].WorkspaceSlug < v.Workspaces[j].WorkspaceSlug
	})
	sort.Slice(v.Projects, func(i, j int) bool {
		return v.Projects[i].ProjectKey < v.Projects[j].ProjectKey
	})
	sort.Slice(v.ServiceInstances, func(i, j int) bool {
		return v.ServiceInstances[i].ServiceSlug < v.ServiceInstances[j].ServiceSlug
	})
	sort.Slice(v.Connections, func(i, j int) bool {
		if v.Connections[i].Kind == v.Connections[j].Kind {
			return v.Connections[i].Label < v.Connections[j].Label
		}
		return v.Connections[i].Kind < v.Connections[j].Kind
	})

	v.Summary = ManagedWorkspaceSummary{
		WorkspaceCount:  len(v.Workspaces),
		ProjectCount:    len(v.Projects),
		ServiceCount:    len(v.ServiceInstances),
		ConnectionCount: len(v.Connections),
		PlaneCount:      len(planes),
	}
}

func buildManagedWorkspaceConnections(v ManagedWorkspaceMap) []ManagedWorkspaceConnection {
	var out []ManagedWorkspaceConnection
	for _, p := range v.Projects {
		out = append(out, ManagedWorkspaceConnection{
			Kind:   "project",
			From:   p.WorkspaceSlug,
			To:     p.ProjectKey,
			Label:  p.ProjectKey,
			Status: p.Status,
			Detail: "repo " + p.RepoDir,
		})
	}
	for _, s := range v.ServiceInstances {
		name := firstNonEmpty(s.DisplayName, s.ServiceSlug)
		if s.PrimaryWorkspaceSlug != "" {
			out = append(out, ManagedWorkspaceConnection{
				Kind:   "service-primary",
				From:   s.ServiceSlug,
				To:     s.PrimaryWorkspaceSlug,
				Label:  name,
				Status: "primary",
				Detail: fmt.Sprintf("%s on %s", s.ServiceKind, firstNonEmpty(s.Host.SSHAlias, s.Host.IP)),
			})
		}
		for _, slug := range s.AttachedWorkspaceSlugs {
			if slug == "" || slug == s.PrimaryWorkspaceSlug {
				continue
			}
			out = append(out, ManagedWorkspaceConnection{
				Kind:   "service-attachment",
				From:   s.ServiceSlug,
				To:     slug,
				Label:  name,
				Status: "attached",
				Detail: "allowed workspace attachment",
			})
		}
	}
	for _, w := range v.Workspaces {
		for _, p := range w.ContributesToPlanes {
			out = append(out, ManagedWorkspaceConnection{
				Kind:   "shared-plane",
				From:   w.WorkspaceSlug,
				To:     planeKey(p),
				Label:  p.Plane,
				Status: "shared",
				Detail: fmt.Sprintf("%s/%s", p.SurrealNS, p.SurrealDB),
			})
		}
	}
	return out
}

func managedWorkspaceRole(w ManagedWorkspace) string {
	switch w.WorkspaceSlug {
	case "wardenclyffe.infra":
		return "Operator control plane"
	case "node.master":
		return "Assistant and specialist-team baseline"
	default:
		return strings.ReplaceAll(w.WorkspaceKind, "-", " ")
	}
}

func planeKey(p ManagedPlaneRef) string {
	return strings.Trim(strings.Join([]string{p.Plane, p.SurrealNS, p.SurrealDB}, "/"), "/")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return "unassigned"
}
