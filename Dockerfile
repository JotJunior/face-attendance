# syntax=docker/dockerfile:1

# --- Build stage (native arm64 on the Raspberry Pi) ---
FROM golang:1.26-alpine AS build
WORKDIR /src

# Cache dependencies first
COPY go.mod go.sum ./
RUN go mod download

# Build the static binary (pgx + amqp091 are pure Go, no CGO needed)
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /out/presenca-facial ./cmd/presenca-facial

# --- Runtime stage ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget \
    && adduser -D -u 10001 app
USER app
COPY --from=build /out/presenca-facial /usr/local/bin/presenca-facial
EXPOSE 8080
# Health check hits the unauthenticated /health endpoint
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=5 \
    CMD wget -qO- http://127.0.0.1:8080/health >/dev/null 2>&1 || exit 1
ENTRYPOINT ["presenca-facial"]
