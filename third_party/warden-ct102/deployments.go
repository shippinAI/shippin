package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type DeploymentStatus string

const (
	DeploymentPending       DeploymentStatus = "pending"
	DeploymentRunning       DeploymentStatus = "running"
	DeploymentAwaiting      DeploymentStatus = "awaiting_manual"
	DeploymentHealthy       DeploymentStatus = "healthy"
	DeploymentFailed        DeploymentStatus = "failed"
	DeploymentCancelled     DeploymentStatus = "cancelled"
	DeploymentNeedsRetry    DeploymentStatus = "needs_retry"
	defaultDeploymentDomain                  = "example.invalid"
)

type DeploymentStage string

const (
	StagePending  DeploymentStage = "PENDING"
	StagePrecheck DeploymentStage = "PRECHECK"
	StagePlan     DeploymentStage = "PLAN"
	StageExecute  DeploymentStage = "EXECUTE"
	StageVerify   DeploymentStage = "VERIFY"
	StageDone     DeploymentStage = "DONE"
)

type DeploymentPlanStep struct {
	Stage       string `json:"stage"`
	Title       string `json:"title"`
	Command     string `json:"command,omitempty"`
	Rationale   string `json:"rationale,omitempty"`
	SafetyCheck string `json:"safety_check,omitempty"`
}

type DeploymentEvent struct {
	At      time.Time `json:"at"`
	Level   string    `json:"level"`
	Stage   string    `json:"stage"`
	Message string    `json:"message"`
}

type Deployment struct {
	ID            string               `json:"id"`
	Name          string               `json:"name"`
	CustomerSlug  string               `json:"customer_slug"`
	ProviderID    string               `json:"provider_id"`
	Estate        string               `json:"estate"`
	Visibility    string               `json:"visibility"`
	VLAN          string               `json:"vlan,omitempty"`
	RequestedBy   string               `json:"requested_by"` // clyffe | warden
	TemplateName  string               `json:"template_name"`
	TargetHost    string               `json:"target_host"`
	Runtime       string               `json:"runtime"` // ct | vm
	Subdomain     string               `json:"subdomain"`
	PublicURL     string               `json:"public_url"`
	Automation    string               `json:"automation"` // guided | auto
	Approved      bool                 `json:"approved"`
	ApprovedBy    string               `json:"approved_by,omitempty"`
	ApprovedAt    time.Time            `json:"approved_at,omitempty"`
	ApprovalNote  string               `json:"approval_note,omitempty"`
	Status        DeploymentStatus     `json:"status"`
	Stage         DeploymentStage      `json:"stage"`
	StageHistory  []string             `json:"stage_history,omitempty"`
	BestPractices []string             `json:"best_practices,omitempty"`
	Plan          []DeploymentPlanStep `json:"plan,omitempty"`
	NextActions   []string             `json:"next_actions,omitempty"`
	LastError     string               `json:"last_error,omitempty"`
	CreatedAt     time.Time            `json:"created_at"`
	UpdatedAt     time.Time            `json:"updated_at"`
}

type DeploymentsPageData struct {
	HeaderData
	Deployments []Deployment
	TopTab      string
	SubTab      string
	TopCounts   map[string]int
	SubCounts   map[string]int
}

type DeploymentNewPageData struct {
	HeaderData
	SelectedTemplate string
	Templates        []CatalogTemplate
	DefaultHost      string
	ProviderOptions  []string
}

type DeploymentDetailPageData struct {
	HeaderData
	Deployment Deployment
	Events     []DeploymentEvent
}

type DeploymentCreateRequest struct {
	Name         string `json:"name"`
	CustomerSlug string `json:"customer_slug"`
	ProviderID   string `json:"provider_id"`
	Estate       string `json:"estate"`
	Visibility   string `json:"visibility"`
	VLAN         string `json:"vlan"`
	RequestedBy  string `json:"requested_by"`
	TemplateName string `json:"template_name"`
	TargetHost   string `json:"target_host"`
	Runtime      string `json:"runtime"`
	Subdomain    string `json:"subdomain"`
	Automation   string `json:"automation"`
}

