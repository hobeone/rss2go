#!/bin/sh
set -e

# Use provided PUID/PGID or default to 1000
PUID=${PUID:-1000}
PGID=${PGID:-1000}

echo "Starting with PUID: $PUID and PGID: $PGID"

# Adjust rss2go user/group to match host IDs
# We delete the existing user/group first to ensure we can set the IDs precisely
deluser rss2go 2>/dev/null || true
addgroup -g "$PGID" rss2go
adduser -u "$PUID" -G rss2go -D rss2go

# Ensure the app directory and especially the database file are owned by the user
# This handles the case where Docker creates the volume as root
chown -R rss2go:rss2go /app

# Run the command as the rss2go user
exec su-exec rss2go "$@"
