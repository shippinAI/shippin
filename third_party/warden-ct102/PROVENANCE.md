# Warden panel from a Proxmox container (2026-08-27)

This tree is the source that was running as `warden.service` in a Proxmox container,
taken from `/var/lib/shippin/warden-src/warden` on 2026-10-31. It is shippin-mesh `main`
`9caff3d` `warden/` plus the backups subsystem that exists in no git history.

Not included:

- The 17 MB `warden` binary (left out of the archive before this import).
- `static/media/login-background.mp4` and `login-background.webm` (login background
  video, not panel logic).

`warden.env.example` is the panel's sample file. `PROXMOX_TOKEN_SECRET` is the
placeholder `REPLACE-WITH-YOUR-TOKEN-UUID`. S3 and Infisical secrets are empty
comments. The sample host `192.0.2.1`, node `server1`, and token id are not
live credentials and are not copied into console config.

`install/*.sh` are CRLF in this copy. Content matches mesh main after LF
normalization. Do not run them from this tree.

This directory is a nested Go module (`module warden`). The console does not
build it. The port lives in `internal/hv` and `internal/backupcov`.
