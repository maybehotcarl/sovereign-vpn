# Multi-stage build for Sovereign VPN node
# Build context: project root (../)

# --- Builder ---
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY gateway/ gateway/
WORKDIR /src/gateway
RUN go build -o /gateway ./cmd/gateway

# --- Runtime ---
FROM alpine:3.21

RUN apk add --no-cache wireguard-tools iptables iproute2 bash

COPY --from=builder /gateway /usr/local/bin/gateway
COPY node/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 8080
EXPOSE 51820/udp

ENTRYPOINT ["/entrypoint.sh"]
