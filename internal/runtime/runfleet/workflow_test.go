package runfleet

import (
	"strings"
	"testing"
)

func TestPlanRunsOnScalarAndList(t *testing.T) {
	files := []WorkflowFile{{
		Path:    ".github/workflows/ci.yml",
		Content: "jobs:\n  test:\n    runs-on: ubuntu-latest\n    steps: []\n  win:\n    runs-on:\n      - windows-latest\n",
	}}
	plan := PlanRunsOn(files, "shippin-linux-x64-2", false, false)
	if !plan.DryRun {
		t.Fatal("plan should stay a dry-run until a pull request exists")
	}
	if len(plan.Changes) != 1 {
		t.Fatalf("changes = %d", len(plan.Changes))
	}
	got := plan.Changes[0].Content
	if strings.Count(got, "runs-on: shippin-linux-x64-2") != 2 {
		t.Fatalf("content:\n%s", got)
	}
	if strings.Contains(got, "ubuntu-latest") || strings.Contains(got, "windows-latest") {
		t.Fatalf("old labels remain:\n%s", got)
	}
	if !strings.Contains(plan.Diff, "-    runs-on: ubuntu-latest") {
		t.Fatalf("diff:\n%s", plan.Diff)
	}
}

func TestPlanRunsOnLeavesExpressions(t *testing.T) {
	files := []WorkflowFile{{
		Path:    ".github/workflows/ci.yml",
		Content: "jobs:\n  test:\n    runs-on: ${{ matrix.os }}\n",
	}}
	plan := PlanRunsOn(files, "shippin-linux-x64-2", false, false)
	if len(plan.Changes) != 0 {
		t.Fatalf("expression was rewritten: %+v", plan.Changes)
	}
	if len(plan.Warnings) == 0 {
		t.Fatal("expected a warning")
	}
}

func TestLovableWorkflowWhenMissing(t *testing.T) {
	plan := PlanRunsOn(nil, "shippin-linux-arm-2", true, true)
	if len(plan.Changes) != 1 || plan.Changes[0].Path != ".github/workflows/shippin-run.yml" {
		t.Fatalf("changes = %+v", plan.Changes)
	}
	if !strings.Contains(plan.Changes[0].Content, "runs-on: shippin-linux-arm-2") {
		t.Fatal(plan.Changes[0].Content)
	}
}
