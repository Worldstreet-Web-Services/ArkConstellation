# ArkConstellation devnet faucet - reuses the arkd binary from the node
# image to sign/broadcast, adds a stdlib-only Python HTTP server on top.
ARG ARKD_IMAGE=ghcr.io/worldstreet-web-services/arkd:ark-v1.0.1
FROM ${ARKD_IMAGE}

USER root
RUN apk add --no-cache python3

COPY ops/docker/faucet/faucet.py /faucet.py
COPY ops/docker/faucet/entrypoint-faucet.sh /entrypoint-faucet.sh
RUN chmod +x /entrypoint-faucet.sh

# Pre-create the faucet home so the faucet-data volume mount point inherits
# nonroot ownership instead of coming up root-owned (Docker only copies
# ownership into a fresh named-volume mount if the path already exists in
# the image).
RUN mkdir -p /home/nonroot/.ark/faucet && chown -R nonroot:nonroot /home/nonroot/.ark

USER nonroot
EXPOSE 8088
ENTRYPOINT ["/entrypoint-faucet.sh"]
