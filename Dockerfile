# The Shippin panel: the web bundle, the console binary and nothing about any one installation.
# Everything a site chooses comes from the environment at run time. See compose.yaml.

# Everything derived from the action files is generated again here, from the source alone: the
# registry, the panel's typed action map and the actions' OpenAPI document (docs/mcpl.md). The image
# is what the source says even if a committed copy is stale; the tests catch a stale copy.
# The Go toolchain is pinned to a patch release (go.mod names the same one): a floating tag hides which
# security fixes a build has.
FROM golang:1.26.11-bookworm AS source
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go generate ./internal/actions

FROM node:24-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
COPY api/ /src/api/
COPY --from=source /src/web/src/lib/api/actions.gen.ts src/lib/api/actions.gen.ts
COPY --from=source /src/api/openapi/actions.gen.json /src/api/openapi/actions.gen.json
RUN npm run generate:openapi && npm run build

FROM source AS server
# The version and channel come from the build (docs/release.md): CI passes them for edge builds and
# releases, and a plain `docker build` is the local development build. Go stamps the commit from .git,
# which is how the panel knows when it is behind; a source archive has no .git and builds unstamped.
ARG VERSION=0.0.0-dev
ARG CHANNEL=local
RUN LDFLAGS="-X shippin.cloud/console/internal/version.version=${VERSION} -X shippin.cloud/console/internal/version.channel=${CHANNEL}" \
  && { CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -o /out/shippin-console ./cmd/console \
  || CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags "$LDFLAGS" -o /out/shippin-console ./cmd/console; }

FROM debian:trixie-slim
RUN apt-get update -qq \
  && apt-get install -y -qq --no-install-recommends ca-certificates curl openssl python3 postgresql-client \
  && rm -rf /var/lib/apt/lists/* \
  && useradd --system --uid 10001 --home-dir /var/lib/shippin shippin \
  && install -d -o shippin -g shippin -m 0700 /var/lib/shippin
COPY --from=server /out/shippin-console /usr/local/bin/shippin-console
COPY --from=web /src/web/dist /opt/shippin/web/dist
COPY config/shippin.example.json config/templates.json /opt/shippin/config/
COPY install/panel/render-config.py /opt/shippin/install/panel/render-config.py
COPY --chmod=0755 install/docker/entrypoint.sh /opt/shippin/entrypoint.sh
COPY config/templates.json /etc/shippin/templates.json
USER shippin
ENV SHIPPIN_LISTEN=:8443
EXPOSE 8443
VOLUME /var/lib/shippin
HEALTHCHECK --interval=10s --timeout=5s --retries=30 CMD curl -fsk https://localhost:8443/healthz || curl -fs http://localhost:8443/healthz
ENTRYPOINT ["/opt/shippin/entrypoint.sh"]
