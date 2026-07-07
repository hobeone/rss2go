FROM golang:1.26.4-alpine@sha256:3ad57304ad93bbec8548a0437ad9e06a455660655d9af011d58b993f6f615648 AS builder

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

FROM alpine:3.23@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40

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
