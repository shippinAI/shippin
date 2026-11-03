package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleAgentWorkspaces(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/agent-workspaces", nil)
	rec := httptest.NewRecorder()

	handleAgentWorkspaces(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Agent Workspaces",
		"open-warden-devstation-codex.cmd",
		"SSH stream",
		"/workspace/warden-storage/projects/WardenClyffe-latest",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q", want)
		}
	}
}

func TestHandleAgentWorkspacesNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/agent-workspaces/extra", nil)
	rec := httptest.NewRecorder()

	handleAgentWorkspaces(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
