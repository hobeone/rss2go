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

echo "=== [1/5] Installing Frontend Dependencies ==="
# Install exactly what bun.lock records so the build and tests below run
# against the locked versions. They are skipped if this step fails.
BUN_READY=0
if command -v bun &> /dev/null; then
  echo "Installing locked frontend dependencies with bun..."
  if (cd frontend && bun install --frozen-lockfile && bun prune); then
    BUN_READY=1
  else
    echo "ERROR: Frontend dependency install failed (is bun.lock out of date?)"
    FAILED=1
  fi
else
  echo "ERROR: bun is required but was not found in PATH."
  FAILED=1
fi

echo ""
echo "=== [2/5] Building Svelte Frontend ==="
if [ "${BUN_READY}" -eq 1 ]; then
  echo "Building frontend assets with bun..."
  if ! (cd frontend && bun run build); then
    echo "ERROR: Frontend build failed!"
    FAILED=1
  fi
else
  echo "Skipped: frontend dependencies are not installed."
fi

echo ""
echo "=== [3/5] Running Svelte UI Unit Tests ==="
if [ "${BUN_READY}" -eq 1 ]; then
  echo "Running frontend unit tests with bun/vitest..."
  if ! (cd frontend && bun run test); then
    echo "ERROR: Frontend unit tests failed!"
    FAILED=1
  fi
else
  echo "Skipped: frontend dependencies are not installed."
fi

echo ""
echo "=== [4/5] Running Go Unit & Integration Tests ==="
if ! go test -race -v ./...; then
  echo "ERROR: Go unit and integration tests failed!"
  FAILED=1
fi

echo ""
echo "=== [5/5] Running Go UI Playwright Tests ==="
if ! go test -tags=uitest -v ./test/uitest/...; then
  echo "ERROR: UI Playwright tests failed!"
  echo "TIP: errors from 'playwright.Run' or 'chromium.Launch' usually mean the"
  echo "     Playwright browser or its system libraries are not installed. Install"
  echo "     them, at the version pinned in go.mod, with:"
  echo "       go run github.com/mxschmitt/playwright-go/cmd/playwright install --with-deps chromium"
  echo "     (--with-deps installs system packages and asks for sudo.)"
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
