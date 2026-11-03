# The Shippin panel: the web bundle, the console binary and nothing about any one installation.
# Everything a site chooses comes from the environment at run time. See compose.yaml.

FROM node:24-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
COPY api/ /src/api/
RUN npm run build

FROM golang:1.26-bookworm AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Go stamps the commit from .git, which is how the panel knows when it is behind.
# A source archive has no .git and builds unstamped.
RUN CGO_ENABLED=0 go build -trimpath -o /out/shippin-console ./cmd/console \
  || CGO_ENABLED=0 go build -buildvcs=false -trimpath -o /out/shippin-console ./cmd/console

FROM debian:trixie-slim
RUN apt-get update -qq \
  && apt-get install -y -qq --no-install-recommends ca-certificates curl openssl python3 \
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
