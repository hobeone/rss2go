FROM golang:1.26-alpine AS builder

WORKDIR /src

# Basic build requirements for CGO (if needed, though modernc sqlite doesn't)
RUN apk add --no-cache build-base

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN GOOS=linux go build -v -o /app/rss2go cmd/rss2go/main.go
RUN GOOS=linux go build -v -o /app/scraper cmd/scraper/main.go

FROM alpine:latest

# Certificates for HTTPS and timezone data
RUN apk add --no-cache ca-certificates tzdata

# Create a non-privileged user
RUN adduser -D rss2go

# Directory for configuration and database
RUN mkdir -p /app/config /app/db && \
    chown -R rss2go:rss2go /app/config /app/db

USER rss2go

# Copy binaries
COPY --from=builder /app/rss2go /usr/local/bin/rss2go
COPY --from=builder /app/scraper /usr/local/bin/scraper

# Default volumes
VOLUME ["/app/config", "/app/db"]

# Entrypoint will be overridden by docker-compose for the scraper
ENTRYPOINT ["/usr/local/bin/rss2go"]
