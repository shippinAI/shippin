# Primary installation

The current development ISO builds on Debian and requests Incus from a network mirror.
It is unsigned and does not install the Shippin control plane or panel. It is not a
production appliance. See the [image builder](../install/primary-image/README.md)
for exact build instructions and limitations.

The intended deployment has a primary server hosting the control plane, durable state,
identity and management interface. Additional physical servers or VMs enroll as
infrastructure nodes through an installer or command issued by that primary. Hosts
and VMs need explicit roles and configurable tags. Incus is the first provider;
Proxmox and other services are future integrations. None of this topology is
implemented by the current image.

A future first-run flow must let the owner establish an account through a unique,
short-lived bootstrap mechanism over trusted HTTPS, with recovery and backup. It must
not rely on a shared factory password. Node enrollment needs independently revocable
credentials and must not silently change cluster membership or erase disks.

See the [release roadmap](roadmap.md) and the [next implementation handoff](../install/primary-image/implementation-plan.md).
