# ==============================================================================
# Multi-stage Dockerfile for Media_FS on Linux ARM64 (Raspberry Pi 5 / Debian)
# ==============================================================================

# Stage 1: Build stage
FROM golang:1.25-bookworm AS builder

# Install build dependencies: GCC compiler and FUSE headers required by cgofuse
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libfuse-dev \
    pkg-config \
    git \
    ca-certificates \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source tree
COPY . .

# Build flags
ARG TARGETOS=linux
ARG TARGETARCH=arm64
ARG VERSION=v1.2.5

# Compile statically optimized CGO binary
RUN CGO_ENABLED=1 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -trimpath \
    -ldflags="-s -w -X github.com/CCoupel/Media_FS/internal/version.Version=${VERSION}" \
    -o /build/mediafs ./cmd/mediafs

# Stage 2: Runtime stage
FROM debian:bookworm-slim

LABEL maintainer="spinline" \
      description="Media_FS Docker ARM64 mount for Jellyfin on Fygos NAS"

# Install FUSE runtime and essentials
RUN apt-get update && apt-get install -y --no-install-recommends \
    fuse \
    libfuse2 \
    ca-certificates \
    tzdata \
    curl \
 && rm -rf /var/lib/apt/lists/* \
 && sed -i 's/#user_allow_other/user_allow_other/' /etc/fuse.conf \
 && mkdir -p /mnt/jellyfin-remote /root/.config/mediafs /root/.cache/mediafs

# Copy binary and entrypoint
COPY --from=builder /build/mediafs /usr/local/bin/mediafs
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# Environment defaults
ENV MOUNT_POINT=/mnt/jellyfin-remote \
    MOUNT_OPTIONS=ro,allow_other \
    CHUNK_SIZE_MB=64 \
    PARALLEL_CHUNKS=4 \
    MAX_CACHE_MB=2048 \
    CACHE_TTL_MIN=120

WORKDIR /root

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["mount"]
