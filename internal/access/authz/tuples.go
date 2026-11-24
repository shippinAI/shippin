package authz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Writer stores relationship tuples. Ownership lives in OpenFGA, not only in Postgres.
type Writer interface {
	Write(ctx context.Context, writes []Tuple) error
}

type writeBody struct {
	Writes struct {
		TupleKeys   []Tuple `json:"tuple_keys"`
		OnDuplicate string  `json:"on_duplicate"`
	} `json:"writes"`
	AuthorizationModelID string `json:"authorization_model_id,omitempty"`
}

// Write runs POST /stores/{id}/write as one transaction (OpenFGA applies all or none). Tuples that
// already exist are ignored (on_duplicate, OpenFGA >= 1.7), so a second project in the same org
// re-states the org tuples safely. At most 100 tuples per call (the server limit).
func (f *FGA) Write(ctx context.Context, writes []Tuple) error {
	if len(writes) == 0 || len(writes) > 100 {
		return fmt.Errorf("openfga write: %d tuples (want 1-100)", len(writes))
	}
	var body writeBody
	body.Writes.TupleKeys, body.Writes.OnDuplicate, body.AuthorizationModelID = writes, "ignore", f.ModelID
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.URL+"/stores/"+f.StoreID+"/write", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if f.Token != "" {
		req.Header.Set("Authorization", "Bearer "+f.Token)
	}
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("openfga write: %s", resp.Status)
	}
	return nil
}

// ProjectChain is the ownership chain for one project:
// tenant -> org -> project -> devspace -> rrflow namespace, with owner on org and project.
type ProjectChain struct {
	TenantID, OrgID, ProjectID, DevspaceID, Namespace, OwnerSub string
	// LinkDevspace: the project bound the devspace and its owner owns the devspace. Only then is
	// devspace#project written (it grants can_exec via can_write from project).
	LinkDevspace bool
}

// Tuples lists the chain: 5 tuples, plus 2 devspace links when LinkDevspace. No org role is
// written here: org roles come from Nomen (contextual tuples), so a create can't mint an org owner.
// ProjectID is the project row id (the id shadow checks use); Namespace is the public id (prj_…).
func (c ProjectChain) Tuples() []Tuple {
	org, prj, dev := "org:"+c.OrgID, "project:"+c.ProjectID, "devspace:"+c.DevspaceID
	ns := "rrflow_namespace:" + c.Namespace
	ts := []Tuple{
		{User: "tenant:" + c.TenantID, Relation: "tenant", Object: org},
		{User: org, Relation: "org", Object: prj},
		{User: "tenant:" + c.TenantID, Relation: "tenant", Object: prj},
		{User: "user:" + c.OwnerSub, Relation: "owner", Object: prj},
		{User: prj, Relation: "project", Object: ns},
	}
	if c.LinkDevspace {
		ts = append(ts, Tuple{User: prj, Relation: "project", Object: dev}, Tuple{User: dev, Relation: "devspace", Object: ns})
	}
	return ts
}

// Valid reports a missing id (every link of the chain is required).
func (c ProjectChain) Valid() error {
	for k, v := range map[string]string{"tenant": c.TenantID, "org": c.OrgID, "project": c.ProjectID,
		"devspace": c.DevspaceID, "namespace": c.Namespace, "owner": c.OwnerSub} {
		if v == "" {
			return fmt.Errorf("project chain: %s id missing", k)
		}
	}
	return nil
}
