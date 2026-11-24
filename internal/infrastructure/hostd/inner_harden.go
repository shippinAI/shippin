package hostd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// HardenedProject reports one pj-<uuid> project of a tenant VM after HardenInner.
type HardenedProject struct {
	Project        string   `json:"project"`
	Updated        []string `json:"updated"`
	PendingRestart []string `json:"pending_restart"`
	Restricted     bool     `json:"restricted"`
	Error          string   `json:"error,omitempty"`
}

type innerRow struct {
	Name   string            `json:"name"`
	Status string            `json:"status"`
	Config map[string]string `json:"config"`
}

// HardenInner brings pj- projects created before the restriction set up to it. Order matters:
// privilege=isolated refuses instances without an isolated idmap, so each instance first gets
// security.idmap.isolated (applied on its next start; restart=true restarts running ones now),
// and a project is restricted only once none of its instances waits for that restart.
func (s *Server) HardenInner(ctx context.Context, t string, plan Plan, restart bool) ([]HardenedProject, error) {
	at, err := s.newPlace(ctx, t)
	if err != nil {
		return nil, err
	}
	if !at.vm {
		return nil, fmt.Errorf("tenant is not VM-backed")
	}
	out, err := at.do(ctx, "query", "/1.0/projects?recursion=1")
	if err != nil {
		return nil, err
	}
	var projects []innerRow
	if json.Unmarshal([]byte(out), &projects) != nil {
		return nil, fmt.Errorf("tenant VM returned an unreadable project list")
	}
	res := []HardenedProject{}
	for _, p := range projects {
		id, ok := strings.CutPrefix(p.Name, "pj-")
		if !ok || ValidUUID(id) != nil {
			continue
		}
		res = append(res, s.hardenProject(ctx, at.in(id), plan, restart))
	}
	return res, nil
}

func (s *Server) hardenProject(ctx context.Context, pl place, plan Plan, restart bool) HardenedProject {
	h := HardenedProject{Project: pl.project(), Updated: []string{}, PendingRestart: []string{}}
	var insts []innerRow
	if err := pl.query(ctx, "/1.0/instances?recursion=1", &insts); err != nil {
		h.Error = "instances unreadable"
		return h
	}
	for _, i := range insts {
		w, ok := strings.CutPrefix(i.Name, "w-")
		if !ok || ValidUUID(w) != nil {
			continue
		}
		set := missingKeys(i.Config, plan)
		if len(set) == 0 {
			continue
		}
		if _, err := pl.do(ctx, append([]string{"config", "set", i.Name}, set...)...); err != nil {
			h.Error = "config set failed for " + i.Name
			return h
		}
		h.Updated = append(h.Updated, i.Name)
		if i.Status == "Running" && i.Config["security.idmap.isolated"] != "true" {
			if !restart {
				h.PendingRestart = append(h.PendingRestart, i.Name)
			} else if _, err := pl.do(ctx, "restart", i.Name); err != nil {
				h.PendingRestart = append(h.PendingRestart, i.Name)
			}
		}
	}
	if len(h.PendingRestart) > 0 {
		return h
	}
	args := append([]string{"project", "set", pl.project()}, innerRestrictions(plan)...)
	if _, err := pl.in("").do(ctx, args...); err != nil {
		h.Error = "project restriction failed"
		return h
	}
	h.Restricted = true
	return h
}

// missingKeys returns the k=v pairs an existing workspace lacks: an isolated idmap and the plan
// limits when the instance has none of its own.
func missingKeys(cfg map[string]string, plan Plan) []string {
	var kv []string
	if cfg["security.idmap.isolated"] != "true" {
		kv = append(kv, "security.idmap.isolated=true")
	}
	if cfg["limits.cpu"] == "" && plan.WorkspaceCPU > 0 {
		kv = append(kv, fmt.Sprintf("limits.cpu=%d", plan.WorkspaceCPU))
	}
	if cfg["limits.memory"] == "" && strings.TrimSpace(plan.WorkspaceMem) != "" {
		kv = append(kv, "limits.memory="+plan.WorkspaceMem)
	}
	return kv
}
