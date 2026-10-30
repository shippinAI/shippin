FROM debian:13-slim@sha256:f6b08b09a05725f9431502d3b5714f5c4576e82c1a84fa1debe0c24a7e0c3ec4

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl debian-keyring gpgv python3 xorriso \
    && rm -rf /var/lib/apt/lists/*

COPY build-image.sh patch-boot-menu.py /usr/local/lib/shippin-image/
ENTRYPOINT ["sh", "/usr/local/lib/shippin-image/build-image.sh"]
