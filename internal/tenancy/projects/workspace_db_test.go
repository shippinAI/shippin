package projects

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestOrganizationWorkspaceJourneyAndIsolation(t *testing.T) {
	s, ctx, tenant := testStore(t)
	raw, err := s.OrganizationWorkspaces(ctx, tenant)
	var spaces []struct {
		ID, Name  string
		IsDefault bool `json:"is_default"`
	}
	if err != nil || json.Unmarshal(raw, &spaces) != nil || len(spaces) != 1 || !spaces[0].IsDefault {
		t.Fatalf("default workspace: %s %v", raw, err)
	}
	defaultID := spaces[0].ID
	id, err := s.CreateOrganizationWorkspace(ctx, tenant, "owner", WorkspaceInput{Name: "Research", Description: "Persistent project grouping"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateOrganizationWorkspace(ctx, tenant, "owner", WorkspaceInput{Name: "research"}); !errors.Is(err, ErrNameTaken) {
		t.Fatal("case-insensitive duplicate", err)
	}
	if err := s.UpdateOrganizationWorkspace(ctx, tenant, id, WorkspaceInput{Name: "Product", Description: "Updated"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first-app", "second-app"} {
		if _, err := s.CreateProjectInWorkspace(ctx, tenant, id, name, "dev", "owner"); err != nil {
			t.Fatal(err)
		}
	}
	var projects []struct {
		ID          string
		WorkspaceID string `json:"workspace_id"`
	}
	raw, err = s.WorkspaceProjects(ctx, tenant, id)
	if err != nil || json.Unmarshal(raw, &projects) != nil || len(projects) != 2 || projects[0].WorkspaceID != id {
		t.Fatalf("two persisted projects: %s %v", raw, err)
	}
	if err := s.DeleteOrganizationWorkspace(ctx, tenant, id); !errors.Is(err, ErrWorkspaceOccupied) {
		t.Fatal("occupied deletion", err)
	}
	if err := s.DeleteOrganizationWorkspace(ctx, tenant, defaultID); !errors.Is(err, ErrWorkspaceOccupied) {
		t.Fatal("default deletion", err)
	}
	other := uuid.NewString()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO tenants(id,idp_org_id,name,plan) VALUES($1,$2,'Other','founder')`, other, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OrganizationWorkspace(ctx, other, id); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign workspace read", err)
	}
	if _, err := s.WorkspaceProjects(ctx, other, id); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign project list", err)
	}
	if _, err := s.CreateProjectInWorkspace(ctx, other, id, "intruder-app", "dev", "other"); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign workspace assignment", err)
	}
	if err := s.UpdateOrganizationWorkspace(ctx, other, id, WorkspaceInput{Name: "Stolen"}); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign update", err)
	}
	if err := s.DeleteOrganizationWorkspace(ctx, other, id); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign delete", err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE projects SET workspace_id=$1 WHERE id=$2`, defaultID, projects[0].ID); err != nil {
		t.Fatal(err)
	}
	// Database constraints enforce ancestry even if a future caller skips the store.
	if _, err := s.Pool.Exec(ctx, `UPDATE projects SET tenant_id=$1 WHERE id=$2`, other, projects[0].ID); err == nil {
		t.Fatal("cross-tenant database write accepted")
	}
	legacy, err := s.CreateProject(ctx, tenant, "legacy-source", "dev", "owner")
	if err != nil {
		t.Fatal(err)
	}
	var assigned string
	if err := s.Pool.QueryRow(ctx, `SELECT workspace_id::text FROM projects WHERE id=$1`, legacy).Scan(&assigned); err != nil || assigned != defaultID {
		t.Fatal("compatibility path lost default workspace", assigned, err)
	}
	empty, err := s.CreateOrganizationWorkspace(ctx, tenant, "owner", WorkspaceInput{Name: "Disposable"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteOrganizationWorkspace(ctx, tenant, empty); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OrganizationWorkspace(ctx, tenant, empty); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted workspace visible", err)
	}
}
