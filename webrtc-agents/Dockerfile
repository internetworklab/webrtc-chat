# Build stage
ARG BUILDPLATFORM

FROM --platform=$BUILDPLATFORM debian:trixie AS build-env

ARG TARGETARCH

ENV DEBIAN_FRONTEND=noninteractive

COPY webrtc-agents/deb-source-overrides.sources /etc/apt/sources.list.d/debian.sources

# Install build dependencies
RUN dpkg --add-architecture ${TARGETARCH} && apt-get update -y && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    golang-go \
    pkg-config \
    libopus-dev:${TARGETARCH} \
    libopusfile-dev:${TARGETARCH} \
    git \
    ca-certificates \
    crossbuild-essential-${TARGETARCH} \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Copy go.mod and go.sum files first for better layer caching
COPY webrtc-server/go.mod webrtc-server/go.sum ./webrtc-server/
COPY webrtc-agents/go.mod webrtc-agents/go.sum ./webrtc-agents/

# Download dependencies
WORKDIR /build

# Copy source code
COPY webrtc-server/ ./webrtc-server/
COPY webrtc-agents/ ./webrtc-agents/

WORKDIR /build/webrtc-agents
RUN CGO_ENABLED=1 GOOS=linux GOARCH=$TARGETARCH go mod download

ARG BUILDPLATFORM
FROM --platform=$BUILDPLATFORM build-env AS builder

# Build the binary
ARG TARGETARCH
ARG TARGETOS
RUN \
    mkdir -p /build/webrtc-agents/bin  && \
    tpl=$(v=$TARGETARCH l=$TARGETOS bash -c 'declare -A arch_translate=([amd64]=x86_64 [arm64]=aarch64); echo -n ${arch_translate[$v]}; echo -n "-"; echo -n "$l"; echo -n "-gnu"') && \
    PKG_CONFIG_PATH=/usr/lib/$tpl/pkgconfig CC=/usr/bin/$tpl-gcc CGO_ENABLED=1 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /build/webrtc-agents/bin/agents ./cmd/agents

# Runtime stage
# Note --platform=$TARGETPLATFORM is default behavior, so, no need to do that explicitly
FROM debian:trixie-slim

# Install runtime dependencies (only what's needed to run the binary)
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    libopus0 \
    libopusfile0 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /build/webrtc-agents/bin/agents /app/agents

ENTRYPOINT ["/app/agents"]
