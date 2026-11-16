# Editions and licensing

How Shippin is licensed and how the enterprise edition is built (OPEN-01). Design only, except for the license itself.

## The license

This repository is licensed under the [Functional Source License 1.1, Apache 2.0 future license](../LICENSE.md) (FSL-1.1-ALv2). In plain terms, and the license text is what counts:

- Anyone may use, copy, modify and self-host it, for any purpose except a competing use.
- A competing use is making it available as a commercial product or service that substitutes for Shippin, such as selling hosting run on it. That needs a commercial license from shippinAI.
- Each version also becomes available under Apache 2.0 two years after it is released.

Outside contributions need a contributor agreement first, so that contributed code can also ship in the enterprise edition. None is set up yet; until it is, only shippinAI's own commits are accepted.

## One panel, an enterprise module

There is one panel: this repository's `web/` and `cmd/console`. The enterprise edition is not a fork. It is a separate, private module that this panel loads:

| Part | Where | License |
| --- | --- | --- |
| The panel, the control plane, the host agent, every self-hosted feature | This public repository | FSL-1.1-ALv2 |
| Enterprise features | A private repository that imports this one as a pinned dependency and adds to it through extension points | Commercial, under an agreement with shippinAI |
| The enterprise build | An image built from the private repository: this panel plus the module | Commercial |

The module never copies or patches panel code. When it needs something the panel does not offer, the panel gains an extension point here, in public, and the module uses it. So a fix in the panel reaches both editions in the next build.

## Extension points (to build)

- **Actions.** The module adds MCPL actions in its own `internal/<domain>/<entity>/<verb>.go` files. The generator finds them the same way, so they get routes, audit, events, the OpenAPI document and typed panel calls like any other action.
- **Settings sections.** The module declares `settings.go` sections, which appear on the Settings page.
- **Panel pages.** The module registers routes and navigation entries through the panel's route and nav configuration. They load lazily, so the self-hosted build stays within its budget.
- **Permissions.** The module adds permissions to the catalog; staff presets and grants work unchanged.

## The license key (to build)

- An enterprise build runs the module's features only with a valid key: a small file signed with shippinAI's private key. It names the customer, the edition, the features, the number of hosts or customers allowed, and an expiry date.
- The panel checks the signature offline against the public key built into the enterprise image, so an unmanaged installation never has to call home.
- Settings has a page to upload the key. It shows what it covers and when it expires, and warns 30 days before it expires.
- When a key expires, the enterprise features turn read-only for a grace period, then off. Self-hosted features, customer data and running workloads are never affected.
- The self-hosted build has no key check at all, because it contains no enterprise code.

## Managed and unmanaged

Both run the same enterprise image:

- **Managed:** shippinAI runs the installation for the customer.
- **Unmanaged:** the customer downloads the image and runs it on their own machines with their key.

## Not decided here

- Which features are enterprise-only. A feature a solo self-hoster needs stays in this repository (SOLO-01).
- The enterprise repository's name.
- Whether a key also limits usage (hosts, customers) or only enables features.
