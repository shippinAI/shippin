# Service contexts and ownership

This is the product and integration map for Shippin's planned service experience.
It is design direction and a draft contract, not a claim that the services,
multi-tenant isolation or managed database UI have been implemented.

## Product hierarchy

- **Installation:** one deployed Shippin control plane and its infrastructure.
  An operator manages hosts, platform services and installation configuration.
- **Tenant / organization:** the customer's authorization and data boundary.
  The existing API calls this a *tenant*; an organization is a user-facing business
  concept. Mapping an external identity provider's organizations to Shippin tenants
  requires an explicit, verified integration; equal names or IDs do not prove it.
- **Project:** a tenant-owned grouping for application work, environments and
  project services. The existing API nests projects under tenants.
- **Workspace:** a tenant-scoped compute environment. A project can refer to
  workspaces through existing project routes; do not assume every workspace must
  belong to a project or that an Incus project is a Shippin project.
- **Service definition and attachment:** a definition describes a capability
  offered by a provider; an attachment records its use at a specific installation,
  tenant, project or workspace scope. Separate the type from each actual instance.
  Host placement and ownership are separate from the tenant-facing scope.

Tenant identifiers in a request are not proof of access. Each operation must use
verified identity and enforce tenant, object and action authorization on the server.
Check scope and relationships in the database and provider adapter. Cover cross-tenant
reads, writes, cache keys, object storage, compute, jobs and logs with real isolation
tests. Schema validation alone cannot enforce ownership or authorization.
[OWASP multi-tenant security](https://cheatsheetseries.owasp.org/cheatsheets/Multi_Tenant_Security_Cheat_Sheet.html)
provides relevant threat categories.

## Capability groups

The Shippin.Cloud operator can offer these capabilities where implemented and
authorized. Product names and domains below describe the owner's requested
ecosystem; they are not default URLs, credentials, deployed services or mandatory
dependencies of a self-hosted installation. Availability and configuration must
come from the installation, and individual attachments need scoped authorization.

| Group | Desired capability and candidate products | Boundary |
| --- | --- | --- |
| Infrastructure | Incus first; Proxmox later | Hosts, placement, workloads and provider-specific capabilities belong to the infrastructure control plane. |
| Data | ShippinDB / PostgreSQL; Dragonfly-compatible cache | Database and cache have separate lifecycle, performance, isolation and recovery requirements. |
| Object storage | ShippinS3 | A separately scoped object-storage capability; S3 compatibility requires testing, not a name alone. |
| Identity and secrets | Nomen / `nomen.sh`; Vaultix / `vaultix.cloud` | Identity and secret systems own their records and expose reviewed interfaces; the panel does not become a second identity or secrets store. |
| Communication | Email; Chatmaton | Email delivery and conversation are distinct capabilities. Chatmaton owns its conversations and intent, not infrastructure inventory or other products' credentials. |
| Agents and specialists | AgentHub, Clyffy and shippin.bot | AgentHub owns its task and event stream; Clyffy is an assistant/harness context; shippin.bot is the government-contract specialist. Their attachment, permissions and readiness require separate implementation. |
| Surfaces and domains | `shippin.cloud`, `shippin.tech` | Branding and domain routing are operator-configured; their exact product roles are not yet established in this repository. |
| Unresolved | OMA | Its meaning and ownership need clarification before assigning a capability. |

A cloud-user experience comparable in usability to a managed database dashboard
should eventually expose authorized database and storage operations, connection
details, metrics, backups and recovery through the owning APIs. Do not call a
service optimized or compatible until representative workloads, resource limits,
failure behavior and restore have been measured. PostgreSQL row-level security
is only one layer: table owners and privileged roles can bypass it, so roles and
actual queries must be tested. [PostgreSQL row security](https://www.postgresql.org/docs/current/ddl-rowsecurity.html).
Dragonfly is a cache candidate, not evidence that session, queue or durability
semantics are interchangeable with another provider.

## Canonical contract and code placement

The draft [service schema](../api/openapi/schemas/service-catalog.json), registered
in [OpenAPI](../api/openapi/openapi.yaml), defines `ServiceDefinition`,
`ServiceAttachment` and their explicit scopes. Definitions are catalog metadata;
availability is not authorization or live health. Attachments separate desired
state from observed state and require a timestamp for a non-unknown observation.
The timestamp uses the existing shared API timestamp definition. Only the eventual
server/provider can establish observation freshness, tenant
ownership, supported-scope membership, safe configuration and lifecycle state.
The schema excludes arbitrary fields; it cannot prevent secrets being placed in a
free-text field, and is not a credential store.

The existing project-services endpoint is a historical draft with an extensible
response. The new schema does not change that response or create a catalog endpoint.
Before using the new shapes on an API, define versioning, migration and compatibility,
verify callers and update the authoritative OpenAPI path and tests together.

Place an API shape under `api/openapi/`, panel features under their owning
`panel/src/features/<context>/`, and operational records under the owning service.
Establish control-plane and host-agent source directories only when real reviewed
implementation is adopted; do not create empty service folders or duplicate a
provider's database or event store. Use lowercase kebab-case for directories and
files, snake_case for API fields, and stable lowercase dotted or hyphenated keys
for service definitions. Display names and endpoints are configuration, not IDs.
Bounded contexts define ownership; they do not by themselves mandate separate
processes. [Domain analysis](https://learn.microsoft.com/azure/architecture/microservices/model/domain-analysis).

The local candidate projects were reviewed at README level only. Their claims and
licenses require source, dependency, rights, security and integration review before
code or deployment artifacts are adopted. In particular, the AgentHub candidate
states its hosted mode is not ready for unrelated tenants in one instance. The
relationship between `nomen.sh` and any particular local identity checkout remains
to be verified; a folder name alone is not evidence of ownership.
