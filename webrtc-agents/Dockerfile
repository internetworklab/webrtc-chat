# Build stage
ARG BUILDPLATFORM

FROM --platform=$BUILDPLATFORM debian:trixie AS build-env

ARG TARGETARCH

# Install build dependencies
RUN dpkg --add-architecture ${TARGETARCH} && apt-get update -y && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    golang-go \
    pkg-config \
    libopus-dev:${TARGETARCH} \
    libopusfile-dev:${TARGETARCH} \
    git \
    ca-certificates \
    crossbuild-essential-${TARGETARCH} \
    && rm -rf /var/lib/apt/lists/*

# gcc-14-multilib-x86-64-linux-gnu \

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
RUN \
    mkdir -p /build/webrtc-agents/bin  && \
    PKG_CONFIG_PATH=/usr/lib/x86_64-linux-gnu/pkgconfig CC=/usr/bin/x86_64-linux-gnu-gcc CGO_ENABLED=1 GOOS=linux GOARCH=$TARGETARCH go build -o /build/webrtc-agents/bin/agents ./cmd/agents

# Runtime stage
FROM debian:trixie-slim

# Install runtime dependencies (only what's needed to run the binary)
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    libopus0 \
    libopusfile0 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --shell /bin/bash appuser

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /build/webrtc-agents/bin/agents /app/agents

# Use non-root user for security
USER appuser

# Default environment variables (can be overridden at runtime)
ENV WS_SERVER=ws://localhost:3001/ws

ENTRYPOINT ["/app/agents"]
