# Shippin.Cloud Control Panel (deployable baseline)

This folder provides a centralized, deployable control panel shell for managing Shippin.Cloud through the control-plane API.

This is an API-workbench prototype; it is not the final customer console or a
claim that the control-plane backend, Incus provider, or node installer exists.
See [source ownership](../docs/source-of-truth.md) before adopting other code.

## Source structure

```text
src/
  main.tsx                 Browser entrypoint
  app/                     Application composition, routes and global styles
  features/auth/           OIDC interaction
  features/configuration/  Browser-scoped connection settings
  features/workbench/      API workbench requests
  shared/                  Shared contracts, runtime configuration and generated index
scripts/                   Build and structure verification
```

Use lowercase kebab-case for new file and directory names. Place feature-specific
UI, state, adapters and tests with the owning feature; put code in `shared/` only
when it is genuinely shared and provider-neutral. `app/` composes features.
Features may import their own files and shared code, not another feature or the
app; shared code may not depend on features, app or the entrypoint. Avoid empty
speculative module folders. The static boundary checker covers TypeScript module
imports and literal dynamic imports and checks source names; it does not establish runtime isolation or
replace security and integration tests. This Vite app does not implement Next.js
file-system routing.

`api/openapi/` remains the API contract owner. The index in
`src/shared/generated/` is generated from it by `npm run generate:openapi-index`;
do not edit the index by hand. The broader console and provider ownership decisions
remain documented in [source ownership](../docs/source-of-truth.md).

## What this baseline includes

- OpenAPI-indexed navigation generated from [`api/openapi/openapi.yaml`](../api/openapi/openapi.yaml)
- Runtime environment config file (`public/config/runtime-config.json`) for API base URL and metadata
- Customer/operator app mode switch with role-aware operation visibility
- Customer/operator route tree split for centralized navigation context
- `/me` identity sync to centralize tenant and role context in one panel
- OIDC code+PKCE sign-in support (runtime-configurable for Zitadel)
- Session expiry handling (JWT exp warning + forced local sign-out on expiry)
- Capability-level action guard using `/me.capabilities` and config hints
- A built-in API workbench for authenticated calls to any indexed operation
- Docker image + Compose deployment for one-command hosting

## Local development

Use Node.js 24 or newer for the TypeScript-aware test runner.

```bash
cd panel
npm install
npm run dev
```

Default URL: `http://localhost:5173`

Optional base path while developing a split origin/path build:

```bash
cd panel
$env:PANEL_BASE_PATH="/console/"
npm run dev
```

## Build

```bash
cd panel
npm run build
```

The build includes TypeScript and dependency-boundary checks. Run `npm test` for
the boundary checker's regression tests.

## Deploy with Docker Compose

From the repository root:

```sh
docker compose -f panel/docker-compose.yml up -d --build
```

Default URLs:

- Customer console: `http://localhost:8088/console/`
- Operator console: `http://localhost:8089/ops/`

Note: compose builds from the repository root context so the OpenAPI source at [`api/openapi/`](../api/openapi/) is available during image build.
Both ports bind to loopback by default. Set `SHIPPIN_BIND_ADDRESS`,
`SHIPPIN_CONSOLE_PORT` or `SHIPPIN_OPS_PORT` before running Compose to choose a
different bind address or ports. Binding beyond loopback exposes this prototype;
configure trusted HTTPS and authentication before any remote use.

For an update, pull the desired revision and rebuild from that checkout:

```sh
git pull --ff-only
docker compose -f panel/docker-compose.yml up -d --build
```

Stop this Compose project with `docker compose -f panel/docker-compose.yml down`.
Images are built locally from the checked-out source; this does not pull a signed
Shippin release image or create a control plane. Back up your own runtime config
before updates and use version-controlled release tags when those exist.

## Runtime config

Use **Connection settings** in the workbench to change the display name, API URL,
documentation URL and OIDC client settings for this browser tab. Saving clears the
current access token; settings survive a refresh in the same tab, not a new tab or
another browser. This prototype does not implement installation-wide settings or
their server-side authorization. Do not enter client secrets in this form.
OIDC authorization and token endpoints are discovered from the configured issuer;
the issuer must return a matching discovery document and secure endpoints.

Administrators can also edit [`runtime-config.json`](./public/config/runtime-config.json)
to configure defaults for the deployment:

```json
{
  "appName": "Example Control Panel",
  "environment": "local",
  "apiBaseUrl": "http://localhost:8080/v1",
  "docsUrl": "",
  "defaultTenantId": "",
  "appMode": "customer",
  "capabilityHints": {
    "createWorkspace": "workspace:create",
    "listWorkspaces": "workspace:read"
  },
  "auth": {
    "enabled": false,
    "issuerUrl": "",
    "clientId": "panel-spa",
    "audience": "",
    "scopes": "openid profile email",
    "redirectUri": "",
    "sessionExpiryWarningSeconds": 120
  }
}
```

For split deployment, use:

- [`runtime-config.console.json`](./public/config/runtime-config.console.json)
- [`runtime-config.ops.json`](./public/config/runtime-config.ops.json)

The compose file mounts those runtime configs into each deployed container, so API/auth targets can be changed without rebuilding.

`capabilityHints` maps OpenAPI `operationId` to the capability key returned by `/me.capabilities`. If a mapped capability is `false`, the action is disabled in UI before request execution.
