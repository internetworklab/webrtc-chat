# Build stage
FROM --platform=$BUILDPLATFORM debian:trixie AS builder

ARG TARGETARCH
ARG TARGETOS

# Install Go
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    golang-go \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Copy go.mod and go.sum files first for better layer caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary (no CGO needed for webrtc-server)
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /build/bin/server .

# Runtime stage
FROM --platform=$TARGETPLATFORM debian:trixie-slim

# Install runtime dependencies
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /build/bin/server /app/server

ENTRYPOINT ["/app/server"]