type DeploymentReview struct {
	DeploymentID string               `json:"deployment_id"`
	Template     string               `json:"template"`
	ProviderID   string               `json:"provider_id"`
	Estate       string               `json:"estate"`
	Visibility   string               `json:"visibility"`
	TargetHost   string               `json:"target_host"`
	Runtime      string               `json:"runtime"`
	PublicURL    string               `json:"public_url"`
	Status       DeploymentStatus     `json:"status"`
	Stage        DeploymentStage      `json:"stage"`
	Approved     bool                 `json:"approved"`
	ApprovedBy   string               `json:"approved_by,omitempty"`
	ApprovedAt   time.Time            `json:"approved_at,omitempty"`
	ApprovalNote string               `json:"approval_note,omitempty"`
	Plan         []DeploymentPlanStep `json:"plan"`
	NextActions  []string             `json:"next_actions"`
	ReviewScript string               `json:"review_script"`
}

var (
	deploymentsMu      sync.RWMutex
	deploymentsByID    = map[string]Deployment{}
	deploymentEvents   = map[string][]DeploymentEvent{}
	deploymentWatchers = map[string][]chan DeploymentEvent{}
	deploymentsDir     string
)

func init() {
	deploymentsDir = resolveDeploymentsDir()
	loadDeployments()
}

func resolveDeploymentsDir() string {
	if d := strings.TrimSpace(os.Getenv("WARDEN_DEPLOYMENTS_DIR")); d != "" {
		_ = os.MkdirAll(d, 0o700)
		return d
	}
	if st, err := os.Stat("/etc/warden"); err == nil && st.IsDir() {
		d := filepath.Join("/etc/warden", "deployments")
		_ = os.MkdirAll(d, 0o700)
		return d
	}
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		cfgDir = os.TempDir()
	}
	d := filepath.Join(cfgDir, "Warden", "deployments")
	_ = os.MkdirAll(d, 0o700)
	return d
}

func loadDeployments() {
	entries, err := os.ReadDir(deploymentsDir)
	if err != nil {
		log.Printf("Failed to read deployments dir %q: %v", deploymentsDir, err)
		return
	}
	deploymentsMu.Lock()
	defer deploymentsMu.Unlock()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(deploymentsDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Failed to read deployment file %q: %v", path, err)
			continue
		}
		var dep Deployment
		if err := json.Unmarshal(data, &dep); err != nil {
			log.Printf("Failed to parse deployment file %q: %v", path, err)
			continue
		}
		deploymentsByID[dep.ID] = dep
	}
}

func deploymentFilePath(id string) string {
	return filepath.Join(deploymentsDir, id+".json")
}

