# Attaching a hypervisor

One server row is one hypervisor API. The console does not keep a second registry or a root password. A secret entered in the panel is sealed in the database with a key derived from `SHIPPIN_SESSION_KEY`, and no response ever returns it.

## From the panel

Open **Infrastructure → Hosts & nodes** and choose **Add host**. The overview lists every provider this installation offers. A provider gets its own tab as soon as a host is attached to it. The form's fields come from the driver, and their names and defaults come from `ui.providers` in the config file or **Provider panel → Interface**.

**Test connection** tries what the form holds and saves nothing. **Add host** creates the server row and attaches its API in one step, which is the `POST /v1/infra/servers` call below. The overview then reports whether the host answers, and keeps listing a host that does not, with the reason. **Detach** removes the API and leaves the server row and its guests alone.

These steps have been run as written against Incus 6.0.4 and Proxmox VE 9.2, on one node each and not yet in a cluster. Try them on a disposable guest first.

### Proxmox VE

On a node, make a user and a token for the panel. `--privsep 0` gives the token its user's permissions, and the command prints the secret once:

```sh
pveum user add shippin@pve
pveum user token add shippin@pve panel --privsep 0
pveum acl modify / --users shippin@pve --roles PVEVMAdmin,PVEAuditor
```

`PVEVMAdmin` covers power and snapshots, and `PVEAuditor` lets the panel read node and cluster status. Then fill in the API URL (`https://<node>:8006`), the token ID (`shippin@pve!panel`) and the secret. In a cluster, name the node to reach it through; the test lists them.

### Incus

The server must listen on an address the panel can reach, and then issue a token:

```sh
incus config set core.https_address <address>:8443
incus config trust add shippin-panel
```

Paste what it prints into **Trust token**. The token says where the server is and which certificate it presents, so there is nothing else to fill in, and the panel refuses a server whose certificate does not match. **Test connection** checks that without using the token up; **Add host** spends it, makes a client certificate for this host, and proves the server now trusts it. A token works once.

**Project** selects the Incus project; the driver's reads and changes stay inside it.

Without a token, open **Advanced**: give the API URL and certificate and key paths on the console host, and run `incus config trust add-certificate` on the server.

### A server's own certificate

Certificate checks are always on. A server with a certificate nobody vouches for, which is the default on both hypervisors, stops the test and shows its SHA-256 fingerprint. Compare it with the one the server reports (`pvenode cert info`, or `incus info`) and choose **Trust this certificate**. The panel then accepts that exact certificate for that host and no other, so a server that changes its certificate stops answering until someone looks. To use your own CA instead, give **Server CA file** under **Advanced**.

### Keeping secrets out of the database

Under **Advanced**, **Token environment variable** names a variable on the console instead of saving a secret: put it in `/etc/shippin/console.env` as `SHIPPIN_HV_<NAME>=secret` and restart the console. An installer re-run keeps `SHIPPIN_HV_*` values.

Replacing `SHIPPIN_SESSION_KEY` makes saved secrets unreadable. The host then reports the error and its secret must be entered again.

### Changing guests

Every change is recorded before it starts, and **Activity** shows who asked and how it ended. A change keeps going if you leave the page. An outcome the panel cannot determine, because the connection dropped or the panel restarted, reads **Unknown** and is never retried: check the guest. **Stop guests** stops the ones you tick, one at a time and in order, each with its own result. It always skips the guest the panel runs on, and stopping that guest on its own asks first (`SHIPPIN_SELF_GUEST`).

### Fleets

The **Fleets** tab groups hosts of any provider so you can see them together. It changes nothing on the hosts. Different providers cannot share a cluster or live-migrate, so each host keeps its own provider and its own native cluster, and the panel says so.

Start, stop, shutdown, reboot, snapshot, listing snapshots and restoring one are shared by every driver. Calls only Proxmox has answer `409` on an Incus host, and the panel does not offer them.

The local installation owner has no IdP to grant `IAM_INFRA`, so the infrastructure routes accept that principal as the installation owner. This does not make it a founder or approve a provider mutation: gated lifecycle actions still require an explicit review policy, and AgentHub integration remains pending.

## What you set through the API

1. Create the server in the org (or use one that already enrolled). Turn on `servers.enabled` so the org can open it.
2. On **Hosts**, pick the server and fill the form. The same call is `PUT /v1/infra/servers/{id}`. `GET /v1/infra/servers/registry` lists servers that are not revoked, including ones with no hypervisor yet.

```json
{
  "kind": "proxmox",
  "base_url": "https://pve.example:8006",
  "node": "node1",
  "token_id": "agent@pve!console",
  "token_env": "SHIPPIN_HV_TOKEN",
  "ca_file": ""
}
```

`base_url` is `https` with a host and no user, query, or path. `token_id` is the Proxmox token id, not the secret. `token_env` is the name of an environment variable on the console process. `ca_file` is optional; empty uses the system trust store plus nothing else. TLS verification stays on.

3. Put the token secret in that environment variable. It is not a column and not a response field.
4. Grant `IAM_INFRA` in Nomen. `roles.iam` maps it to `op_infra`. The console does not create the Nomen role.
5. The org opens **Servers**, then the row. Guests, start, stop, snapshot, and task lookup are on that page. The operator page **Hosts** is the same model for every attached server, plus create.

## Authorization and review

The sample installation has no local approval policy or board. Provider mutations
that require a policy fail closed. The old local task board and HTTP board are not
AgentHub and must not be used to authorize provider changes. AgentHub policy,
system mapping and reviewed ticket execution are still being implemented; this
document does not claim that review workflow is available.

Every provider change must be audited before it runs. See [the control plane](control-plane.md).

## Who sees which guests

| Caller | Sees | Can change |
|---|---|---|
| Infra role | Every server with a hypervisor kind | Power, snapshots, create, delete, migrate, storage, attach |
| Org owner or admin | Servers that org owns or is assigned | Power and snapshots, only after a configured policy permits the action |
| Org member | Those same servers | Read guests and tasks |

Another org's server returns 404. An estate seed row with no assignment is operator-only.
