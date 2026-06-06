# syntax=docker/dockerfile:1

# ── Build stage ─────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Cache Go modules before copying source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-s -w" -o eerp-mcp-server ./cmd/server

# ── Runtime stage ────────────────────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /build/eerp-mcp-server .
COPY configs/ configs/

# Docs are mounted at runtime via docker-compose.
VOLUME ["/docs"]

EXPOSE 8080

ENTRYPOINT ["/app/eerp-mcp-server", "--config", "/app/configs/config.yaml"]
