package hostd

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// docs/plans.md is the source of truth. hostd.json, the console default plan, and the catalog
// seed (011 plus the 028 upsert) have to match it.

type planTier struct {
	ID          string          `json:"id"`
	Label       string          `json:"label"`
	Description string          `json:"description"`
	Rank        int             `json:"rank"`
	PriceCents  int             `json:"price_cents"`
	Public      bool            `json:"public"`
	Hostd       Plan            `json:"hostd"`
	Mail        *mailLimits     `json:"mail"`
	Vaultix     *vaultixLimits  `json:"vaultix"`
	Identity    *identityLimits `json:"identity"`
	AgentHub    *hubLimits      `json:"agent_hub"`
	Chatmaton   *chatLimits     `json:"chatmaton"`
	DNS         *dnsLimits      `json:"dns"`
}

type mailLimits struct {
	Placement string `json:"placement"`
	Mailboxes int    `json:"mailboxes,omitempty"`
	Domains   int    `json:"domains,omitempty"`
	QuotaGiB  int    `json:"quota_gib,omitempty"`
	PerDay    int    `json:"per_day,omitempty"`
}

type vaultixLimits struct {
	Placement string `json:"placement"`
	Projects  int    `json:"projects,omitempty"`
}

type identityLimits struct {
	Placement string `json:"placement"`
}

type hubLimits struct {
	Seats              int `json:"seats"`
	ConcurrentSessions int `json:"concurrent_sessions"`
}

type chatLimits struct {
	Seats         int `json:"seats"`
	RetentionDays int `json:"retention_days"`
}

type dnsLimits struct {
	Zones int `json:"zones"`
}

type devLimits struct {
	Mode        string `json:"mode"`
	Cowork      bool   `json:"cowork"`
	Workspaces  int    `json:"workspaces"`
	CPU         int    `json:"cpu"`
	MemoryGiB   int    `json:"memory_gib"`
	HomeGiB     int    `json:"home_gib"`
	QuotaGiB    int    `json:"quota_gib"`
	RootGiB     int    `json:"root_gib,omitempty"`
	DataGiB     int    `json:"data_gib,omitempty"`
	VMCPU       int    `json:"vm_cpu,omitempty"`
	VMMemoryGiB int    `json:"vm_memory_gib,omitempty"`
}

func TestPlansMatchCanonicalDoc(t *testing.T) {
	doc := loadPlanDoc(t)
	raw, err := os.ReadFile("../../deploy/hostd.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Plans map[string]Plan `json:"plans"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Plans) != len(doc) {
		t.Fatalf("hostd.json has %d plans, docs/plans.md has %d", len(file.Plans), len(doc))
	}
	wantNames := []string{"free", "starter", "pro", "business", "founder"}
	if len(doc) != len(wantNames) {
		t.Fatalf("tier count %d", len(doc))
	}
	seed, err := os.ReadFile("../../internal/db/migrations/011_catalog_seed.sql")
	if err != nil {
		t.Fatal(err)
	}
	upsert, err := os.ReadFile("../../internal/db/migrations/034_canonical_plans.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i, tier := range doc {
		if tier.ID != wantNames[i] {
			t.Fatalf("tier %d is %s, want %s", i, tier.ID, wantNames[i])
		}
		got, ok := file.Plans[tier.ID]
		if !ok || !reflect.DeepEqual(got, tier.Hostd) {
			t.Fatalf("hostd.json plans.%s = %+v, docs/plans.md hostd = %+v", tier.ID, got, tier.Hostd)
		}
		if tier.ID == "founder" && tier.Public {
			t.Fatal("founder is not sold")
		}
		if tier.ID != "founder" && !tier.Public {
			t.Fatalf("%s should be public", tier.ID)
		}
		pub := "false"
		if tier.Public {
			pub = "true"
		}
		row011 := sprintfPlan(tier, pub, false)
		row028 := sprintfPlan(tier, pub, true)
		if !strings.Contains(string(seed), row011) {
			t.Fatalf("011 missing %s", row011)
		}
		if !strings.Contains(string(upsert), row028) {
			t.Fatalf("028 missing %s", row028)
		}
		requireLimit(t, seed, upsert, tier.ID, "devspace", "bounded", devspaceLimits(tier.Hostd))
		requireLimit(t, seed, upsert, tier.ID, "mail", mailLock(*tier.Mail), tier.Mail)
		requireLimit(t, seed, upsert, tier.ID, "vaultix", vaultixLock(*tier.Vaultix), tier.Vaultix)
		requireLimit(t, seed, upsert, tier.ID, "identity", "bounded", tier.Identity)
		requireOptional(t, seed, upsert, tier.ID, "agent_hub", "bounded", tier.AgentHub)
		requireOptional(t, seed, upsert, tier.ID, "chatmaton", "bounded", tier.Chatmaton)
		requireOptional(t, seed, upsert, tier.ID, "dns", "bounded", tier.DNS)
		if tier.Hostd.VM == nil {
			if tier.ID != "free" {
				t.Fatalf("%s has no vm block", tier.ID)
			}
		} else if tier.Hostd.Instances != devspaceLimits(tier.Hostd).Workspaces+1 {
			t.Fatalf("%s instances must be workspaces+1 (the tenant VM)", tier.ID)
		}
	}
	cfg, err := LoadConfig("../../deploy/hostd.example.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Check(); err != nil {
		t.Fatal(err)
	}
	var console struct {
		Workspaces struct {
			DefaultPlan string `json:"default_plan"`
		} `json:"workspaces"`
		Onboarding struct {
			Plan string `json:"plan"`
		} `json:"onboarding"`
	}
	b, err := os.ReadFile("../../config/shippin.example.json")
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(b, &console) != nil || console.Workspaces.DefaultPlan != "free" || console.Onboarding.Plan != "free" {
		t.Fatalf("console default plan: %+v", console)
	}
	retired := []string{"micro", "single", "dual", "b1", "b2", "b4", "b8", "custom", "team", "enterprise"}
	up := string(upsert)
	for _, id := range retired {
		if !strings.Contains(up, "'"+id+"'") {
			t.Fatalf("028 does not retire %s", id)
		}
		if _, ok := file.Plans[id]; ok {
			t.Fatalf("hostd.json still has retired plan %s", id)
		}
	}
}

