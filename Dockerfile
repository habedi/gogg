# ---- Builder Stage ----
FROM golang:1.24-bookworm as builder

# Make sure /tmp exists and is writable
RUN mkdir -p /tmp && chmod 1777 /tmp

# Install build dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        build-essential \
        pkg-config \
        libgl1-mesa-dev \
        libx11-dev \
        libxcursor-dev \
        libxrandr-dev \
        libxinerama-dev \
        libxi-dev \
        libxxf86vm-dev \
        libasound2-dev \
        ca-certificates \
        htop nano duf ncdu \
        && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Go module download
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build GUI (default)
RUN go build -o bin/gogg .

# ---- Final Stage ----
FROM debian:bookworm-slim

# GUI and network deps
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        libgl1 \
        libx11-6 \
        libxcursor1 \
        libxrandr2 \
        libxinerama1 \
        libxi6 \
        libasound2 \
        ca-certificates \
        && rm -rf /var/lib/apt/lists/*

# Copy binary
COPY --from=builder /app/bin/gogg /usr/local/bin/gogg

# Optional: non-root user
RUN addgroup --system gogg && adduser --system --ingroup gogg gogg
USER gogg

# Volumes for config & downloads
VOLUME /config
VOLUME /downloads
ENV GOGG_HOME=/config

ENTRYPOINT ["gogg"]
