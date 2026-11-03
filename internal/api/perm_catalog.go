package api

// Perm is one entry of the permission catalog: the only fixed list in staff authorization,
// because routes are code. Roles (data) pick from it; every operator route declares one.
type Perm struct {
	Key      string `json:"key"`
	Section  string `json:"section"`
	Label    string `json:"label"`
	Risk     bool   `json:"high_risk"`
	Reserved bool   `json:"reserved"` // no route yet; may be assigned, grants nothing today
	// Grantable permissions may also be granted to one principal directly (Team > Direct grants),
	// cloud-IAM style. Only the Owner preset holds them by default; every other role is denied.
	Grantable bool `json:"grantable"`
}

// permPanel is the pseudo-permission "holds any staff permission" (panel shell, /me switch).
const permPanel = "panel"

// permOwner marks an Owner: the last active holder can't lose it (last-Owner guard).
const permOwner = "team.write"

var permCatalog = []Perm{
	{Key: "tenants.read", Section: "Tenants", Label: "View tenants and tenant VMs"},
	{Key: "tenants.lifecycle", Section: "Tenants", Label: "Create, provision, suspend, resume; enable and run tenant VMs", Risk: true},
	{Key: "tenants.delete", Section: "Tenants", Label: "Delete tenants", Risk: true},
	{Key: "projects.create", Section: "Tenants", Label: "Create a project for a tenant (operator path, owner set in Nomen + OpenFGA)", Risk: true},
	{Key: "workspaces.read", Section: "Workspaces", Label: "View workspace metadata"},
	{Key: "workspaces.act", Section: "Workspaces", Label: "Run workspace actions (start, stop, restart, delete)", Risk: true},
	{Key: "identity.read", Section: "Users & sign-in", Label: "View users, apps and identity providers"},
	{Key: "host.read", Section: "Host", Label: "View host health"},
	{Key: "host.admin_tools", Section: "Host", Label: "Open platform admin tools behind the edge (mail admin)", Risk: true},
	{Key: "edge.read", Section: "Edge", Label: "View edge routes and certificates"},
	{Key: "access.request", Section: "Access & audit", Label: "Request customer-approved access; see and end own grants"},
	{Key: "access.breakglass", Section: "Access & audit", Label: "Request break-glass access", Risk: true},
	{Key: "access.approve_breakglass", Section: "Access & audit", Label: "Approve or deny others' break-glass requests", Risk: true},
	{Key: "access.legal_hold", Section: "Access & audit", Label: "Delay the customer notice under a legal-hold category", Risk: true},
	{Key: "audit.read", Section: "Access & audit", Label: "Read the full audit log"},
	{Key: "templates.read", Section: "Templates", Label: "View workspace templates"},
	{Key: "templates.write", Section: "Templates", Label: "Create, edit and delete templates"},
	{Key: "mail.read", Section: "Mail", Label: "View mail settings", Reserved: true},
	{Key: "mail.write", Section: "Mail", Label: "Change mail settings", Reserved: true},
	{Key: "dns.read", Section: "DNS", Label: "View DNS zones", Reserved: true},
	{Key: "dns.write", Section: "DNS", Label: "Change DNS records", Reserved: true},
	{Key: "billing.read", Section: "Billing", Label: "View invoices", Reserved: true},
	{Key: "billing.write", Section: "Billing", Label: "Issue credits and change invoices", Reserved: true},
	{Key: "catalog.plans.read", Section: "Catalog", Label: "View plans, the service catalog, the plan x service matrix and tier defaults (no prices)", Grantable: true},
	{Key: "catalog.prices.read", Section: "Catalog", Label: "See plan prices", Grantable: true},
	{Key: "catalog.plans.write", Section: "Catalog", Label: "Create, edit and delete plans, services and the plan x service matrix", Risk: true, Grantable: true},
	{Key: "config.defaults.write", Section: "Catalog", Label: "Change tier defaults and their lock modes", Risk: true, Grantable: true},
	{Key: "customers.read", Section: "Customers", Label: "View customers, their service overrides and provision intents (no contacts or notes)", Grantable: true},
	{Key: "customers.contacts.read", Section: "Customers", Label: "See customer contacts and endpoints", Grantable: true},
	{Key: "customers.notes.read", Section: "Customers", Label: "See internal customer notes", Grantable: true},
	{Key: "customers.write", Section: "Customers", Label: "Create, edit and delete draft customers", Risk: true, Grantable: true},
	{Key: "customers.services.write", Section: "Customers", Label: "Change a customer's service overrides", Risk: true, Grantable: true},
	{Key: "config.effective.read", Section: "Customers", Label: "View a customer's effective configuration", Grantable: true},
	{Key: "provision.intent.create", Section: "Customers", Label: "Record a provision intent (desired state for a later Gold ticket)", Risk: true, Grantable: true},
	{Key: "platform.settings.read", Section: "Platform", Label: "View ops-editable platform settings (IdP account page, rrflow auto-attach)", Grantable: true},
	{Key: "platform.settings.write", Section: "Platform", Label: "Change ops-editable platform settings (audited)", Risk: true, Grantable: true},
	{Key: "infra.read", Section: "Infrastructure", Label: "View hosts, guests, node detail and the change history"},
	{Key: "infra.hosts.write", Section: "Infrastructure", Label: "Add, test and detach hosts", Risk: true},
	{Key: "infra.fleets.write", Section: "Infrastructure", Label: "Create and change fleets"},
	{Key: "infra.guests.power", Section: "Infrastructure", Label: "Start, stop, shut down and reboot guests", Risk: true},
	{Key: "infra.guests.snapshot", Section: "Infrastructure", Label: "Take snapshots of guests"},
	{Key: "infra.guests.restore", Section: "Infrastructure", Label: "Restore a guest from a snapshot or backup", Risk: true},
	{Key: "infra.guests.backup", Section: "Infrastructure", Label: "Back up guests"},
	{Key: "infra.guests.manage", Section: "Infrastructure", Label: "Create, clone, reconfigure, resize, migrate and delete guests", Risk: true},
	{Key: "infra.storage.write", Section: "Infrastructure", Label: "Download and upload to hypervisor storage", Risk: true},
	{Key: "infra.self", Section: "Infrastructure", Label: "Change the guest this panel runs on", Risk: true},
	{Key: "team.read", Section: "Team", Label: "View staff, roles and team settings"},
	{Key: "team.write", Section: "Team", Label: "Invite staff, assign roles, deactivate (Owner)", Risk: true},
	{Key: "team.roles", Section: "Team", Label: "Create, edit and delete roles", Risk: true},
	{Key: "team.settings", Section: "Team", Label: "Change team settings (approvals, timeouts, mode)", Risk: true},
}

var permIndex = func() map[string]Perm {
	m := map[string]Perm{}
	for _, p := range permCatalog {
		m[p.Key] = p
	}
	return m
}()

// knownPerm reports whether k is in the catalog.
func knownPerm(k string) bool { _, ok := permIndex[k]; return ok }

// grantablePerm reports whether k may be granted to a principal directly.
func grantablePerm(k string) bool { return permIndex[k].Grantable }

// GranularCatalogPerms are the customer-catalog permissions (Owner preset + direct grants only).
func GranularCatalogPerms() []string {
	out := []string{}
	for _, p := range permCatalog {
		if p.Grantable {
			out = append(out, p.Key)
		}
	}
	return out
}

// highRisk reports whether any of perms is a high-risk permission.
func highRisk(perms []string) bool {
	for _, k := range perms {
		if permIndex[k].Risk {
			return true
		}
	}
	return false
}
