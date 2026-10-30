# Debian and Incus installer: development foundation

This builds an **unsigned development image**, not a finished Shippin.Cloud appliance.
It starts from Debian 13.7.0 amd64 netinst, checks Debian's signed SHA-256 manifest,
and adds `pkgsel/include=incus` to the normal graphical and text installer choices.
Debian's installer still asks for account and disk choices; this image supplies no
passwords and does not preselect disks. Installation requires an internet-connected
Debian package mirror. Incus is requested during installation, not bundled offline.
The advanced, rescue and automated installer choices remain Debian's unchanged
choices and do not request Incus. Do not use them as a Shippin setup flow.

The image does **not** provide the Shippin.Cloud control plane, panel, first-owner
bootstrap, node installer, artifact signature, SBOM or a reproducible pinned package
snapshot. It is unsuitable for production or distribution as a finished appliance.
The build verifies upstream Debian media; its generated checksum detects accidental
corruption but does not authenticate the derived image. Until the release pipeline
signs derived images with independently distributed keys, keep the ISO local.

## Build

Requires Docker, internet access, and approximately 3 GB of free space. From the
repository root in PowerShell:

```powershell
New-Item -ItemType Directory -Force .cache/primary-image/output | Out-Null
docker build -t shippin-primary-image-dev install/primary-image
docker run --rm -v "${PWD}/.cache/primary-image/output:/output" shippin-primary-image-dev
```

The result and its checksum are in `.cache/primary-image/output/`, which is ignored
by Git. The build refuses to overwrite an existing ISO. Move or rename an old output
if a new build is needed. The build fetches the pinned ISO and signed checksum from
Debian; alternatively mount a previously downloaded exact image read-only at `/input`:

```powershell
docker run --rm -v "${PWD}/.cache/primary-image/input:/input:ro" -v "${PWD}/.cache/primary-image/output:/output" shippin-primary-image-dev
```

An input image is always checked against the signed manifest. The builder is pinned
to an image digest, but its installed tools and the Debian packages selected during
installation can change as security updates are published: this is not yet a
reproducible release. Never turn off package signature checks to make a build pass.

Test only in a disposable VM with a disposable disk. Select the normal graphical or
text installation, choose the installation disk explicitly, create your own OS
credentials, configure a working network mirror, and check that `incus --version`
works on the installed system. A VM needs nested virtualization to run Incus VMs;
containers can still be evaluated separately. The installer modifies the disk you
select. Physical hardware, Secure Boot, full installation and Incus runtime must be
independently tested before claiming support.

See [primary installation design](../../docs/primary-installation.md) for the
remaining first-owner, release signing, node enrollment and acceptance requirements.
