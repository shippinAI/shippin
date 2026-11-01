package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

func handleDeployments(w http.ResponseWriter, r *http.Request) {
	topTab := normalizeDeploymentsTopTab(r.URL.Query().Get("view"))
	subTab := normalizeDeploymentsSubTab(r.URL.Query().Get("tab"))
	allDeployments := listDeployments()
	topCounts := deploymentTopTabCounts(allDeployments)
	topFiltered := filterDeploymentsByTopTab(allDeployments, topTab)
	subCounts := deploymentSubTabCounts(topFiltered)
	filtered := filterDeploymentsBySubTab(topFiltered, subTab)
	data := DeploymentsPageData{
		HeaderData:  buildHeaderData(r),
		Deployments: filtered,
		TopTab:      topTab,
		SubTab:      subTab,
		TopCounts:   topCounts,
		SubCounts:   subCounts,
	}
	data.PageTitle = "Deployments"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "deployments.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleCustomersDeployments(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/deployments?view=customers&tab=active", http.StatusSeeOther)
}

func handleInternalDeployments(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/deployments?view=internal&tab=active", http.StatusSeeOther)
}

func handleDeploymentsNew(w http.ResponseWriter, r *http.Request) {
	inv := getCatalog()
	graph := getInfrastructureGraph(collectStats())
	providers := make([]string, 0, len(graph.Providers))
	for _, p := range graph.Providers {
		providers = append(providers, p.ID)
	}
	data := DeploymentNewPageData{
		HeaderData:       buildHeaderData(r),
		SelectedTemplate: strings.TrimSpace(r.URL.Query().Get("template")),
		Templates:        inv.Templates,
		DefaultHost:      strings.TrimSpace(os.Getenv("WARDEN_DEFAULT_HOST")),
		ProviderOptions:  providers,
	}
	data.PageTitle = "New Deployment"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "deployments_new.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleDeploymentDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/deployments/")
	if id == "" || id == "new" {
		http.NotFound(w, r)
		return
	}
	dep, ok := getDeployment(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	data := DeploymentDetailPageData{
		HeaderData: buildHeaderData(r),
		Deployment: dep,
		Events:     getDeploymentEvents(id),
	}
	data.PageTitle = "Deployment Detail"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "deployments_detail.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPIDeployments(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/deployments" {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"deployments": listDeployments()})
		case http.MethodPost:
			handleAPIDeploymentCreate(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/deployments/")
	if rest == r.URL.Path || rest == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rest, "/")
	id := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		dep, ok := getDeployment(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(dep)
		return
	}
	if len(parts) == 2 && parts[1] == "events" && r.Method == http.MethodGet {
		handleAPIDeploymentEvents(w, r, id)
		return
	}
	if len(parts) == 2 && parts[1] == "review" && r.Method == http.MethodGet {
		handleAPIDeploymentReview(w, r, id)
		return
	}
	if len(parts) == 2 && parts[1] == "action" && r.Method == http.MethodPost {
		handleAPIDeploymentAction(w, r, id)
		return
	}
	http.NotFound(w, r)
}

func handleAPIDeploymentCreate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	req := DeploymentCreateRequest{}
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "Invalid JSON: "+err.Error())
			return
		}
	} else {
		_ = r.ParseForm()
		req = DeploymentCreateRequest{
			Name:         strings.TrimSpace(r.FormValue("name")),
			CustomerSlug: strings.TrimSpace(r.FormValue("customer_slug")),
			ProviderID:   strings.TrimSpace(r.FormValue("provider_id")),
			Estate:       strings.TrimSpace(r.FormValue("estate")),
			Visibility:   strings.TrimSpace(r.FormValue("visibility")),
			VLAN:         strings.TrimSpace(r.FormValue("vlan")),
			RequestedBy:  strings.TrimSpace(r.FormValue("requested_by")),
			TemplateName: strings.TrimSpace(r.FormValue("template_name")),
			TargetHost:   strings.TrimSpace(r.FormValue("target_host")),
			Runtime:      strings.TrimSpace(r.FormValue("runtime")),
			Subdomain:    strings.TrimSpace(r.FormValue("subdomain")),
			Automation:   strings.TrimSpace(r.FormValue("automation")),
		}
	}
	dep, err := createDeployment(req)
	if err != nil {
		jsonError(w, err.Error())
		return
	}
	auditFromRequest(r, "deployment_create", "Created deployment "+dep.ID+" from template "+dep.TemplateName, "deployments")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":         true,
		"deployment": dep,
		"detail_url": "/deployments/" + dep.ID,
	})
}

func handleAPIDeploymentEvents(w http.ResponseWriter, r *http.Request, id string) {
	if r.URL.Query().Get("stream") == "1" {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "stream unsupported", http.StatusInternalServerError)
			return
		}
		for _, ev := range getDeploymentEvents(id) {
			data, _ := json.Marshal(ev)
			_, _ = w.Write([]byte("data: " + string(data) + "\n\n"))
		}
		flusher.Flush()
		ch, cancel := watchDeploymentEvents(id)
		defer cancel()
		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-ch:
				data, _ := json.Marshal(ev)
				_, _ = w.Write([]byte("data: " + string(data) + "\n\n"))
				flusher.Flush()
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"events": getDeploymentEvents(id)})
}

