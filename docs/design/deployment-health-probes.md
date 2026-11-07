# Target scoped deployment health probes

Status: implemented in the control plane and host agent; live provider verification
is still required. This is the health stage of
[`platform-absorption-plan.md`](platform-absorption-plan.md), not evidence that a
catalog service is turnkey or production ready.

## Ownership

- The deployment generation owns the declared TCP or HTTP check in its immutable
  runtime snapshot. The target system and provider selection are part of that
  same generation.
- The server enrollment protocol owns delivery to the selected target. The host
  credential authenticates one server; it cannot poll another server's command
  or submit that command's result.
- `internal/hostd` owns Incus-local identity and network read-back. It derives
  the instance name from the deployment UUID and verifies the deployment ID,
  generation, image reference hash, running state, NIC and managed network in
  the configured Incus project.
- `internal/healthprobe` owns the bounded TCP/HTTP behavior. It receives only an
  IP derived from verified Incus state and never accepts a host, URL, proxy or
  executable from the deployment request.
- `internal/deployments` owns the plan, step intent, result reconciliation,
  generation fence, health history and observed deployment state. AgentHub
  remains the only policy authority for provider writes; this probe is read only.

## Command lifecycle

1. The deployment worker validates the current generation, target, runtime
   declaration, execution plan and AgentHub-approved review. It writes the
   existing lease-fenced health-step intent.
2. It inserts a row into `shippin_server_commands`. The row is bound to the
   selected `server_id`, tenant, deployment, generation, plan digest and step
   idempotency key. Its JSON payload contains only the declared protocol, port,
   path, timeout, Incus network and image reference. It contains no secret.
3. The existing credential-authenticated heartbeat returns the oldest pending
   command for that active server and current deployment generation. An expired
   pending command is renewed for delivery on that authenticated heartbeat.
   Delivery is repeatable; the host keeps no command queue of its own.
4. `shippin-hostd` accepts the one supported typed command. It uses fixed
   `incus query` invocations, rejects any mismatch in owner, generation, image,
   running state or managed NIC, then requires a private unicast address inside
   the network's configured CIDR before probing it.
5. TCP succeeds only when connect succeeds. HTTP succeeds only on 2xx; redirects
   are returned as failures and are not followed. The check has a configured
   timeout capped at 60 seconds. Response bodies are not read.
6. The result endpoint authenticates the same host credential, stores one
   bounded immutable result, and enqueues the deployment generation for
   reconciliation. An identical result retry is idempotent; a different result
   conflicts. The durable result remains available if queue insertion has to be
   retried by the host.
7. A worker with a current generation and valid job lease claims recovery of the
   unfinished health-step intent, reads the persisted result and appends a
   health event. Only a verified `passing` result can make the deployment
   `ready`. A verified failure is `degraded`. Missing results stay `unknown`;
   stale generations cannot update current observed state.

## Failure behavior

Invalid, expired, wrong-server, malformed or conflicting messages fail closed.
Incus query errors, missing instances, owner/generation/image mismatch, stopped
instances, unmanaged/wrong networks, invalid CIDRs and addresses outside the
selected network cannot pass health. A missing heartbeat/result keeps health
unknown. Provider power state by itself never creates a passing event.
While a probe is queued, the deployment remains `unknown`; that active health
workflow is not recorded as `runtime_unsupported`. The authenticated result
enqueues the generation for reconciliation.

If the host accepts a result but the queue is unavailable, the endpoint returns
an error so the host retries the same command result. The database result is
idempotent. The deployment remains not-ready until a leased worker reconciles
the current generation. No probe can mutate provider state.

## Verification gates

Local tests cover 2xx/redirect/5xx, TCP success/failure, private address and
path bounds, managed subnet membership, exact Incus ownership/generation, and
the authenticated agent's command/result behavior. PostgreSQL CI must verify
command target binding, one-result semantics, replay, wrong credential, stale
generation, execution recovery and history persistence. A disposable Incus
host must then verify stopped/wrong-owner/wrong-generation/wrong-network cases,
real passing and failing services, host restart, console restart, expired
command recovery and environment-specific firewall/routing behavior.

Until the disposable provider and hosted PostgreSQL checks pass, the feature is
implemented but not proven live. No server-specific endpoint, credential,
tenant or address is embedded in code.
