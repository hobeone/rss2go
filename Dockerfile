# Stage 1: Build the Svelte UI
FROM oven/bun:1.3-alpine@sha256:5acc90a93e91ff07bf72aa90a7c9f0fa189765aec90b47bdbf2152d2196383c0 AS ui-builder
WORKDIR /ui
COPY frontend/package.json frontend/package-lock.json* ./
RUN bun install
COPY frontend/ ./
RUN bun run build

# Stage 2: Build the Go application
FROM golang:1.26.5-alpine@sha256:99e12cfb19b753915f9b9fdc5a99f1869a24a69d3a0955832d5702e7fa68f1be AS go-builder
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Copy the built UI assets from Stage 1 into Go's embedding directory
COPY --from=ui-builder /internal/server/ui/dist /src/internal/server/ui/dist
RUN git config --global --add safe.directory /src || true
RUN GOOS=linux go build -v -buildvcs=true -o /app/rss2go ./cmd/rss2go

# Stage 3: Final lightweight execution environment
FROM alpine:3.23@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40
RUN apk add --no-cache ca-certificates tzdata su-exec
RUN adduser -D rss2go
RUN mkdir -p /app/config /app/db && chown -R rss2go:rss2go /app
COPY --from=go-builder /app/rss2go /usr/local/bin/rss2go
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["/usr/local/bin/rss2go", "daemon", "--config", "/app/rss2go.yaml"]
