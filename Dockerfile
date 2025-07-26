# ---- Builder Stage ----
FROM golang:1.24-alpine AS builder

# Install only what's needed for building a static binary
RUN apk add --no-cache bash make musl-dev

# Set static build environment
ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

WORKDIR /src

# Leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Optional: clean prior builds
RUN make clean || true

# Build static CLI binary
RUN go build -trimpath -ldflags="-s -w -extldflags '-static'" -tags "cli" -o bin/gogg ./main.go

# ---- Final Stage ----
FROM alpine:latest

# Add CA certs if gogg needs to do HTTPS calls
RUN apk add --no-cache ca-certificates

# Copy static binary
COPY --from=builder /src/bin/gogg /usr/local/bin/gogg

# Create non-root user
RUN addgroup -S gogg && adduser -S gogg -G gogg
USER gogg

# Volumes
VOLUME /config
ENV GOGG_HOME=/config
VOLUME /downloads

# Run the CLI
ENTRYPOINT ["gogg"]
