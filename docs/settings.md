# Settings and secrets

How an installation's configuration is edited in the panel (CFG-01 in [What Shippin is](objective.md))
and where its keys are kept. Status and order are in the [roadmap](roadmap.md), stage 2; this page is
the design.

## What staff see

1. **Settings > Secret store.** Staff connect the installation to its Vaultix account with a machine
   identity: the Vaultix address, the project, the environment, a folder, and the identity's client
   ID and client secret. **Test** signs in, writes a check value, reads it back and deletes it, and says
   which step failed.
2. **Settings > Sections.** Each configuration section (build runners, the job queue, the GitHub App,
   and more as they move) is a form generated from the section's own description. Every field says
   where its value comes from now: the panel, the bootstrap file, or the default.
3. A key field (a client secret, a private key, a webhook secret) is never shown. The form says
   whether one is saved and when. Staff replace it or remove it. The value goes to Vaultix and nowhere
   else.
4. Saving asks for a reason, is refused with the field's own message when a value is invalid, takes
   effect without a restart, and is in the audit log and the section's history.

## Rules

| Rule | How |
| --- | --- |
| One owner per value | A section is declared once, in the package that uses it. The panel's value wins; the bootstrap file only seeds an installation that has not saved the section yet. |
| Keys go to Vaultix | A secret field is written to the connected Vaultix project and the database keeps only where it is: project, environment, folder, key name. With no secret store connected, a key can't be saved and the form says so. Keys still in the environment keep working until they are entered in the panel. |
| One local secret | The machine identity's client secret is the key to Vaultix itself, so it can't be stored there. It is sealed in PostgreSQL with the panel's at-rest key (derived from the session key, like the AgentHub credential), never returned, and replaced or cleared only. |
| Never shown | No action returns a key's value. Audit entries, history, events and logs name the field, never the value. |
| Validated first | A section's own check runs on the whole effective section before anything is written. |
| Safe against two editors | Each save names the version it was based on; a stale one is refused with "changed since you opened it". |
| Live | Readers get the current section from an in-process cache that a save clears and that is re-read at least every 10 seconds, so another panel process sees a change too. Clients built from a section (the GitHub App) are rebuilt when its version changes. |

## Writing a key without losing one

A Vaultix write can't share the database transaction, so a save never overwrites the key in use:

1. Each new key value is written to Vaultix under a new name, the field and the new section version
   (`/shippin/github/PRIVATE_KEY_V7`).
2. The section row and its history are saved, pointing at the new name, in one transaction with the
   audit entry. The name being replaced goes into `shippin_setting_retired_secrets`.
3. After the commit, the reaction `shippin.platform.setting.remove_retired` deletes the retired names
   from Vaultix. A failure stays in the table and the reconciler tries again.
4. If the save fails after the write, the new name is deleted straight away, and the retired-key sweep
   catches it if that delete fails too.

## Entities

| Entity | Who | Actions |
| --- | --- | --- |
| `platform/secret_store` (one per installation) | Staff with `platform.settings.read` / `write` | get, update, delete, test |
| `platform/setting` | Staff with `platform.settings.read` / `write` | list, get, update, history |

A section is a file `settings.go` in its owner's folder declaring `var Settings = settings.Section{...}`.
`go generate ./internal/actions` finds it, like actions and reactions, so adding a section is the whole
change. Field kinds: text, long text, URL, true or false, whole number with bounds, choice, list, key
and long key (multi-line, such as a PEM private key).

## Order

1. Done. The settings store, history and retired-key outbox; the secret store entity and its Vaultix
   port; the section generator.
2. Done. First sections: build runners, the job queue (live start and stop), the GitHub App (its client
   ID, client secret, private key and webhook secret go to Vaultix). The panel reads each where it is
   used. The job queue follows its section: turned on, the workers and the reconciler start; turned off,
   they take no more work and finish what they hold, and new work runs in the request; a changed timing
   starts new ones while the old finish. The GitHub App client is built from the section and its keys
   and kept until the section's version changes; with the section off, or a key that can't be read, every
   GitHub route answers as when the file turns GitHub off.
3. Done, not yet seen in a browser. The panel pages: secret store, sections, history. A section's
   first save moves its keys from the environment into the store, since the panel stops reading the
   environment for a section once it is saved there.
4. Next. The rest of the file sections and environment keys, and the keys still sealed in the database
   (host API secrets, the AgentHub credential), one owner at a time: mail, AI, identity
   provider tokens, staff team token, authorization, tenant databases, the customer Vaultix product,
   updates, branding. Then the about-eight `platform_settings` keys become sections and that table goes.
5. The bootstrap file shrinks to what CFG-01 allows: database, ClickHouse and cache addresses and
   credentials, the session key, the listen address and TLS files.

## Proved against a real Vaultix

`internal/platform/secret_store/rig_test.go` runs the check and the port's write, read and delete
against a disposable Vaultix (`deploy/vaultix/proof-secret-store.mjs`, see its README) and is skipped
without one. What it shows a real Vaultix does: a wrong client secret is a 401 at sign-in; an identity
outside the project is a 403 naming the missing membership; a missing environment, project, folder or
secret is a 404 whose message says which; creating a folder that exists is a 400 (taken as made);
creating a secret name that exists is a 400, so a name is written once; deleting a missing secret is a
404 (taken as done). The project's member role is enough for the panel's identity. Not covered: TLS,
token expiry and renewal, rate limits, and a Vaultix that restarts mid-save.

## Not decided here

- Rotating the machine identity's own client secret from the panel, through the Vaultix API, rather
  than pasting a new one.
- An installation with no Vaultix: today it can't save keys in the panel. A second secret store
  behind the same port would change that.
