FROM  golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ransomware-bot .

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
RUN adduser -D -s /bin/sh botuser
COPY --from=builder /app/ransomware-bot .
RUN mkdir -p /app/logs /app/data && \
    chown -R botuser:botuser /app
USER botuser
CMD ["./ransomware-bot", "--config-dir", "/app/config"]