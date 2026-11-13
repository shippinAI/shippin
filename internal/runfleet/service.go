package runfleet

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Options configures a control plane. Zero values get the published defaults.
type Options struct {
	Now              func() time.Time
	GitHub           GitHub
	Provisioner      Provisioner
	Sizes            []Size
	FreeMinutes      int
	FreeSize         string
	DefaultRetention int
	AutoApproveOSS   *bool
}

// Service is the runner control plane. HTTP and MCP both call it.
type Service struct {
	dir              string
	now              func() time.Time
	gh               GitHub
	provision        Provisioner
	sizes            []Size
	freeMinutes      int
	freeSize         string
	defaultRetention int
	autoOSS          bool
}

// Open creates the state directory and a control plane bound to it.
func Open(dir string, opt Options) (*Service, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("state directory is required")
	}
	dir = filepath.Clean(dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Service{
		dir: dir, now: opt.Now, gh: opt.GitHub, provision: opt.Provisioner,
		sizes: opt.Sizes, freeMinutes: opt.FreeMinutes, freeSize: opt.FreeSize,
		defaultRetention: opt.DefaultRetention, autoOSS: true,
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.provision == nil {
		s.provision = Unconfigured{}
	}
	if len(s.sizes) == 0 {
		s.sizes = DefaultSizes()
	}
	switch {
	case opt.FreeMinutes < 0:
		s.freeMinutes = 0
	case opt.FreeMinutes == 0:
		s.freeMinutes = 3000
	default:
		s.freeMinutes = opt.FreeMinutes
	}
	if s.freeSize == "" {
		s.freeSize = "linux-x64-2"
	}
	if s.defaultRetention == 0 {
		s.defaultRetention = 14
	}
	if opt.AutoApproveOSS != nil {
		s.autoOSS = *opt.AutoApproveOSS
	}
	if _, err := loadDisk(dir); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) Sizes() []Size { return append([]Size(nil), s.sizes...) }

func (s *Service) withDisk(write bool, fn func(*disk) error) error {
	f, err := os.OpenFile(filepath.Join(s.dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := lockFile(f); err != nil {
		return err
	}
	defer unlockFile(f)
	d, err := loadDisk(s.dir)
	if err != nil {
		return err
	}
	if err := fn(d); err != nil {
		return err
	}
	if write {
		return saveDisk(s.dir, d)
	}
	return nil
}

// ConnectInput asks the control plane to adopt repositories.
type ConnectInput struct {
	Account          string     `json:"account"`
	AccountKind      string     `json:"account_kind"`
	Repos            []string   `json:"repos"`
	Size             string     `json:"size"`
	Registration     string     `json:"registration,omitempty"`
	Facts            []RepoFact `json:"facts,omitempty"`
	ApplyWorkflow    bool       `json:"apply_workflow,omitempty"`
	LogRetentionDays *int       `json:"log_retention_days,omitempty"`
}

// RepoConnect is the per-repository result.
type RepoConnect struct {
	Repo           string           `json:"repo"`
	Approval       string           `json:"approval"`
	ApprovalReason string           `json:"approval_reason"`
	Lovable        bool             `json:"lovable"`
	Runner         *Runner          `json:"runner,omitempty"`
	Workflow       *WorkflowPlan    `json:"workflow,omitempty"`
	Provision      ProvisionOutcome `json:"provision"`
}

// ConnectResult is the account-level onboarding result.
type ConnectResult struct {
	ConnectionID     string        `json:"connection_id"`
	Account          string        `json:"account"`
	AccountKind      string        `json:"account_kind"`
	Registration     string        `json:"registration"`
	RegistrationNote string        `json:"registration_note,omitempty"`
	Size             string        `json:"size"`
	RunsOn           string        `json:"runs_on"`
	GitHub           string        `json:"github"`
	Repos            []RepoConnect `json:"repos"`
}

// Connect records onboarding. Guest creation stays on the provisioner, which
// reports itself when it cannot create a machine. Workflow edits are a dry-run
// unless ApplyWorkflow is set.
func (s *Service) Connect(ctx context.Context, in ConnectInput) (ConnectResult, error) {
	size, ok := findSize(s.sizes, in.Size)
	if !ok {
		return ConnectResult{}, fmt.Errorf("unknown size %q", in.Size)
	}
	if !validAccount(in.Account) {
		return ConnectResult{}, fmt.Errorf("account %q is not a GitHub account name", in.Account)
	}
	if len(in.Repos) == 0 {
		return ConnectResult{}, fmt.Errorf("at least one repository is required")
	}
	level, note, err := RegistrationLevel(in.AccountKind, in.Registration)
	if err != nil {
		return ConnectResult{}, err
	}
	retention := s.defaultRetention
	if in.LogRetentionDays != nil {
		if *in.LogRetentionDays < 0 || *in.LogRetentionDays > 3650 {
			return ConnectResult{}, fmt.Errorf("log_retention_days must be from 0 to 3650")
		}
		retention = *in.LogRetentionDays
	}
	facts := map[string]RepoFact{}
	for _, fact := range in.Facts {
		facts[fact.FullName] = fact
	}
	githubState := "not_configured"
	if s.gh != nil && s.gh.Configured() {
		githubState = "configured"
	}
	resolved := make([]RepoFact, 0, len(in.Repos))
	for _, repo := range in.Repos {
		if !validRepo(repo) {
			return ConnectResult{}, fmt.Errorf("repository %q is not owner/name", repo)
		}
		fact, err := s.resolveRepo(ctx, repo, facts)
		if err != nil {
			return ConnectResult{}, err
		}
		resolved = append(resolved, fact)
	}
	plans := make([]WorkflowPlan, len(resolved))
	for i, fact := range resolved {
		plans[i] = PlanRunsOn(fact.Workflows, size.Label, fact.Lovable, len(fact.Workflows) == 0)
		if in.ApplyWorkflow && len(plans[i].Changes) > 0 {
			if s.gh == nil || !s.gh.Configured() {
				return ConnectResult{}, fmt.Errorf("apply_workflow requires a configured GitHub App")
			}
			url, err := s.gh.OpenPull(ctx, fact.FullName, "Use Shippin Run", pullBody(size.Label), "shippin-run/"+newID(), plans[i].Changes)
			if err != nil {
				return ConnectResult{}, err
			}
			plans[i].DryRun = false
			plans[i].PullURL = url
		}
	}
	result := ConnectResult{
		Account: in.Account, AccountKind: in.AccountKind, Registration: level,
		RegistrationNote: note, Size: size.ID, RunsOn: size.Label, GitHub: githubState,
	}
	err = s.withDisk(true, func(d *disk) error {
		conn := Connection{
			ID: newID(), Account: in.Account, AccountKind: in.AccountKind,
			Registration: level, Size: size.ID, CreatedAt: s.now().UTC(),
		}
		var orgRepos []string
		var orgRunner *Runner
		for i, fact := range resolved {
			status, reason := Approval(fact, s.autoOSS)
			row := ConnectedRepo{
				FullName: fact.FullName, Private: fact.Private, License: fact.License,
				Lovable: fact.Lovable, Approval: status, ApprovalReason: reason,
				RetentionDays: retention,
			}
			item := RepoConnect{
				Repo: fact.FullName, Approval: status, ApprovalReason: reason,
				Lovable: fact.Lovable, Workflow: &plans[i],
			}
			if status != "auto" {
				item.Provision = skippedProvision(s.provision, "waiting for approval")
				conn.Repos = append(conn.Repos, row)
				result.Repos = append(result.Repos, item)
				continue
			}
			if level == "organization" {
				orgRepos = append(orgRepos, fact.FullName)
				if orgRunner == nil {
					runner, outcome := s.makeRunner(ctx, size, in.Account, in.AccountKind, nil)
					orgRunner = &runner
					item.Provision = outcome
				} else {
					item.Provision = ProvisionOutcome{
						Implemented: false, Provider: s.provision.Name(),
						Message: "this repository shares the organization runner",
					}
				}
				row.RunnerID = orgRunner.ID
				item.Runner = orgRunner
			} else {
				runner, outcome := s.makeRunner(ctx, size, in.Account, in.AccountKind, []string{fact.FullName})
				d.Runners = append(d.Runners, runner)
				row.RunnerID = runner.ID
				item.Runner = &runner
				item.Provision = outcome
			}
			conn.Repos = append(conn.Repos, row)
			result.Repos = append(result.Repos, item)
		}
		if orgRunner != nil {
			orgRunner.Repos = append([]string(nil), orgRepos...)
			for i := range result.Repos {
				if result.Repos[i].Runner != nil && result.Repos[i].Runner.ID == orgRunner.ID {
					result.Repos[i].Runner = orgRunner
				}
			}
			for i := range conn.Repos {
				if conn.Repos[i].RunnerID == orgRunner.ID {
					// runner id already set
				}
			}
			d.Runners = append(d.Runners, *orgRunner)
		}
		d.Connections = append(d.Connections, conn)
		result.ConnectionID = conn.ID
		return nil
	})
	return result, err
}

func (s *Service) resolveRepo(ctx context.Context, repo string, supplied map[string]RepoFact) (RepoFact, error) {
	if s.gh != nil && s.gh.Configured() {
		fact, err := s.gh.Repo(ctx, repo)
		if err != nil {
			return RepoFact{}, err
		}
		if fact.FullName == "" {
			fact.FullName = repo
		}
		return fact, nil
	}
	fact, ok := supplied[repo]
	if !ok {
		return RepoFact{}, fmt.Errorf("repository %s needs facts when the GitHub App is not configured", repo)
	}
	fact.FullName = repo
	return fact, nil
}

func (s *Service) makeRunner(ctx context.Context, size Size, account, kind string, repos []string) (Runner, ProvisionOutcome) {
	id := newID()
	runner := Runner{
		ID: id, Name: "shippin-" + size.ID + "-" + id[:8],
		Repos: append([]string(nil), repos...), Size: size.ID, Labels: []string{size.Label},
		Account: account, AccountKind: kind, Status: "desired",
	}
	outcome := outcomeFromProvision(s.provision, ctx, Spec{
		RunnerID: runner.ID, Name: runner.Name, Size: size, Repos: repos,
		Account: account, AccountKind: kind,
	})
	runner.ProvisionDetail = outcome.Message
	if outcome.Machine != nil {
		runner.Kind = outcome.Machine.Kind
		runner.VMID = outcome.Machine.VMID
		runner.Host = outcome.Machine.Host
		runner.Status = "registered"
	}
	return runner, outcome
}

func pullBody(label string) string {
	return "Sets runs-on to " + label + ".\n\nShippin Run opens this pull request only when apply is set. Guest creation is a separate step and may still be unavailable."
}

// ApproveInput lets an operator release a pending repository.
type ApproveInput struct {
	ConnectionID  string `json:"connection_id"`
	Repo          string `json:"repo"`
	Reason        string `json:"reason"`
	ApplyWorkflow bool   `json:"apply_workflow,omitempty"`
}

// Approve records an operator approval and then asks the provisioner for a guest.
func (s *Service) Approve(ctx context.Context, in ApproveInput) (RepoConnect, error) {
	reason := strings.TrimSpace(in.Reason)
	if reason == "" || len(reason) > 500 {
		return RepoConnect{}, fmt.Errorf("reason must be 1 to 500 characters")
	}
	var item RepoConnect
	err := s.withDisk(true, func(d *disk) error {
		conn, repo, err := findRepo(d, in.ConnectionID, in.Repo)
		if err != nil {
			return err
		}
		sz, ok := findSize(s.sizes, conn.Size)
		if !ok {
			return fmt.Errorf("unknown size %q", conn.Size)
		}
		repo.Approval = "approved"
		repo.ApprovalReason = reason
		var runner Runner
		var outcome ProvisionOutcome
		if conn.Registration == "organization" {
			if existing := runnerByAccount(d, conn.Account); existing != nil {
				if !contains(existing.Repos, repo.FullName) {
					existing.Repos = append(existing.Repos, repo.FullName)
				}
				repo.RunnerID = existing.ID
				runner = *existing
				outcome = ProvisionOutcome{Implemented: false, Provider: s.provision.Name(), Message: "added to the existing organization runner"}
			}
		}
		if repo.RunnerID == "" {
			runner, outcome = s.makeRunner(ctx, sz, conn.Account, conn.AccountKind, []string{repo.FullName})
			d.Runners = append(d.Runners, runner)
			repo.RunnerID = runner.ID
		}
		replaceRepo(conn, repo)
		item = RepoConnect{
			Repo: repo.FullName, Approval: repo.Approval, ApprovalReason: repo.ApprovalReason,
			Lovable: repo.Lovable, Runner: &runner, Provision: outcome,
		}
		return nil
	})
	return item, err
}

func findRepo(d *disk, connID, repo string) (*Connection, *ConnectedRepo, error) {
	for i := range d.Connections {
		if d.Connections[i].ID != connID {
			continue
		}
		for j := range d.Connections[i].Repos {
			if d.Connections[i].Repos[j].FullName == repo {
				return &d.Connections[i], &d.Connections[i].Repos[j], nil
			}
		}
		return nil, nil, fmt.Errorf("repository is not on this connection")
	}
	return nil, nil, fmt.Errorf("connection not found")
}

func replaceRepo(conn *Connection, repo *ConnectedRepo) {
	for i := range conn.Repos {
		if conn.Repos[i].FullName == repo.FullName {
			conn.Repos[i] = *repo
		}
	}
}

func runnerByAccount(d *disk, account string) *Runner {
	for i := range d.Runners {
		if d.Runners[i].Account == account && d.Runners[i].AccountKind == "organization" {
			return &d.Runners[i]
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

// ListRunners returns the fleet file without probing.
func (s *Service) ListRunners() ([]Runner, error) {
	var out []Runner
	err := s.withDisk(false, func(d *disk) error {
		out = append([]Runner(nil), d.Runners...)
		return nil
	})
	return out, err
}

// RunnerHealth probes one runner. Missing GitHub credentials and a missing
// guest are unknown, not a failure of the whole call.
func (s *Service) RunnerHealth(ctx context.Context, id string) (HealthReport, error) {
	var runner Runner
	var repos []string
	err := s.withDisk(false, func(d *disk) error {
		for _, r := range d.Runners {
			if r.ID == id || r.Name == id {
				runner = r
				repos = append([]string(nil), r.Repos...)
				return nil
			}
		}
		return fmt.Errorf("runner not found")
	})
	if err != nil {
		return HealthReport{}, err
	}
	runner.Repos = repos
	configured := s.gh != nil && s.gh.Configured()
	var states map[string]string
	if configured && len(repos) > 0 {
		states, err = s.gh.RunnerStates(ctx, repos)
		if err != nil {
			states = nil
			configured = false
		}
	}
	gh := githubSignal(runner, states, s.gh != nil && s.gh.Configured() && err == nil)
	if s.gh != nil && s.gh.Configured() && err != nil {
		gh = Signal{State: HealthUnknown, Detail: "GitHub probe failed"}
	}
	sub, load := s.provision.Health(ctx, runner)
	return HealthReport{
		Runner: runner, GitHub: gh, Substrate: sub, HostLoad: load,
		Overall: Compose(gh.State, sub.State, load.State), CheckedAt: s.now().UTC(),
	}, nil
}

func githubSignal(runner Runner, states map[string]string, configured bool) Signal {
	if !configured {
		return Signal{State: HealthUnknown, Detail: "GitHub credentials are not configured"}
	}
	if len(runner.Repos) == 0 {
		return Signal{State: HealthUnknown, Detail: "runner has no repositories"}
	}
	worst := HealthOK
	details := make([]string, 0, len(runner.Repos))
	for _, repo := range runner.Repos {
		st, ok := states[repo+"|"+runner.Name]
		if !ok {
			st = "unregistered"
		}
		details = append(details, repo+": "+st)
		if !strings.HasPrefix(st, "online") {
			worst = HealthCritical
		}
	}
	return Signal{State: worst, Detail: strings.Join(details, "; ")}
}

// ListJobs returns jobs for a repository, newest first, without log bodies.
func (s *Service) ListJobs(repo string) ([]JobSummary, error) {
	var out []JobSummary
	err := s.withDisk(false, func(d *disk) error {
		for _, job := range d.Jobs {
			if repo == "" || job.Repo == repo {
				out = append(out, job.Summary())
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].FinishedAt.After(out[j].FinishedAt) })
	if out == nil {
		out = []JobSummary{}
	}
	return out, err
}

// LogResult is a job log after redaction and retention.
type LogResult struct {
	JobID      string `json:"job_id"`
	Repo       string `json:"repo"`
	Log        string `json:"log"`
	Redactions int    `json:"redactions"`
	Stored     bool   `json:"stored"`
	Expired    bool   `json:"expired"`
	Message    string `json:"message,omitempty"`
}

// JobLogs returns the stored log, redacted again on read.
func (s *Service) JobLogs(id string) (LogResult, error) {
	var job Job
	var days int
	var found bool
	err := s.withDisk(false, func(d *disk) error {
		for _, item := range d.Jobs {
			if item.ID == id {
				job = item
				days = s.retentionFor(d, item.Repo)
				found = true
				return nil
			}
		}
		return fmt.Errorf("job not found")
	})
	if err != nil || !found {
		return LogResult{}, err
	}
	res := LogResult{JobID: job.ID, Repo: job.Repo, Redactions: job.Redactions}
	when := job.FinishedAt
	if when.IsZero() {
		when = job.StartedAt
	}
	if days == 0 || (!when.IsZero() && s.now().After(when.Add(time.Duration(days)*24*time.Hour))) {
		res.Expired = true
		res.Message = "logs were discarded under this repository's retention"
		return res, nil
	}
	if !job.LogStored {
		res.Message = "no log is stored for this job"
		return res, nil
	}
	text, n := RedactLog(job.Log)
	res.Log = text
	res.Redactions += n
	res.Stored = true
	return res, nil
}

func (s *Service) retentionFor(d *disk, repo string) int {
	for _, conn := range d.Connections {
		for _, row := range conn.Repos {
			if row.FullName == repo {
				return row.RetentionDays
			}
		}
	}
	return s.defaultRetention
}

// Explain returns a structured reading of one job.
func (s *Service) Explain(id string) (Explanation, error) {
	logs, err := s.JobLogs(id)
	if err != nil {
		return Explanation{}, err
	}
	var job Job
	err = s.withDisk(false, func(d *disk) error {
		for _, item := range d.Jobs {
			if item.ID == id {
				job = item
				return nil
			}
		}
		return fmt.Errorf("job not found")
	})
	if err != nil {
		return Explanation{}, err
	}
	job.Redactions = logs.Redactions
	return ExplainLog(job, logs.Log, logs.Stored, logs.Expired), nil
}

// IngestInput is a job log pushed by an operator or a future runner agent.
type IngestInput struct {
	Repo        string    `json:"repo"`
	Name        string    `json:"name"`
	Conclusion  string    `json:"conclusion"`
	RunnerName  string    `json:"runner_name,omitempty"`
	Log         string    `json:"log,omitempty"`
	HTMLURL     string    `json:"html_url,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
	GitHubJobID int64     `json:"github_job_id,omitempty"`
}

// IngestJob stores a job for a connected repository. The log is redacted first.
// A retention of zero drops the log body.
func (s *Service) IngestJob(in IngestInput) (JobSummary, error) {
	if !validRepo(in.Repo) {
		return JobSummary{}, fmt.Errorf("repository %q is not owner/name", in.Repo)
	}
	var summary JobSummary
	err := s.withDisk(true, func(d *disk) error {
		if !repoConnected(d, in.Repo) {
			return fmt.Errorf("repository is not connected")
		}
		job := s.prepareJob(d, Job{
			Repo: in.Repo, Name: in.Name, Conclusion: in.Conclusion, RunnerName: in.RunnerName,
			StartedAt: in.StartedAt, FinishedAt: in.FinishedAt, Log: in.Log, HTMLURL: in.HTMLURL,
			GitHubJobID: in.GitHubJobID,
		})
		upsertJob(d, job)
		summary = job.Summary()
		return nil
	})
	return summary, err
}

func (s *Service) prepareJob(d *disk, job Job) Job {
	text, n := RedactLog(job.Log)
	job.Redactions = n
	if s.retentionFor(d, job.Repo) == 0 || strings.TrimSpace(text) == "" {
		job.Log = ""
		job.LogStored = false
		return ensureJobID(job)
	}
	job.Log = text
	job.LogStored = true
	return ensureJobID(job)
}

func ensureJobID(job Job) Job {
	if job.ID == "" {
		job.ID = newID()
	}
	return job
}

func repoConnected(d *disk, repo string) bool {
	for _, conn := range d.Connections {
		for _, row := range conn.Repos {
			if row.FullName == repo {
				return true
			}
		}
	}
	return false
}

func upsertJob(d *disk, job Job) {
	for i, existing := range d.Jobs {
		if job.GitHubJobID != 0 && existing.GitHubJobID == job.GitHubJobID && existing.Repo == job.Repo {
			if existing.LogStored && !job.LogStored {
				job.Log = existing.Log
				job.LogStored = true
				job.Redactions = existing.Redactions
			}
			job.ID = existing.ID
			d.Jobs[i] = job
			return
		}
	}
	d.Jobs = append(d.Jobs, job)
}

// RunsOnInput edits workflow files. Apply defaults to false.
type RunsOnInput struct {
	Repo      string         `json:"repo"`
	Size      string         `json:"size"`
	Apply     bool           `json:"apply"`
	Workflows []WorkflowFile `json:"workflows,omitempty"`
	Lovable   bool           `json:"lovable,omitempty"`
}

// SetRunsOn previews a runs-on edit. Apply opens a pull request.
func (s *Service) SetRunsOn(ctx context.Context, in RunsOnInput) (WorkflowPlan, error) {
	size, ok := findSize(s.sizes, in.Size)
	if !ok {
		return WorkflowPlan{}, fmt.Errorf("unknown size %q", in.Size)
	}
	if !validRepo(in.Repo) {
		return WorkflowPlan{}, fmt.Errorf("repository %q is not owner/name", in.Repo)
	}
	files := in.Workflows
	lovable := in.Lovable
	if s.gh != nil && s.gh.Configured() {
		fact, err := s.gh.Repo(ctx, in.Repo)
		if err != nil {
			return WorkflowPlan{}, err
		}
		files = fact.Workflows
		lovable = fact.Lovable
	} else if files == nil && !lovable {
		return WorkflowPlan{}, fmt.Errorf("workflows are required when the GitHub App is not configured")
	}
	plan := PlanRunsOn(files, size.Label, lovable, len(files) == 0)
	if !in.Apply {
		return plan, nil
	}
	if len(plan.Changes) == 0 {
		return plan, fmt.Errorf("nothing to apply")
	}
	if s.gh == nil || !s.gh.Configured() {
		return WorkflowPlan{}, fmt.Errorf("apply requires a configured GitHub App")
	}
	url, err := s.gh.OpenPull(ctx, in.Repo, "Use Shippin Run", pullBody(size.Label), "shippin-run/"+newID(), plan.Changes)
	if err != nil {
		return WorkflowPlan{}, err
	}
	plan.DryRun = false
	plan.PullURL = url
	return plan, nil
}

// SetRetention sets how many days a repository keeps logs. Zero discards logs.
func (s *Service) SetRetention(repo string, days int) error {
	if !validRepo(repo) {
		return fmt.Errorf("repository %q is not owner/name", repo)
	}
	if days < 0 || days > 3650 {
		return fmt.Errorf("log_retention_days must be from 0 to 3650")
	}
	return s.withDisk(true, func(d *disk) error {
		for i := range d.Connections {
			for j := range d.Connections[i].Repos {
				if d.Connections[i].Repos[j].FullName == repo {
					d.Connections[i].Repos[j].RetentionDays = days
					return nil
				}
			}
		}
		return fmt.Errorf("repository is not connected")
	})
}

// Usage is the meter for one account and UTC month (YYYY-MM). It does not collect payment.
type Usage struct {
	Account          string    `json:"account"`
	Period           string    `json:"period"`
	FreeMinutes      int       `json:"free_minutes"`
	FreeSize         string    `json:"free_size"`
	BySize           []SizeUse `json:"by_size"`
	BillableMicroUSD int64     `json:"billable_micro_usd"`
	BillableUSD      string    `json:"billable_usd"`
	Note             string    `json:"note"`
}

// SizeUse is metered time for one size.
type SizeUse struct {
	Size            string `json:"size"`
	Seconds         int64  `json:"seconds"`
	BillableSeconds int64  `json:"billable_seconds"`
	MicroUSD        int64  `json:"micro_usd"`
}

// Usage sums job durations. Free minutes apply only to FreeSize.
func (s *Service) Usage(account, period string) (Usage, error) {
	if !validAccount(account) {
		return Usage{}, fmt.Errorf("account %q is not a GitHub account name", account)
	}
	if period == "" {
		period = s.now().UTC().Format("2006-01")
	}
	start, err := time.Parse("2006-01", period)
	if err != nil {
		return Usage{}, fmt.Errorf("period must be YYYY-MM")
	}
	end := start.AddDate(0, 1, 0)
	var out Usage
	err = s.withDisk(false, func(d *disk) error {
		repos := map[string]string{}
		sizesByRunner := map[string]string{}
		for _, r := range d.Runners {
			if account == "" || r.Account == account {
				sizesByRunner[r.Name] = r.Size
			}
		}
		for _, conn := range d.Connections {
			if conn.Account != account {
				continue
			}
			for _, repo := range conn.Repos {
				repos[repo.FullName] = conn.Account
			}
		}
		seconds := map[string]int64{}
		for _, job := range d.Jobs {
			owner, ok := repos[job.Repo]
			if !ok || owner != account {
				continue
			}
			when := job.StartedAt
			if when.IsZero() || when.Before(start) || !when.Before(end) {
				continue
			}
			sizeID := sizesByRunner[job.RunnerName]
			if sizeID == "" {
				sizeID = "unmetered"
			}
			seconds[sizeID] += billedSeconds(job.StartedAt, job.FinishedAt)
		}
		freeLeft := int64(s.freeMinutes) * 60
		ids := make([]string, 0, len(seconds))
		for id := range seconds {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			sec := seconds[id]
			billable := sec
			var price int64
			if sz, ok := findSize(s.sizes, id); ok {
				price = sz.MicroUSDPerMinute
			}
			if id == s.freeSize {
				used := freeLeft
				if used > billable {
					used = billable
				}
				billable -= used
				freeLeft -= used
			}
			micro := billable * price / 60
			out.BySize = append(out.BySize, SizeUse{Size: id, Seconds: sec, BillableSeconds: billable, MicroUSD: micro})
			out.BillableMicroUSD += micro
		}
		if out.BySize == nil {
			out.BySize = []SizeUse{}
		}
		return nil
	})
	out.Account = account
	out.Period = period
	out.FreeMinutes = s.freeMinutes
	out.FreeSize = s.freeSize
	out.BillableUSD = formatUSD(out.BillableMicroUSD)
	out.Note = "This meter does not collect payment."
	return out, err
}

func billedSeconds(start, end time.Time) int64 {
	if end.IsZero() || !end.After(start) {
		return 0
	}
	ms := end.Sub(start).Milliseconds()
	return (ms + 999) / 1000
}

// HandleWebhook accepts a signed GitHub event body. The caller already checked the signature.
func (s *Service) HandleWebhook(event string, body []byte) error {
	switch event {
	case "installation":
		var payload struct {
			Action       string `json:"action"`
			Installation struct {
				ID      int64 `json:"id"`
				Account struct {
					Login string `json:"login"`
					Type  string `json:"type"`
				} `json:"account"`
			} `json:"installation"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return fmt.Errorf("installation payload")
		}
		kind := "user"
		if strings.EqualFold(payload.Installation.Account.Type, "Organization") {
			kind = "organization"
		}
		return s.withDisk(true, func(d *disk) error {
			next := d.Installations[:0]
			for _, inst := range d.Installations {
				if inst.ID != payload.Installation.ID {
					next = append(next, inst)
				}
			}
			d.Installations = next
			if payload.Action == "deleted" {
				return nil
			}
			if payload.Action == "created" || payload.Action == "" {
				d.Installations = append(d.Installations, Installation{
					ID: payload.Installation.ID, Account: payload.Installation.Account.Login, AccountKind: kind,
				})
			}
			return nil
		})
	case "workflow_job":
		var payload struct {
			Action     string `json:"action"`
			Repository struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
			WorkflowJob struct {
				ID          int64     `json:"id"`
				Name        string    `json:"name"`
				Conclusion  string    `json:"conclusion"`
				RunnerName  string    `json:"runner_name"`
				HTMLURL     string    `json:"html_url"`
				StartedAt   time.Time `json:"started_at"`
				CompletedAt time.Time `json:"completed_at"`
			} `json:"workflow_job"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return fmt.Errorf("workflow_job payload")
		}
		if payload.Action != "completed" {
			return nil
		}
		return s.withDisk(true, func(d *disk) error {
			if !repoConnected(d, payload.Repository.FullName) {
				return nil
			}
			job := s.prepareJob(d, Job{
				Repo: payload.Repository.FullName, Name: payload.WorkflowJob.Name,
				Conclusion: payload.WorkflowJob.Conclusion, RunnerName: payload.WorkflowJob.RunnerName,
				StartedAt: payload.WorkflowJob.StartedAt, FinishedAt: payload.WorkflowJob.CompletedAt,
				HTMLURL: payload.WorkflowJob.HTMLURL, GitHubJobID: payload.WorkflowJob.ID,
			})
			upsertJob(d, job)
			return nil
		})
	default:
		return nil
	}
}

// StateDir is the directory this service persists.
func (s *Service) StateDir() string { return s.dir }
