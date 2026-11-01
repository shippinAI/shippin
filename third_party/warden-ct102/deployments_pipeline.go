package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func runDeploymentPipeline(id string) {
	if err := updateDeploymentStage(id, DeploymentRunning, StagePrecheck, "running deployment prechecks"); err != nil {
		return
	}
	dep, ok := getDeployment(id)
	if !ok {
		return
	}
	tpl, ok := getCatalogTemplate(dep.TemplateName)
	if !ok {
		failDeployment(id, StagePrecheck, fmt.Errorf("template %q no longer available", dep.TemplateName))
		return
	}

	if dep.Subdomain == "" || dep.TargetHost == "" {
		failDeployment(id, StagePrecheck, fmt.Errorf("invalid deployment request: missing host or subdomain"))
		return
	}
	graph := getInfrastructureGraph(collectStats())
	providerFound := false
	hostFound := false
	for _, p := range graph.Providers {
		if p.ID != dep.ProviderID {
			continue
		}
		providerFound = true
		for _, h := range p.Hosts {
			if strings.EqualFold(h.Name, dep.TargetHost) || strings.EqualFold(h.ID, dep.TargetHost) {
				hostFound = true
				break
			}
		}
	}
	if !providerFound {
		failDeployment(id, StagePrecheck, fmt.Errorf("provider boundary %q not found in infrastructure graph", dep.ProviderID))
		return
	}
	if !hostFound {
		failDeployment(id, StagePrecheck, fmt.Errorf("target host %q not found under provider %q", dep.TargetHost, dep.ProviderID))
		return
	}

	if err := updateDeploymentStage(id, DeploymentRunning, StagePlan, "generated rollout plan and best-practice guardrails"); err != nil {
		failDeployment(id, StagePlan, err)
		return
	}

	// Guided mode is the default safest path: generate exact commands and require operator confirmation.
	if dep.Automation != "auto" {
		dep.Status = DeploymentAwaiting
		dep.Stage = StageExecute
		dep.NextActions = guidedNextActions(dep, tpl)
		dep.UpdatedAt = time.Now().UTC()
		if err := setDeployment(dep); err != nil {
			failDeployment(id, StageExecute, err)
			return
		}
		appendDeploymentEvent(id, "manual", string(StageExecute), "guided mode generated exact rollout steps; awaiting operator apply")
		return
	}
	if !dep.Approved {
		dep.Status = DeploymentAwaiting
		dep.Stage = StagePlan
		dep.UpdatedAt = time.Now().UTC()
		dep.NextActions = append(dep.NextActions, "Approve production automation in deployment detail before EXECUTE stage.")
		if err := setDeployment(dep); err != nil {
			failDeployment(id, StagePlan, err)
			return
		}
		appendDeploymentEvent(id, "manual", string(StagePlan), "auto mode blocked pending production approval")
		return
	}

	if err := updateDeploymentStage(id, DeploymentRunning, StageExecute, "executing automated rollout hook"); err != nil {
		failDeployment(id, StageExecute, err)
		return
	}
	if err := executeRolloutHook(dep, tpl); err != nil {
		failDeployment(id, StageExecute, err)
		return
	}

	if err := updateDeploymentStage(id, DeploymentRunning, StageVerify, "verifying rollout result and publishing health state"); err != nil {
		failDeployment(id, StageVerify, err)
		return
	}

	dep, _ = getDeployment(id)
	dep.Status = DeploymentHealthy
	dep.Stage = StageDone
	dep.UpdatedAt = time.Now().UTC()
	dep.NextActions = []string{
		"Capture bootstrap credentials and rotate generated secrets.",
		"Attach customer-visible links into Clyffe order timeline.",
		"Record post-deploy smoke test result and SLO baseline.",
	}
	if err := setDeployment(dep); err != nil {
		failDeployment(id, StageDone, err)
		return
	}
	appendDeploymentEvent(id, "info", string(StageDone), "deployment completed and marked healthy")
}

func guidedNextActions(dep Deployment, tpl CatalogTemplate) []string {
	actions := []string{
		fmt.Sprintf("Create runtime on %s (%s mode) and reserve a VMID before compose push.", dep.TargetHost, dep.Runtime),
		fmt.Sprintf("Render `%s` with deployment overrides and write `.env` securely (0600).", tpl.Name),
		fmt.Sprintf("Route `%s` only after health endpoint responds successfully.", dep.PublicURL),
	}
	if len(tpl.Secrets) > 0 {
		actions = append(actions, fmt.Sprintf("Generate %d secret values and store out of band (never paste in chat/logs).", len(tpl.Secrets)))
	}
	return actions
}

func executeRolloutHook(dep Deployment, tpl CatalogTemplate) error {
	hook := os.Getenv("WARDEN_ROLLOUT_HOOK")
	if hook == "" {
		return fmt.Errorf("WARDEN_ROLLOUT_HOOK not configured; keep deployment in guided mode or set hook")
	}

	payload := map[string]any{
		"deployment":     dep,
		"template":       tpl,
		"best_practices": deploymentBestPractices(),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	cmd := exec.Command(hook)
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rollout hook failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	appendDeploymentEvent(dep.ID, "info", string(StageExecute), "rollout hook completed successfully")
	return nil
}
