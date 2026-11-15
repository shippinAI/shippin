# Host shell (design, not built)

A shell on an Incus or Proxmox host, opened from the control plane by the owner and named
staff. It follows standard privileged-access practice: the privilege is granted per person and
per host, for a stated time, with documented minimum requirements, and every use is recorded. It serves INF-01 and INF-03 under IAM-01 and IAM-02, in roadmap stage 4 ("Control plane
access"). Nothing here is implemented: `internal/hv` returns `ErrUnsupported` for console and
exec, and hostd's routes and `run.go` commands are closed sets.

## Why it was rejected, and what changes

Wardenclyffe's `terminal.go` gave a host root PTY on a websocket page. The
[capture matrix](absorption/warden-capture-matrix.md) rejected it for security: root on every
host, no session broker, and a copied page instead of an authorized, audited action. This
design changes four things:

- **A broker.** The browser only talks to the console. The console checks every condition,
  writes the audit entry and only then opens the host side. The browser never sees a host
  address, a Proxmox ticket or hostd.
- **No root by default.** On Incus hosts the shell runs as a configured unprivileged account.
  Its sudo rights are the owner's host policy, not something Shippin grants.
- **A granted, bounded session.** A privileged permission granted in a dialog with its scope and
  duration, a reason, a fresh second factor, limits, open and close audit entries, and optionally
  a recording.
- **Off unless switched on** in hostd's config.

## Who may open it

- A privileged permission, `host.shell` ("Open a shell on a server"). It is flagged as
  privileged in the permission catalog, so it is never part of a preset other than Owner and is
  never implied by another permission. The owner grants it to named staff.
- Never tenants. It is an operator route. Tenant membership, tenant roles and tenant access
  grants do not count: they cover customer workspaces, not hosts.
- Only people: no tokens, device sessions, MCP or AI agents. Automation gets narrow actions.

## Granting it

The owner grants it from **Staff > (person) > Privileged access > Grant host shell**, a dialog
whose fields are all choices, with the minimum requirements stated beside them:

| Field | Choices | Minimum |
| --- | --- | --- |
| Hosts | All hosts, a site, or chosen hosts | At least one host |
| Run as | The accounts configured for those hosts (`host_shell.users`), root only where a host allows it | An account that exists on each chosen host |
| Duration | 1 hour, 8 hours, 7 days, 30 days, or until removed | Owner only for "until removed" |
| Session limits | Idle 10 or 30 minutes; maximum 1 or 4 hours | Within the installation's bounds |
| Approval | None, or a second staff member approves each open | Required for root |
| Recording | Output, output and input, or none | The installation's policy may require output |
| Reason | Text | `access.reason_min` characters |

The grant is itself audited, shows on the person's page with its expiry, can be removed at once,
and ends by itself when its duration runs out. The same dialog shows what the person can already
do, so a grant never widens access unseen.

## Impersonation and escalation

- Impersonating a staff member or a customer never carries `host.shell`: an impersonated session
  is refused, whoever is impersonated.
- A staff member without the grant asks for it from the host's page. The request names the host,
  the account and the reason and goes to the owner, or to staff the owner allows to approve.
  Approving fills the same dialog; nothing is granted by the request alone.
- An approver cannot approve their own request.

## Opening a session

1. `POST /v1/operator/hosts/{hostId}/shell-sessions` is an MCPL action
   (`internal/infra/host_shell/open.go`). As a write it takes a reason, within the
   `access.reason_min` and `reason_max` bounds. It returns a session ID and a single-use
   ticket that expires in 30 seconds.
2. The checks run in this order:
   - origin;
   - `host.shell`, granted for this host and account and not expired;
   - not an impersonated session;
   - the staff account is active;
   - the host is attached and enabled;
   - a second factor (a passkey or an identity provider) on this session;
   - a sign-in within `host_shell.reauth_minutes` (default 10), reusing the break-glass
     re-authentication check;
   - one session per person per host.
3. The console writes `host.shell.open` (actor, host, reason, transport, client IP) before it
   contacts the host. If the write fails, the session is refused.
4. The websocket attach route redeems the ticket once. It is mounted with `a.transport`, so it
   adds no legacy route. It speaks the workspace terminal's protocol: binary keystrokes and
   output, a JSON resize message, a ready frame, and close codes.

## Limits and close

- The session closes after 10 minutes without keystrokes and after 1 hour in any case. It does
  not reconnect by itself: a new session needs a reason and passes the checks again.
- Every minute the console re-checks the permission, the account, the host and the sign-in
  age. A failure closes the session as "revoked".
- `host.shell.close` records why it ended (idle, maximum, revoked, closed or ended), how long it
  ran and the bytes each way. It is written even when the host side failed.

## Recording

This is an open decision. The proposal:

- Record output by default as asciicast v2. The console streams it, so a host cannot opt out.
- Seal the recording with the installation key, keep it with the audit entry for 90 days
  (configurable), and let only people with `audit.read` and `host.shell` read it.
- Keep input recording off unless the owner turns it on, because input captures passwords
  typed at prompts.

## Transport per kind

**Proxmox: the node shell, through the driver's API client.**

1. `POST /nodes/{node}/termproxy` returns a port and a ticket.
2. The console opens `wss://<host>/api2/json/nodes/{node}/vncwebsocket?port=&vncticket=` with
   TLS verification on.
3. It sends `user:ticket\n`.
4. It translates Proxmox's frames to the browser protocol: `0:len:data` for input,
   `1:cols:rows:` for resize and `2` for ping.

Resize works and nothing is installed on the host. The token needs `Sys.Console` on
`/nodes/{node}`. A non-root user gets a login prompt, not a root shell. That this also holds
for API tokens is unverified: check it on the local Proxmox rig.

**Incus: hostd.** Incus has no host shell API.

- hostd gets a new route, `GET /host/shell` with `Upgrade: shippin-host-pty.1`. It is in the
  closed route set only when `host_shell.enabled` is true.
- It runs a fixed login shell for `host_shell.user`. The user must match
  `^[a-z_][a-z0-9_-]{0,31}$`, must exist, and may be root only with `host_shell.allow_root`.
  hostd starts the shell with `SysProcAttr.Credential` in the host PTY the workspace terminal
  added (`terminal_pty_linux.go`), with the same framing and hangup.
- Nothing from the caller reaches argv, environment or path. Only rows and cols are read, and
  they are bounded.
- The console reaches it over the hostd unix socket, which the Incus proxy device exposes to
  the console alone (`incus.hostd_url`). There is no new listener.

## What it must refuse

- A tenant member or owner. Staff who hold only a tenant grant. An impersonated session.
- A grant that does not cover this host or account, or has expired.
- Staff without `host.shell`, a deactivated account, or an expired role.
- A missing or short reason. No second factor, or a stale sign-in.
- Tokens, device sessions, MCP and agents. A cross-origin page. A reused or expired ticket.
- A host that is detached or disabled, or a Proxmox host whose TLS does not verify.
- hostd with the feature off, an invalid user, or root without `allow_root`.
- A second session by the same person on the same host.
- Any open whose audit entry cannot be written.

## Tests that would prove it

- **Permissions.** `host.shell` alone opens, and so does the owner. `host.read`, `infra.*`,
  `host.admin_tools`, break-glass holders and tenant owners are refused. A refusal writes no
  `host.shell.open` and makes no hostd or Proxmox call.
- **Sign-in.** Password-only, stale and bearer-token sessions are each refused.
- **Ticket.** Single use, 30 seconds, bound to the person and host.
- **Limits.** Idle, the maximum, and a mid-session revocation (role removed, account
  deactivated, host detached) each close with their code and a matching `host.shell.close`.
- **hostd.** The route is absent when the feature is off. argv is fixed and the user rules
  hold. The workspace terminal's PTY resize and hangup tests apply. A run against the local
  Incus rig uses an unprivileged user.
- **Proxmox.** On the local Proxmox rig: termproxy and vncwebsocket give a login prompt,
  resize and close, and a host whose TLS does not verify is refused.
- **Recording.** Output captured, input not by default; other readers refused.

## Open decisions for the owner

1. Approval: which grants need a second staff member's approval for each open, beyond root?
2. Recording: the installation's default, and how long recordings are kept.
3. The default account on Incus hosts, and whether root is ever allowed.
4. The bounds for the session limits and grant durations above.
5. Whether customers get this later on servers they bring (BYO-01), with its own permission
   scoped to those servers.
