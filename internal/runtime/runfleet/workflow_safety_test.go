package runfleet

import (
	"strings"
	"testing"
)

func TestWorkflowKeepsScriptDataAndDynamicLabels(t *testing.T) {
	for _, original := range []string{
		"jobs:\n  test:\n    runs-on:\n      - self-hosted\n      - ${{ matrix.os }}\n",
		"jobs:\n  test:\n    runs-on: {group: trusted, labels: linux}\n",
		"jobs:\n  test:\n    steps:\n      - run: |\n          runs-on: do-not-change-this-script\n",
		"jobs:\n  test:\n    runs-on: *runner\n",
	} {
		plan := PlanRunsOn([]WorkflowFile{{Path: ".github/workflows/ci.yml", Content: original}}, "shippin-2x", false, false)
		if len(plan.Changes) != 0 {
			t.Fatalf("unsafe rewrite: %+v", plan.Changes)
		}
	}
	input := "jobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          runs-on: script-content\n"
	plan := PlanRunsOn([]WorkflowFile{{Path: ".github/workflows/ci.yml", Content: input}}, "shippin-2x", false, false)
	if len(plan.Changes) != 1 || !strings.Contains(plan.Changes[0].Content, "runs-on: shippin-2x") || !strings.Contains(plan.Changes[0].Content, "runs-on: script-content") {
		t.Fatalf("bad mixed workflow: %+v", plan)
	}
}

func TestWorkflowRejectsLabelInjection(t *testing.T) {
	plan := PlanRunsOn(nil, "shippin-2x\n    extra: injected", true, true)
	if len(plan.Changes) != 0 || len(plan.Warnings) == 0 {
		t.Fatal(plan)
	}
}
