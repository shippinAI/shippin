# Identity and access

The console already signs people in as an OIDC client of an external issuer (`internal/auth`) and stores the browser session in Postgres (`sessions`, encrypted with `SHIPPIN_SESSION_KEY`). This document is the model for the directory that sits on that same database and session cookie. It does not add a second login store.

A site turns the pieces on through `config/shippin.example.json` (`directory`) and `/etc/shippin/site.env`. Nothing here hardcodes a host, tenant, or secret.

## Accounts

`identity_users` is the local account: email, verified flag, display name, and role `owner`, `admin`, or `member`. `identity_orgs` and `identity_members` are the local org membership. They are not Incus projects and they do not replace `tenants`.

The first owner is created only by redeeming a one-time setup code. The installer runs `shippin-console setup`, which prints `setup_code=...` once and stores a SHA-256 hash. There is no "first visitor becomes admin" path. A second call while an owner already exists prints nothing and creates nothing.

After that, a person can hold:

- a passkey (WebAuthn, ES256). The server stores the credential id, the P-256 public key, and the signature counter.
- recovery codes (single use, stored as hashes, shown once at setup).
- email magic links (single use, hashed, short expiry).

Sessions stay in the existing `sessions` table. A local session sets `uid` on the encrypted payload and uses the same `shippin_session` cookie. The panel API accepts it like an IdP token: the user's directory orgs become org memberships, and the installation owner holds `operator.role`.

## This console as an OIDC provider

The panel is its own authorization server for its own clients.

- `GET /.well-known/openid-configuration`
- `GET /oauth/authorize` (local session required, PKCE S256 required)
- `POST /oauth/token` (authorization code, then refresh)
- `GET /oauth/userinfo`

The public client id defaults to `shippin-panel`. Redirect URLs come from `directory.redirects`, or `https://<console_host>/auth/callback` when that list is empty. Refresh tokens rotate. Reusing an old refresh token revokes the family.

## Upstream Nomen and other issuers

Nomen remains an optional upstream. The existing `idp.issuer` and `idp.client_id` (or `SHIPPIN_NOMEN_ISSUER` and `SHIPPIN_OIDC_CLIENT_ID`) keep the current BFF login at `/auth/login`.

`directory.upstreams[]` adds issuers that are discovered at `{issuer}/.well-known/openid-configuration`. Each entry needs `id`, `issuer`, and `client_id`. The client secret, when the issuer requires one, is the env var named by `secret_env`. An entry with an empty issuer or client id is ignored.

On callback the directory links `(issuer, sub)` to one local user:

- the same pair returns the same user
- a verified email that matches one local user attaches the link to that user
- a second subject at the same issuer for a user who already has a different subject is rejected
- otherwise a new local user is created only when signup policy allows it

`directory.upstream_required: true` refuses local magic-link signup. Bootstrap and passkeys already registered still work. The site must then have a client id configured.

## Signup policy

`directory.signup` is `open`, `invite_only`, or `waitlist`.

- `open` lets a new verified email create a member.
- `invite_only` requires an unexpired invite with uses left.
- `waitlist` does not create members from the public form. An admin invite does.

Invites have a hash, optional email, role, optional org, max uses, and expiry. Redeeming one consumes a use and writes an audit row.

## Waitlist

`POST /v1/waitlist` takes email, name, optional company and notes, and a captcha token. It is rate limited per client address. `directory.captcha.provider` is `none` or `turnstile`. `turnstile` checks Cloudflare's siteverify with the secret in the env var named by `secret_env`. `none` does not accept a token as proof of a person; it only means the site chose not to run a captcha.

The row starts `pending`. The confirmation mail is a second single-use token. Confirming moves it to `confirmed`. Admins can list, search, export CSV, and bulk-invite confirmed rows (that issues an invite and moves the row to `invited`). Later states are `joined` and `rejected`.

Mail uses the console SMTP transport when `mail.enabled` is true. `SHIPPIN_MAIL_SINK` names a file that receives the same messages for a disposable install. Leave it unset on a real site.

## Admin configuration

`GET /v1/admin/config` returns the directory and brand the process is using, plus a monotonic `version`. `PUT /v1/admin/branding` stores an overlay (name, logo URL, color, privacy URL, terms URL) that `GET /v1/settings/public` merges onto the config file. The overlay is data in `identity_settings`, not a rewrite of the example file.

Callers are the owner session or a scoped API key (`shp_...`, stored as a hash). Scopes are `waitlist:read`, `waitlist:write`, `invite:write`, `config:read`, `config:write`, and `audit:read`. Creating a key returns the secret once.

Outbound webhooks are HTTPS URLs and an event list. The body is signed `Shippin-Signature: t=<unix>,v1=<hex>` with HMAC-SHA256 over `timestamp.body`. The secret is shown once. Events emitted by this directory are `user.signup`, `waitlist.joined`, `invite.redeemed`, and `host.enrolled` (from the existing server enroll handler, without the host credential).

## Threat notes

- Setup codes, magic links, invites, and recovery codes are stored as hashes. The database is the secret store for webhook signing keys.
- Cookies stay `HttpOnly`, `Secure`, and `SameSite=Lax`, matching the existing session cookie.
- PKCE `S256` is required for the local authorization server. A missing verifier is a rejection.
- Upstream email merge runs only when the issuer says the email is verified.
- Captcha `none` is an explicit site choice. `turnstile` with an empty secret fails closed.
- Rate limiting is per process. Several consoles do not share the counter; put the public form behind one address or add a shared limiter later.
- `SHIPPIN_MAIL_SINK` writes confirmation links to disk. It is for the smoke test and other disposable machines.
