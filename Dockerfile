FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ransomware-bot .

FROM alpine:latest

# Update and install necessary packages
RUN apk update && \
    apk upgrade --no-cache && \
    apk add --no-cache ca-certificates tzdata openssl && \
    rm -rf /var/cache/apk/*

WORKDIR /app

# Create non-root user for security
RUN adduser -D -s /bin/sh botuser

# Copy binary from builder
COPY --from=builder /app/ransomware-bot .

# Create directories and set permissions
RUN mkdir -p /app/logs /app/data /app/configs && \
    chown -R botuser:botuser /app

# Define volumes for persistence
VOLUME ["/app/configs", "/app/logs", "/app/data"]

# Switch to non-root user
USER botuser

# Start the bot
CMD ["./ransomware-bot", "--config-dir", "/app/configs"]