func saveDeploymentUnlocked(dep Deployment) error {
	data, err := json.MarshalIndent(dep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(deploymentFilePath(dep.ID), data, 0o600)
}

func listDeployments() []Deployment {
	deploymentsMu.RLock()
	defer deploymentsMu.RUnlock()
	out := make([]Deployment, 0, len(deploymentsByID))
	for _, d := range deploymentsByID {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func getDeployment(id string) (Deployment, bool) {
	deploymentsMu.RLock()
	defer deploymentsMu.RUnlock()
	d, ok := deploymentsByID[id]
	return d, ok
}

func setDeployment(dep Deployment) error {
	deploymentsMu.Lock()
	defer deploymentsMu.Unlock()
	deploymentsByID[dep.ID] = dep
	return saveDeploymentUnlocked(dep)
}

func appendDeploymentEvent(id, level, stage, message string) {
	ev := DeploymentEvent{
		At:      time.Now().UTC(),
		Level:   level,
		Stage:   stage,
		Message: message,
	}
	deploymentsMu.Lock()
	deploymentEvents[id] = append(deploymentEvents[id], ev)
	for _, ch := range deploymentWatchers[id] {
		select {
		case ch <- ev:
		default:
		}
	}
	deploymentsMu.Unlock()
}

func getDeploymentEvents(id string) []DeploymentEvent {
	deploymentsMu.RLock()
	defer deploymentsMu.RUnlock()
	evs := deploymentEvents[id]
	out := make([]DeploymentEvent, len(evs))
	copy(out, evs)
	return out
}

func watchDeploymentEvents(id string) (chan DeploymentEvent, func()) {
	ch := make(chan DeploymentEvent, 32)
	deploymentsMu.Lock()
	deploymentWatchers[id] = append(deploymentWatchers[id], ch)
	deploymentsMu.Unlock()
	cancel := func() {
		deploymentsMu.Lock()
		defer deploymentsMu.Unlock()
		list := deploymentWatchers[id]
		next := list[:0]
		for _, c := range list {
			if c != ch {
				next = append(next, c)
				continue
			}
			close(c)
		}
		deploymentWatchers[id] = next
	}
	return ch, cancel
}

func createDeployment(req DeploymentCreateRequest) (Deployment, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.TemplateName = strings.TrimSpace(req.TemplateName)
	req.CustomerSlug = strings.TrimSpace(req.CustomerSlug)
	req.ProviderID = strings.TrimSpace(req.ProviderID)
	req.Estate = strings.TrimSpace(req.Estate)
	req.Visibility = strings.TrimSpace(req.Visibility)
	req.VLAN = strings.TrimSpace(req.VLAN)
	req.Subdomain = strings.TrimSpace(req.Subdomain)
	if req.TemplateName == "" {
		return Deployment{}, fmt.Errorf("template_name required")
	}
	tpl, ok := getCatalogTemplate(req.TemplateName)
	if !ok {
		return Deployment{}, fmt.Errorf("unknown template %q", req.TemplateName)
	}
	if req.Name == "" {
		req.Name = req.TemplateName
	}
	if req.Runtime != "vm" {
		req.Runtime = "ct"
	}
	if req.TargetHost == "" {
		if host := strings.TrimSpace(os.Getenv("WARDEN_DEFAULT_HOST")); host != "" {
			req.TargetHost = host
		}
	}
	if req.RequestedBy == "" {
		req.RequestedBy = "operator"
	}
	if req.RequestedBy == "operator" && req.CustomerSlug == "" {
		return Deployment{}, fmt.Errorf("customer_slug required for operator-triggered deployment")
	}
	if req.ProviderID == "" {
		req.ProviderID = "proxmox-x"
	}
	if req.Estate == "" {
		req.Estate = estateForTemplate(tpl)
	}
	req.Estate = normalizeEstate(req.Estate)
	if req.Visibility == "" {
		req.Visibility = visibilityForTemplate(tpl)
	}
	req.Visibility = normalizeVisibility(req.Visibility)
	if req.Estate == EstateMediastack && req.Visibility != VisibilityInternal {
		return Deployment{}, fmt.Errorf("mediastack deployments must remain internal by default")
	}
	if req.Subdomain == "" {
		req.Subdomain = sanitizeSubdomain(req.Name)
	}
	if req.Automation != "auto" {
		req.Automation = "guided"
	}

	id, err := newDeploymentID()
	if err != nil {
		return Deployment{}, err
	}
	now := time.Now().UTC()
	dep := Deployment{
		ID:            id,
		Name:          req.Name,
		CustomerSlug:  req.CustomerSlug,
		ProviderID:    req.ProviderID,
		Estate:        req.Estate,
		Visibility:    req.Visibility,
		VLAN:          req.VLAN,
		RequestedBy:   req.RequestedBy,
		TemplateName:  tpl.Name,
		TargetHost:    req.TargetHost,
		Runtime:       req.Runtime,
		Subdomain:     req.Subdomain,
		PublicURL:     "https://" + req.Subdomain + "." + defaultDeploymentDomain,
		Automation:    req.Automation,
		Approved:      false,
		Status:        DeploymentPending,
		Stage:         StagePending,
		CreatedAt:     now,
		UpdatedAt:     now,
		BestPractices: deploymentBestPractices(),
	}
	dep.Plan = buildDeploymentPlan(dep, tpl)
	dep.NextActions = []string{
		"Validate generated environment values before first deploy.",
		"Run preflight connectivity checks from Warden host to target runtime.",
		"Promote to auto mode only after one successful guided rollout.",
	}

	if err := setDeployment(dep); err != nil {
		return Deployment{}, err
	}
	appendDeploymentEvent(dep.ID, "info", string(StagePending), "deployment created")
	go runDeploymentPipeline(dep.ID)
	return dep, nil
}

func markDeploymentApproved(id, approvedBy, note string) (Deployment, error) {
	dep, ok := getDeployment(id)
	if !ok {
		return Deployment{}, fmt.Errorf("deployment %q not found", id)
	}
	dep.Approved = true
	dep.ApprovedBy = strings.TrimSpace(approvedBy)
	dep.ApprovalNote = strings.TrimSpace(note)
	dep.ApprovedAt = time.Now().UTC()
	dep.UpdatedAt = time.Now().UTC()
	if err := setDeployment(dep); err != nil {
		return Deployment{}, err
	}
	appendDeploymentEvent(id, "info", string(dep.Stage), "deployment approved for production automation")
	return dep, nil
}

func updateDeploymentStage(id string, status DeploymentStatus, stage DeploymentStage, msg string) error {
	dep, ok := getDeployment(id)
	if !ok {
		return fmt.Errorf("deployment %q not found", id)
	}
	dep.Status = status
	dep.Stage = stage
	dep.StageHistory = append(dep.StageHistory, string(stage))
	dep.UpdatedAt = time.Now().UTC()
	if err := setDeployment(dep); err != nil {
		return err
	}
	appendDeploymentEvent(id, "info", string(stage), msg)
	return nil
}

func failDeployment(id string, stage DeploymentStage, err error) {
	dep, ok := getDeployment(id)
	if !ok {
		return
	}
	dep.Status = DeploymentFailed
	dep.Stage = stage
	dep.LastError = err.Error()
	dep.UpdatedAt = time.Now().UTC()
	_ = setDeployment(dep)
	appendDeploymentEvent(id, "error", string(stage), err.Error())
}

func getCatalogTemplate(name string) (CatalogTemplate, bool) {
	inv := getCatalog()
	for _, t := range inv.Templates {
		if t.Name == name && t.ParseErr == "" {
			return t, true
		}
	}
	return CatalogTemplate{}, false
}

func newDeploymentID() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "dep-" + hex.EncodeToString(buf), nil
}

func sanitizeSubdomain(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.ReplaceAll(v, "_", "-")
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-':
			b.WriteRune(r)
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "deployment"
	}
	return s
}

func deploymentBestPractices() []string {
	return []string{
		"Use generated high-entropy secrets per deployment and rotate after bootstrap.",
		"Pin template version and capture immutable rollout metadata for audit/replay.",
		"Use staged promotion: guided rollout first, auto execution after verification.",
		"Apply health checks and timeout budgets before exposing public routes.",
		"Prefer least-privilege runtime defaults (unprivileged CT unless workload needs VM).",
	}
}

func buildDeploymentPlan(dep Deployment, tpl CatalogTemplate) []DeploymentPlanStep {
	steps := []DeploymentPlanStep{
		{
			Stage:       string(StagePrecheck),
			Title:       "Validate template and runtime fit",
			Rationale:   "Prevents partial deploys by failing fast on schema/runtime mismatches.",
			SafetyCheck: "Confirm template parsed without errors and runtime preference is explicit.",
		},
		{
			Stage:       string(StagePlan),
			Title:       "Render deployment plan and env contract",
			Rationale:   "Captures exact values, secrets, and resources before executing changes.",
			SafetyCheck: "Review generated config and diff against previous deployment.",
		},
	}
	if dep.Runtime == "ct" {
		steps = append(steps, DeploymentPlanStep{
			Stage:       string(StageExecute),
			Title:       "Provision target LXC runtime",
			Command:     "pct create <vmid> <template> --cores <n> --memory <mb> --unprivileged 1",
			Rationale:   "Fast, low-overhead runtime for most service stacks.",
			SafetyCheck: "Reserve VMID and verify bridge/storage availability first.",
		})
	} else {
		steps = append(steps, DeploymentPlanStep{
			Stage:       string(StageExecute),
			Title:       "Provision target VM runtime",
			Command:     "qm create <vmid> ... && qm start <vmid>",
			Rationale:   "Required for workloads needing custom kernel/modules.",
			SafetyCheck: "Confirm guest agent/cloud-init templates are available.",
		})
	}
	steps = append(steps, DeploymentPlanStep{
		Stage:       string(StageExecute),
		Title:       "Push compose and secrets",
		Command:     "docker compose pull && docker compose up -d",
		Rationale:   "Keeps deployment deterministic from template source.",
		SafetyCheck: fmt.Sprintf("Validate %d configurable values and %d secrets before apply.", len(tpl.Configurable), len(tpl.Secrets)),
	})
	steps = append(steps, DeploymentPlanStep{
		Stage:       string(StageVerify),
		Title:       "Health verify and route cutover",
		Command:     "curl -fsS https://" + dep.Subdomain + "." + defaultDeploymentDomain,
		Rationale:   "Blocks exposure until service is healthy.",
		SafetyCheck: "Only publish DNS/Caddy route after health checks pass and provider/tenant boundary tags are confirmed.",
	})
	return steps
}

func buildDeploymentReview(dep Deployment) (DeploymentReview, error) {
	tpl, ok := getCatalogTemplate(dep.TemplateName)
	if !ok {
		return DeploymentReview{}, fmt.Errorf("template %q not found", dep.TemplateName)
	}
	plan := dep.Plan
	if len(plan) == 0 {
		plan = buildDeploymentPlan(dep, tpl)
	}
	review := DeploymentReview{
		DeploymentID: dep.ID,
		Template:     dep.TemplateName,
		ProviderID:   dep.ProviderID,
		Estate:       dep.Estate,
		Visibility:   dep.Visibility,
		TargetHost:   dep.TargetHost,
		Runtime:      dep.Runtime,
		PublicURL:    dep.PublicURL,
		Status:       dep.Status,
		Stage:        dep.Stage,
		Approved:     dep.Approved,
		ApprovedBy:   dep.ApprovedBy,
		ApprovedAt:   dep.ApprovedAt,
		ApprovalNote: dep.ApprovalNote,
		Plan:         plan,
		NextActions:  dep.NextActions,
	}

	var script strings.Builder
	script.WriteString("#!/usr/bin/env bash\n")
	script.WriteString("set -euo pipefail\n\n")
	script.WriteString(fmt.Sprintf("# Deployment review for %s (%s)\n", dep.ID, dep.TemplateName))
	script.WriteString(fmt.Sprintf("echo \"Provider: %s | Host: %s | Runtime: %s\"\n", dep.ProviderID, dep.TargetHost, dep.Runtime))
	script.WriteString(fmt.Sprintf("echo \"Estate: %s | Visibility: %s\"\n", emptyOr(dep.Estate, EstateAIaaS), emptyOr(dep.Visibility, VisibilityInternal)))
	script.WriteString(fmt.Sprintf("echo \"Customer: %s | VLAN: %s\"\n", emptyOr(dep.CustomerSlug, "system"), emptyOr(dep.VLAN, "n/a")))
	script.WriteString(fmt.Sprintf("echo \"Public URL: %s\"\n\n", dep.PublicURL))
	for i, step := range plan {
		script.WriteString(fmt.Sprintf("echo \"[%d/%d] %s - %s\"\n", i+1, len(plan), step.Stage, step.Title))
		if strings.TrimSpace(step.Command) != "" {
			script.WriteString(fmt.Sprintf("# %s\n", step.Command))
		}
		if strings.TrimSpace(step.SafetyCheck) != "" {
			script.WriteString(fmt.Sprintf("# safety: %s\n", step.SafetyCheck))
		}
		script.WriteString("\n")
	}
	if len(dep.NextActions) > 0 {
		script.WriteString("echo \"Next actions:\"\n")
		for _, n := range dep.NextActions {
			script.WriteString(fmt.Sprintf("echo \" - %s\"\n", n))
		}
	}
	review.ReviewScript = script.String()
	return review, nil
}

func emptyOr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
