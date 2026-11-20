# Documentation

Start with [What Shippin is](objective.md). The [README](../README.md) gets it
running.

## The product

- [What Shippin is](objective.md): the self-hosted panel and the enterprise module; the contract every change follows, with requirement IDs.
- [Roadmap](roadmap.md): what exists today, and the order of the rest, starting with separating the enterprise code.
- [Design system](design-system.md): tokens, accessibility and speed rules for the panel.
- [Panel framework](web-framework.md): routes and navigation from folders, the lexicon, rot checks, tokens and components.
- [Builds and releases](release.md): the local, edge and release channels, versions, what a release carries and the checks before anything is published.
- [MCPL](mcpl.md): one canonical action surface for people, the API and an AI acting for a person; the rules every route and input follows.
- [Build runners](runners.md): GitHub Actions jobs on single-use runner machines, per organization (RUN-01).
- [Settings and secrets](settings.md): every setting edited in the panel, and keys saved to the installation's Vaultix account (CFG-01).
- [Metrics](metrics.md): host and guest CPU, memory, disk and network sampled into ClickHouse, the metric names, and the range read (OBS-01).
- [Editions and licensing](editions.md): what belongs to the panel and what to the enterprise module, how the module plugs in, FSL-1.1 and the license key (OPEN-01).
- [Host shell](host-shell.md): a design, not built, for an audited staff shell on an Incus or Proxmox host.
- [Code map](architecture.md): the contexts, where the code stands against them, and the order of the rewrite.

## Run it and change it

- [Deploy](deploy.md): containers or a Debian/Ubuntu machine, TLS, upgrades.
- [Develop](development.md): the local loop and the checks to run before pushing.
- [Attach a hypervisor](reference/proxmox-connector.md): Incus and Proxmox.
- [Source ownership](source-of-truth.md): which folder owns what.
- [Assistant skills](../.agents/library/README.md): the canonical library external assistants load after a flat export.
- [Panel variants](panel-variants.md): every other place a panel lived, and what became of it.
- [Hosting plans](plans.md): the plans the catalog and host agent use today, and the coverage tiers beside them. Enterprise; moves to the enterprise repository.

## How the parts work

[reference/](reference/) describes the parts as they are designed: hypervisor
drivers, identity and access, staff roles, approval, deployments, managed
services, devspaces, enrollment and more. Each page says which parts are built.

## Where older code went

[absorption/](absorption/) is the ledger of capabilities taken from earlier
Shippin, Warden and Proxmox panels, and what remains to re-author.

## History

[archive/](archive/) keeps superseded plans and reviews. They were true when
written and do not describe the product now.
