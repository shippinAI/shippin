# Handoff: prove the primary image in a disposable VM

## Objective and current evidence

Implement a repeatable, isolated test that installs the current development ISO,
reboots, and exercises a real Incus container. The existing builder in
`build-image.sh` verifies Debian's signed checksum manifest, patches the normal boot
entries to request the `incus` package, and creates an unsigned derived image.
`README.md` describes the network mirror requirement and current limitations. A
successful image build or a boot menu screenshot does not prove installation or
Incus runtime. This handoff does not authorize use of a physical installation disk.

## Before changing code

Read the complete builder, boot-menu patcher, image README, current CI workflow and
this handoff. Review Debian installer and Incus documentation for the versions being
used, particularly unattended installation, package availability, container
prerequisites and the VM platform. Record sources, assumptions, test environment and
baseline hashes in a new project session. Adapt the design if those sources or the
code have changed; record any deviation rather than silently following a stale plan.

## Implementation

1. Choose and document a reproducible VM runner and its prerequisites. Give it a
   fresh, explicitly created disposable virtual disk and isolated test resources.
   Refuse a host disk, an existing disk or an ambiguous target. Do not automatically
   remove resources outside the test directory. Ensure cleanup on failure.
2. Build the existing ISO as documented. Keep test-only unattended installation
   settings and throwaway credentials outside the distributed image. Boot a normal
   installer entry that requests Incus; use a verified network mirror and capture
   serial or equivalent diagnostic output. Time out instead of hanging indefinitely.
3. Verify the guest actually installs, shuts down or reboots, and starts from its
   virtual disk without depending on the installer media. Observe installed Debian
   and Incus versions and check the package and service state. Do not write a
   permanent credential or reusable secret into tracked files or shared logs.
4. Initialize Incus with settings appropriate to the isolated guest, start a real
   container from a documented image source, verify it is running and responds to a
   command, then stop and delete it. Capture errors as well as results. If nested
   virtualization is unavailable, this does not prove Incus VM workloads.
5. Document exact commands, test environment, evidence locations and cleanup. Add
   the test to CI only where the runner supports its prerequisites; keep the ordinary
   fast tests available without virtualization or network access.

## Acceptance and negative checks

- From a clean checkout, independently reproduce the complete build, install,
  reboot and container lifecycle on a disposable VM. Save logs and exit statuses.
- Check that the installed system, not installer environment, reports the package
  and that the container command ran inside the guest.
- Demonstrate safe refusal for an existing or non-disposable disk, an unreachable
  mirror and a failed installation; failures must not be reported as passes. Check
  timeout and cleanup behavior. Test the refusal path without risking real disks.
- Run the existing panel and governance checks, review the full diff, and validate
  the session record and plan against project schemas. Record skipped checks and
  reasons; do not claim hardware, Secure Boot, offline install, first-owner setup,
  remote node enrollment or a production release from this test.

If virtualization, a mirror, or upstream image availability blocks execution,
record the exact failing command and environment. Leave this milestone incomplete
until the actual guest behavior has been observed.