func loadPlanDoc(t *testing.T) []planTier {
	t.Helper()
	b, err := os.ReadFile("../../docs/plans.md")
	if err != nil {
		t.Fatal(err)
	}
	const start, end = "```plans\n", "\n```"
	s := string(b)
	i := strings.Index(s, start)
	if i < 0 {
		t.Fatal("docs/plans.md has no ```plans block")
	}
	s = s[i+len(start):]
	j := strings.Index(s, end)
	if j < 0 {
		t.Fatal("docs/plans.md plans block is not closed")
	}
	var doc struct {
		Tiers []planTier `json:"tiers"`
	}
	if err := json.Unmarshal([]byte(s[:j]), &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Tiers
}

func devspaceLimits(p Plan) devLimits {
	d := devLimits{
		Mode: "container", Cowork: p.Cowork, Workspaces: p.Instances,
		CPU: p.WorkspaceCPU, HomeGiB: p.HomeGiB, QuotaGiB: p.QuotaGiB, RootGiB: p.RootGiB,
	}
	if n, ok := p.workspaceMemGiB(); ok {
		d.MemoryGiB = n
	}
	if p.VM != nil {
		d.Mode = "vm"
		d.Workspaces = p.Instances - 1
		d.RootGiB = p.VM.RootGiB
		d.DataGiB = p.VM.DataGiB
		d.VMCPU = p.VM.CPU
		if n, ok := memGiB(p.VM.Memory); ok {
			d.VMMemoryGiB = n
		}
	}
	return d
}

func mailLock(m mailLimits) string {
	if m.Mailboxes > 0 {
		return "bounded"
	}
	return "overridable"
}

func vaultixLock(v vaultixLimits) string {
	if v.Placement == "shared" {
		return "bounded"
	}
	return "overridable"
}

func requireLimit(t *testing.T, seed, upsert []byte, plan, svc, lock string, limits any) {
	t.Helper()
	raw, err := json.Marshal(limits)
	if err != nil {
		t.Fatal(err)
	}
	row := "('" + plan + "','" + svc + "',true,'" + lock + "','" + string(raw) + "')"
	if !strings.Contains(string(seed), row) {
		t.Fatalf("011 missing %s", row)
	}
	if !strings.Contains(string(upsert), row) {
		t.Fatalf("028 missing %s", row)
	}
}

func requireOptional(t *testing.T, seed, upsert []byte, plan, svc, lock string, limits any) {
	t.Helper()
	if limits == nil || reflect.ValueOf(limits).IsNil() {
		row := "('" + plan + "','" + svc + "'"
		if strings.Contains(string(seed), row) || strings.Contains(string(upsert), row) {
			t.Fatalf("%s should not include %s", plan, svc)
		}
		return
	}
	requireLimit(t, seed, upsert, plan, svc, lock, limits)
}

func sprintfPlan(tier planTier, pub string, withBoundaries bool) string {
	base := "('" + tier.ID + "','" + tier.Label + "','" + tier.Description + "'," +
		itoa(tier.Rank) + "," + itoa(tier.PriceCents) + "," + pub
	if !withBoundaries {
		return base + ")"
	}
	return base + "," + itoa(tier.Hostd.Boundaries) + ")"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
