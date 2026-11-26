package mcpl

// Boundary is one domain as an AI client meets it: what the domain is for, and what to know before
// acting in it. The entrance lists boundaries by Purpose; a boundary's own MCP opens with Knowledge.
type Boundary struct {
	Purpose   string
	Knowledge string
}

// Boundaries has one entry for every domain in Domains.
var Boundaries = map[string]Boundary{
	"public": {
		Purpose:   "What anyone can read without signing in.",
		Knowledge: "Nothing here needs a session or changes anything for a person.",
	},
	"auth": {
		Purpose:   "Signing in and getting a session.",
		Knowledge: "A person signs in themselves. Never ask for or handle their password.",
	},
	"access": {
		Purpose:   "Who may do what: roles, permissions and the audit log.",
		Knowledge: "Changing access changes what another person can do. Say who is affected before a change, and confirm it with the person you act for.",
	},
	"identity": {
		Purpose:   "The person's own account: access keys and SSH keys.",
		Knowledge: "These belong to the person you act for. An access key or private key is shown once; hand it to the person and do not keep it.",
	},
	"tenancy": {
		Purpose:   "Organizations, customers, plans and coverage.",
		Knowledge: "An organization owns everything under it. A plan sets what an organization may use; a coverage tier sets what support it gets. Most changes here are staff actions and need a reason.",
	},
	"infrastructure": {
		Purpose:   "Servers, the guests on them, networks and metrics.",
		Knowledge: "A server is an Incus host, a Proxmox host or a plain machine, added by its own address and credential. A guest belongs to the server it runs on. Stopping or deleting a guest interrupts whatever runs in it.",
	},
	"services": {
		Purpose:   "What can be deployed: the service catalog and runner sizes.",
		Knowledge: "The catalog decides what people may deploy and with which options. Changing an entry affects new deployments, not running ones.",
	},
	"runtime": {
		Purpose:   "Running things: development spaces, templates, runners and their jobs.",
		Knowledge: "A development space is a person's working machine. Stopping one keeps its disk; deleting one does not. Check its state before starting or stopping it.",
	},
	"platform": {
		Purpose:   "This installation's own settings and secret stores.",
		Knowledge: "Settings apply to the whole installation and take effect at once. A secret's value is never returned; only its name and where it is kept.",
	},
	"integrations": {
		Purpose:   "Connections to outside systems, such as GitHub.",
		Knowledge: "An integration acts on an outside account with the permission that account granted. Listing is safe; connecting or removing changes what the installation may reach.",
	},
}
