# Documentation

Start with [What Shippin is](objective.md). The [README](../README.md) gets it
running.

## The product

- [What Shippin is](objective.md): software for running a commercial cloud provider; the contract every change follows, with requirement IDs.
- [Roadmap](roadmap.md): what exists today, and the order toward the friends-and-family launch and then a commercial cloud.
- [Design system](design-system.md): tokens, accessibility and speed rules for the panel.
- [MCPL](mcpl.md): one canonical action surface for people, the API and an AI acting for a person; the rules every route and input follows.
- [Code map](architecture.md): the contexts, where the code stands against them, and the order of the rewrite.

## Run it and change it

- [Deploy](deploy.md): containers or a Debian/Ubuntu machine, TLS, upgrades.
- [Develop](development.md): the local loop and the checks to run before pushing.
- [Attach a hypervisor](reference/proxmox-connector.md): Incus and Proxmox.
- [Source ownership](source-of-truth.md): which folder owns what.
- [Panel variants](panel-variants.md): every other place a panel lived, and what became of it.
- [Hosting plans](plans.md): the plans the catalog and host agent use today, and the coverage tiers beside them.

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
