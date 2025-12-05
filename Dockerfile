# Build Stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Copy go mod files for better layer caching
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build with optimizations and security hardening
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -a \
    -ldflags='-w -s -extldflags "-static"' \
    -trimpath \
    -o ransomware-bot \
    .

# Runtime Stage
FROM alpine:3.21

# Install runtime dependencies and security updates
RUN apk update && \
    apk upgrade --no-cache && \
    apk add --no-cache \
        ca-certificates \
        tzdata \
        tini && \
    rm -rf /var/cache/apk/*

WORKDIR /app

# Create non-root user with specific UID/GID for consistency
RUN addgroup -g 1000 botuser && \
    adduser -D -u 1000 -G botuser -s /sbin/nologin botuser

# Copy binary from builder
COPY --from=builder --chown=botuser:botuser /build/ransomware-bot /app/

# Create directories with proper permissions
RUN mkdir -p /app/logs /app/data /app/configs && \
    chown -R botuser:botuser /app && \
    chmod 755 /app && \
    chmod 750 /app/logs /app/data && \
    chmod 755 /app/configs

# Define volumes for persistence
VOLUME ["/app/configs", "/app/logs", "/app/data"]

# Switch to non-root user
USER botuser

# Health check (optional - adjust based on your needs)
# HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
#   CMD pgrep ransomware-bot || exit 1

# Set environment variables
ENV TZ=UTC \
    CONFIG_DIR=/app/configs \
    LOG_DIR=/app/logs \
    DATA_DIR=/app/data

# Use tini as init system to handle signals properly
ENTRYPOINT ["/sbin/tini", "--"]

# Start the bot
CMD ["/app/ransomware-bot", "--config-dir", "/app/configs"]
