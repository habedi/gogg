# ---- Builder Stage ----
FROM golang:1.24-bookworm as builder

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
        && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Go module download
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build Gogg
RUN go build -o bin/gogg .

# ---- Final Stage ----
FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        libgl1 \
        libx11-6 \
        libxcursor1 \
        libxrandr2 \
        libxinerama1 \
        libxi6 \
        libasound2 \
        libxxf86vm1 \
        xvfb \
        ca-certificates \
        htop nano duf ncdu \
        && rm -rf /var/lib/apt/lists/* \
        && apt-get autoremove -y \
        && apt-get clean

# Set up directories
COPY --from=builder /app/bin/gogg /usr/local/bin/gogg
COPY scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Create a non-root user and group
RUN addgroup --system gogg && adduser --system --ingroup gogg gogg
RUN mkdir -p /config /downloads && chown -R gogg:gogg /config /downloads

# Set user and volume
USER gogg
VOLUME /config
VOLUME /downloads
ENV GOGG_HOME=/config

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
