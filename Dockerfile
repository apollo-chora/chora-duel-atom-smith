# syntax=docker/dockerfile:1.6
#
# chora-duel-atom-smith Dockerfile — standalone Go ADK agent crew
# (P1 single-agent with a web_research tool).
#
# Build context = this repository. Shared Chora modules (chora-adk-common,
# chora-contracts) are resolved through the Go module proxy, not a workspace.
#
# The crew is cloud-neutral: NATS is not used (no event-bus dependency),
# traces go to standard OTLP, and model calls route to chora-model-gateway
# over gRPC. No cloud account or managed service is required.

ARG GO_VERSION=1.26.6
ARG ALPINE_VERSION=3.23
ARG SERVICE_NAME=chora-duel-atom-smith
ARG GIT_SHA=unknown
ARG BUILD_TIME=unknown

############################
# Stage 1 — build
############################
FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS builder

ARG SERVICE_NAME
ARG GIT_SHA
ARG BUILD_TIME

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY . .

RUN go mod download

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64
RUN go build -trimpath \
      -ldflags "-s -w" \
      -o /out/duel_atom_smith \
      ./cmd/duel_atom_smith

############################
# Stage 2 — runtime
############################
FROM alpine:${ALPINE_VERSION}

ARG SERVICE_NAME
ARG GIT_SHA
ARG BUILD_TIME

RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S -G app app

LABEL org.opencontainers.image.title="${SERVICE_NAME}" \
      org.opencontainers.image.source="https://github.com/apollo-chora/chora-duel-atom-smith" \
      org.opencontainers.image.revision="${GIT_SHA}" \
      org.opencontainers.image.created="${BUILD_TIME}" \
      org.opencontainers.image.vendor="Chora Platform" \
      org.opencontainers.image.licenses="UNLICENSED" \
      io.chora.service="${SERVICE_NAME}" \
      io.chora.git-sha="${GIT_SHA}" \
      io.chora.build-time="${BUILD_TIME}"

WORKDIR /

COPY --from=builder /out/duel_atom_smith /duel_atom_smith

USER app:app
ENTRYPOINT ["/duel_atom_smith"]
CMD ["web", "-port", "8080", "agentengine"]
