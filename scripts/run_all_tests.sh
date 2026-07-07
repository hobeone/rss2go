#!/usr/bin/env bash
#
# run_all_tests.sh
# Runs the full suite of Go unit tests, integration tests, and UI tests.
#

set -uo pipefail

# Get repository root directory
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_ROOT}"

FAILED=0

echo "=== [1/3] Building Svelte Frontend ==="
if command -v bun &> /dev/null; then
  echo "Building frontend assets with bun..."
  if ! (cd frontend && bun run build); then
    echo "ERROR: Frontend build failed!"
    FAILED=1
  fi
else
  echo "ERROR: bun is required for the build but was not found in PATH."
  FAILED=1
fi

echo ""
echo "=== [2/3] Running Go Unit & Integration Tests ==="
if ! go test -race -v ./...; then
  echo "ERROR: Go unit and integration tests failed!"
  FAILED=1
fi

echo ""
echo "=== [3/3] Running Go UI Playwright Tests ==="
if ! go test -tags=uitest -v ./test/uitest/...; then
  echo "ERROR: UI Playwright tests failed!"
  FAILED=1
fi

echo ""
if [ "${FAILED}" -ne 0 ]; then
  echo "❌ Some test suites failed. Review the errors above."
  exit 1
else
  echo "✅ All test suites completed successfully!"
  exit 0
fi
