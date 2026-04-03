#!/bin/bash
set -euo pipefail

usage() {
    cat <<EOF
Usage: $(basename "$0") [-d] [-h] [source.db] [dest.db]

Live-backup a SQLite database using sqlite3's online backup API.

Arguments:
  source.db   Source database (default: rss2go.db)
  dest.db     Destination file (default: <source>.backup, or <source>.backup.<date> with -d)

Options:
  -d   Append today's date to the default destination filename (YYYY-MM-DD)
  -h   Show this help text
EOF
}

DATESTAMP=false

while getopts ":dh" opt; do
    case "$opt" in
        d) DATESTAMP=true ;;
        h) usage; exit 0 ;;
        *) echo "Error: unknown option -${OPTARG}" >&2; usage >&2; exit 1 ;;
    esac
done
shift $((OPTIND - 1))

SOURCE="${1:-rss2go.db}"

if [[ -n "${2:-}" ]]; then
    DEST="$2"
elif [[ "$DATESTAMP" == true ]]; then
    DEST="${SOURCE}.backup.$(date +%Y-%m-%d)"
else
    DEST="${SOURCE}.backup"
fi

if ! command -v sqlite3 &>/dev/null; then
    echo "Error: sqlite3 not found in PATH" >&2
    exit 1
fi

if [[ ! -f "$SOURCE" ]]; then
    echo "Error: source database not found: $SOURCE" >&2
    exit 1
fi

sqlite3 "$SOURCE" ".timeout 10000" ".backup '${DEST}'"
echo "Backed up $SOURCE → $DEST"
