# ── Build stage ──────────────────────────────────────────────
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /bin/smoll-url ./cmd/smoll-url

# ── Runtime stage ────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

RUN addgroup -S smoll && adduser -S smoll -G smoll

WORKDIR /app

COPY --from=builder /bin/smoll-url /app/smoll-url

RUN mkdir -p /data && chown smoll:smoll /data

USER smoll

ENV port=4567
ENV listen_address=0.0.0.0
ENV db_url=/data/urls.db
ENV use_wal_mode=true

EXPOSE 4567

VOLUME ["/data"]

ENTRYPOINT ["/app/smoll-url"]
