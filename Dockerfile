# syntax=docker/dockerfile:1
# Multi-platform build: linux/amd64 and linux/arm64 (Oracle A1 Flex).
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
ARG TARGETOS=linux
ARG TARGETARCH=amd64
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/funda ./cmd/funda

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && addgroup -S -g 10001 funda && adduser -S -u 10001 -G funda funda
WORKDIR /app
COPY --from=builder /out/funda /usr/local/bin/funda
RUN mkdir -p /data /config && chown -R funda:funda /data /config
USER funda
ENV FUNDA_DB=/data/funda.db \
    FUNDA_HEALTH_FILE=/data/last-successful-check \
    TZ=Europe/Amsterdam
VOLUME ["/data"]
# Health is based on a completed monitoring cycle, not an HTTP request.
# Set --max-age greater than your monitor.json interval plus expected crawl duration.
HEALTHCHECK --interval=5m --timeout=10s --start-period=10m --retries=3 \
  CMD ["/usr/local/bin/funda", "health", "--max-age", "30m"]
ENTRYPOINT ["/usr/local/bin/funda"]
CMD ["watch", "--config", "/config/monitor.json"]
