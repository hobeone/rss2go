FROM golang:1.26.2-alpine AS builder

WORKDIR /src

# git is required for automatic VCS stamping (ReadBuildInfo)
RUN apk add --no-cache git

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source (including .git) and build
COPY . .

# Ensure git works even if directory ownership differs
RUN git config --global --add safe.directory /src
RUN GOOS=linux go build -v -buildvcs=true -o /app/rss2go ./cmd/rss2go
RUN GOOS=linux go build -v -buildvcs=true -o /app/scraper ./cmd/scraper

FROM alpine:latest

# Certificates for HTTPS and timezone data
RUN apk add --no-cache ca-certificates tzdata su-exec

# Create a non-privileged user (will be adjusted by entrypoint)
RUN adduser -D rss2go

# Directory for configuration and database
RUN mkdir -p /app/config /app/db && \
    chown -R rss2go:rss2go /app

# Copy binaries
COPY --from=builder /app/rss2go /usr/local/bin/rss2go
COPY --from=builder /app/scraper /usr/local/bin/scraper

# Copy entrypoint script
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["/usr/local/bin/rss2go", "daemon", "--config", "/app/rss2go.yaml"]
