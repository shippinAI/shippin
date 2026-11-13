# Runner fleet health

The reference probe lives in `third_party/warden-ct102/ci_runners.go`. That
package is not served, and its HTML template is not mounted. `internal/runfleet`
re-authors the pill rule and is what `cmd/shippin-run` calls.

## Signals

Each runner has three signals.

| Signal | When it is ok | When it is warn | When it is critical | When it is unknown |
| --- | --- | --- | --- | --- |
| GitHub | A configured probe reports a status that starts with `online`. Busy runners still count as online. | — | The runner name is missing or not online for a repository it serves. | GitHub credentials are not configured, the probe failed, or the runner has no repositories. |
| Substrate | Proxmox reports the guest `running`. A QEMU guest whose agent answers is described as alive. | The guest is running and the QEMU guest agent does not answer. | Proxmox reports a status other than `running`. | No guest is recorded, Proxmox is not configured, or the provider is Incus. Incus probes are not implemented. |
| Host load | CPU and memory are both under 85%. | Either one is at least 85%. | — | The guest is not running, or the probe did not return numbers. |

## Overall

`Compose` matches the reference rule:

- substrate critical or GitHub critical becomes critical
- any warn becomes warn
- all three unknown stays unknown
- every other mix is ok

`TestCompose` locks those cases. Health is computed when asked. It is not stored.
There is no stats history. The reference file mentioned a later stats layer;
this repository does not implement it.

## Fleet file

Runners are `ci_runners.json` in the state directory. The schema is
[config/ci_runners.schema.json](../../config/ci_runners.schema.json). See
[configuration](../reference/shippin-run.md).
