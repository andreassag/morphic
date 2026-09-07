# Stage 1: Build binary
FROM golang:alpine AS builder

WORKDIR /app

# Install build tools
RUN apk add --no-cache git gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bin/morphic ./cmd/morphic

# Stage 2: Runtime environment
FROM alpine:3.21

# Install ffmpeg, ca-certificates, and hardware acceleration libraries
RUN apk add --no-cache \
    ffmpeg \
    ca-certificates \
    tzdata \
    libva \
    mesa-va-gallium \
    curl \
    && rm -rf /var/cache/apk/*

WORKDIR /app

# Create non-root app user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=builder --chown=appuser:appgroup /app/bin/morphic /usr/local/bin/morphic

# Default media and safe trash mount directories
RUN mkdir -p /media /app/data && chown -R appuser:appgroup /media /app/data

USER appuser

EXPOSE 8001

ENV DATABASE_URL=""
ENV TRASH_RETENTION_DAYS="30"
ENV WATCH_POLL_INTERVAL_SECS="30"

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8001/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/morphic", "--host=0.0.0.0", "--port=8001"]
