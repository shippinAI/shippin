package db

// StaffPresets are the starting roles, inserted once on a fresh database (when no team
// settings exist yet). After that they are ordinary rows: editable, renamable, deletable.
var StaffPresets = []StaffRole{
	{Key: "staff.owner", Name: "Owner", Description: "Runs the team and every setting.", Permissions: []string{
		"tenants.read", "tenants.lifecycle", "tenants.delete", "projects.create", "workspaces.read", "workspaces.act", "identity.read",
		"host.read", "host.admin_tools", "edge.read", "access.request", "access.breakglass", "access.approve_breakglass",
		"access.legal_hold", "audit.read", "templates.read", "templates.write", "mail.read", "mail.write", "dns.read", "dns.write",
		"billing.read", "billing.write", "team.read", "team.write", "team.roles", "team.settings",
		"catalog.plans.read", "catalog.prices.read", "catalog.plans.write", "config.defaults.write", "customers.read",
		"customers.contacts.read", "customers.notes.read", "customers.write", "customers.services.write",
		"config.effective.read", "provision.intent.create", "platform.settings.read", "platform.settings.write",
		"infra.read", "infra.hosts.write", "infra.fleets.write", "infra.guests.power", "infra.guests.snapshot", "infra.guests.restore", "infra.guests.backup", "infra.guests.manage", "infra.storage.write", "infra.self"}},
	{Key: "staff.platform_admin", Name: "Platform Admin", Description: "Configures the platform: tenants, templates, mail, DNS, edge.", Permissions: []string{
		"tenants.read", "tenants.lifecycle", "tenants.delete", "workspaces.read", "workspaces.act", "identity.read",
		"host.read", "host.admin_tools", "edge.read", "access.request", "audit.read", "templates.read", "templates.write",
		"mail.read", "mail.write", "dns.read", "dns.write", "team.read"}},
	{Key: "staff.ops", Name: "SRE / Ops", Description: "Keeps hosts, VMs and workspaces running.", Permissions: []string{
		"tenants.read", "tenants.lifecycle", "workspaces.read", "workspaces.act", "identity.read", "host.read",
		"edge.read", "access.request", "access.breakglass", "templates.read", "infra.read", "infra.guests.power", "infra.guests.snapshot"}},
	{Key: "staff.infra", Name: "Infrastructure", Description: "Adds hosts and changes guests. Does not touch the guest the panel runs on.", Permissions: []string{
		"infra.read", "infra.hosts.write", "infra.fleets.write", "infra.guests.power", "infra.guests.snapshot", "infra.guests.restore", "infra.guests.backup", "infra.guests.manage", "infra.storage.write"}},
	{Key: "staff.support", Name: "Support", Description: "Helps customers; tenant data only with their consent.", Permissions: []string{
		"tenants.read", "workspaces.read", "identity.read", "access.request", "templates.read"}},
	{Key: "staff.billing", Name: "Billing", Description: "Plans, invoices and credits.", Permissions: []string{
		"tenants.read", "billing.read", "billing.write"}},
	{Key: "staff.security", Name: "Security / Compliance", Description: "Reads the audit log, approves break-glass, reviews the team.", Permissions: []string{
		"tenants.read", "workspaces.read", "identity.read", "host.read", "edge.read", "access.approve_breakglass",
		"access.legal_hold", "audit.read", "templates.read", "mail.read", "dns.read", "billing.read", "team.read", "infra.read"}},
	{Key: "staff.auditor", Name: "Read-only Auditor", Description: "Read-only access to configuration and the audit log.", Permissions: []string{
		"tenants.read", "workspaces.read", "host.read", "edge.read", "audit.read", "templates.read", "mail.read",
		"dns.read", "billing.read", "team.read", "infra.read"}},
}

// DefaultStaffSettings are the starting team settings (all editable in the Team page).
var DefaultStaffSettings = StaffSettings{
	Mode: "shadow", LegacyFallback: true, TwoPerson: false, IdleDays: 90,
	BreakGlassApproval: true, ApprovalMinutes: 30, SoleOwnerActivation: true,
}