func handleAPIDeploymentAction(w http.ResponseWriter, r *http.Request, id string) {
	dep, ok := getDeployment(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		Action string `json:"action"`
		Note   string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid JSON: "+err.Error())
		return
	}
	switch req.Action {
	case "retry":
		if dep.Status != DeploymentFailed && dep.Status != DeploymentNeedsRetry {
			jsonError(w, "deployment is not in a retryable state")
			return
		}
		appendDeploymentEvent(id, "info", string(dep.Stage), "retry requested")
		go runDeploymentPipeline(id)
	case "cancel":
		dep.Status = DeploymentCancelled
		dep.UpdatedAt = time.Now().UTC()
		if err := setDeployment(dep); err != nil {
			jsonError(w, err.Error())
			return
		}
		appendDeploymentEvent(id, "warn", string(dep.Stage), "deployment cancelled by operator")
	case "approve_production":
		approver, _ := getSession(r)
		updated, err := markDeploymentApproved(id, approver, req.Note)
		if err != nil {
			jsonError(w, err.Error())
			return
		}
		auditFromRequest(r, "deployment_approve_production", "Approved deployment "+id+" for automated execution", "deployments")
		if updated.Automation == "auto" {
			appendDeploymentEvent(id, "info", string(updated.Stage), "resuming auto rollout after approval")
			go runDeploymentPipeline(id)
		}
	default:
		jsonError(w, "unknown action")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func handleAPIDeploymentReview(w http.ResponseWriter, r *http.Request, id string) {
	dep, ok := getDeployment(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	review, err := buildDeploymentReview(dep)
	if err != nil {
		jsonError(w, err.Error())
		return
	}
	if strings.EqualFold(r.URL.Query().Get("format"), "shell") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(review.ReviewScript))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(review)
}

func normalizeDeploymentsTopTab(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "customers":
		return "customers"
	case "internal":
		return "internal"
	default:
		return "deployments"
	}
}

func normalizeDeploymentsSubTab(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "pending":
		return "pending"
	case "backups":
		return "backups"
	case "recovery":
		return "recovery"
	case "migrations":
		return "migrations"
	default:
		return "active"
	}
}

func deploymentTopTabCounts(deps []Deployment) map[string]int {
	counts := map[string]int{
		"deployments": len(deps),
		"customers":   0,
		"internal":    0,
	}
	for _, dep := range deps {
		if strings.TrimSpace(dep.CustomerSlug) == "" {
			counts["internal"]++
		} else {
			counts["customers"]++
		}
	}
	return counts
}

func deploymentSubTabCounts(deps []Deployment) map[string]int {
	counts := map[string]int{
		"active":     0,
		"pending":    0,
		"backups":    0,
		"recovery":   0,
		"migrations": 0,
	}
	for _, dep := range deps {
		if isActiveDeployment(dep) {
			counts["active"]++
		}
		if isPendingOrRolloutDeployment(dep) {
			counts["pending"]++
		}
		if deploymentHasBackupPolicy(dep) {
			counts["backups"]++
		}
		if deploymentHasRecoveryProfile(dep) {
			counts["recovery"]++
		}
		if isMigrationDeployment(dep) {
			counts["migrations"]++
		}
	}
	return counts
}

func filterDeploymentsByTopTab(deps []Deployment, tab string) []Deployment {
	out := make([]Deployment, 0, len(deps))
	for _, dep := range deps {
		customerOwned := strings.TrimSpace(dep.CustomerSlug) != ""
		switch tab {
		case "customers":
			if customerOwned {
				out = append(out, dep)
			}
		case "internal":
			if !customerOwned {
				out = append(out, dep)
			}
		default:
			out = append(out, dep)
		}
	}
	return out
}

func filterDeploymentsBySubTab(deps []Deployment, tab string) []Deployment {
	out := make([]Deployment, 0, len(deps))
	for _, dep := range deps {
		include := false
		switch tab {
		case "pending":
			include = isPendingOrRolloutDeployment(dep)
		case "backups":
			include = deploymentHasBackupPolicy(dep)
		case "recovery":
			include = deploymentHasRecoveryProfile(dep)
		case "migrations":
			include = isMigrationDeployment(dep)
		default:
			include = isActiveDeployment(dep)
		}
		if include {
			out = append(out, dep)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func isActiveDeployment(dep Deployment) bool {
	return dep.Status == DeploymentRunning || dep.Status == DeploymentHealthy
}

func isPendingOrRolloutDeployment(dep Deployment) bool {
	switch dep.Status {
	case DeploymentPending, DeploymentRunning, DeploymentAwaiting, DeploymentNeedsRetry:
		return true
	default:
		return false
	}
}

func deploymentHasBackupPolicy(dep Deployment) bool {
	tpl, ok := getCatalogTemplate(dep.TemplateName)
	if !ok {
		return false
	}
	return len(tpl.Backup) > 0 || len(tpl.Backups) > 0 || len(tpl.BackupPolicy) > 0
}

func deploymentHasRecoveryProfile(dep Deployment) bool {
	tpl, ok := getCatalogTemplate(dep.TemplateName)
	if !ok {
		return false
	}
	return len(tpl.Recovery) > 0 || len(tpl.RecoveryProfile) > 0
}

func isMigrationDeployment(dep Deployment) bool {
	stage := strings.ToLower(string(dep.Stage))
	if strings.Contains(stage, "migrat") || strings.Contains(stage, "promot") {
		return true
	}
	for _, ev := range getDeploymentEvents(dep.ID) {
		msg := strings.ToLower(ev.Message)
		if strings.Contains(msg, "migrat") || strings.Contains(msg, "promot") {
			return true
		}
	}
	return false
}
