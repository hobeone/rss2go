#!/usr/bin/env bash
#
# update-deps.sh
# Updates Go and bun dependencies, then verifies the result.
#
# Usage: scripts/update-deps.sh [--latest] [--skip-verify] [--commit] [--yes]
#   --latest       also cross major versions for bun (rewrites package.json);
#                  packages in BUN_LATEST_EXCLUDES are skipped
#   --skip-verify  skip the build/test checks after updating
#   --commit       commit the dependency files with a summary of what changed;
#                  shows the message and asks for confirmation first
#   --yes          with --commit, skip the confirmation prompt (also skipped
#                  when stdin is not a terminal)
#

set -euo pipefail

DEP_FILES=(go.mod go.sum frontend/package.json frontend/bun.lock)
DIFF_FILES=(go.mod frontend/package.json frontend/bun.lock)
MAX_LINES=40

# Packages that --latest must not bump, as bun negation patterns.
# typescript: svelte-check only supports TypeScript 5 and 6 (7 is the native
# port without the JS compiler API). Drop this once svelte-check supports 7.
BUN_LATEST_EXCLUDES=('!typescript')

# run <cmd...>: prints the command, then runs it.
run() {
  printf '+ %s\n' "$*"
  "$@"
}

# run_in <dir> <cmd...>: like run, but executes the command inside <dir>.
run_in() {
  local dir="$1"
  shift
  printf '+ (cd %s && %s)\n' "${dir}" "$*"
  (cd "${dir}" && "$@")
}

# summarize_changes <mode> <file>
# Reads the uncommitted diff of <file> and prints one line per dependency that
# was bumped, added or removed. <mode> is "go" (go.mod) or "bun" (bun.lock).
summarize_changes() {
  local mode="$1" file="$2"
  git diff -U0 -- "${file}" | awk -v mode="${mode}" '
    function parse(line,    f, m) {
      name = ""; ver = ""
      if (mode == "go") {
        split(line, f, /[ \t]+/)
        # f[1] is empty (leading tab), f[2]=module, f[3]=version
        if (f[2] != "" && f[3] ~ /^v[0-9]/) { name = f[2]; ver = f[3] }
      } else if (match(line, /"[^"]+": \["[^"]+@[^"]+"/)) {
        m = substr(line, RSTART, RLENGTH)
        sub(/^"/, "", m); name = m; sub(/".*/, "", name)
        ver = m; sub(/.*@/, "", ver); sub(/"$/, "", ver)
      }
    }
    /^---|^\+\+\+/ { next }
    /^-/ { parse(substr($0, 2)); if (name != "") old[name] = ver }
    /^\+/ { parse(substr($0, 2)); if (name != "") new[name] = ver }
    END {
      for (n in new) {
        if (!(n in old)) printf "- %s %s (added)\n", n, new[n]
        else if (old[n] != new[n]) printf "- %s %s -> %s\n", n, old[n], new[n]
      }
      for (n in old) if (!(n in new)) printf "- %s %s (removed)\n", n, old[n]
    }' | sort
}

# section <title> <summary>: prints a titled, length-capped list (or nothing).
section() {
  local title="$1" summary="$2" total
  [[ -z "${summary}" ]] && return 0
  total="$(wc -l <<<"${summary}")"
  printf '%s (%d):\n' "${title}" "${total}"
  head -n "${MAX_LINES}" <<<"${summary}"
  if ((total > MAX_LINES)); then
    printf -- '- ... and %d more\n' "$((total - MAX_LINES))"
  fi
  printf '\n'
}

# build_commit_message <verified: 0|1>
build_commit_message() {
  local verified="$1" go_summary bun_summary
  go_summary="$(summarize_changes go go.mod)"
  bun_summary="$(summarize_changes bun frontend/bun.lock)"

  printf 'chore(deps): update go and bun dependencies\n\n'
  section "Go modules" "${go_summary}"
  section "Bun packages" "${bun_summary}"
  if [[ "${verified}" -eq 1 ]]; then
    printf 'Verified with go build, go test, bun run check, bun run build and bun run test.\n'
  else
    printf 'Not verified: update ran with --skip-verify.\n'
  fi
}

main() {
  local latest=0 verify=1 commit=0 yes=0 arg
  for arg in "$@"; do
    case "${arg}" in
      --latest) latest=1 ;;
      --skip-verify) verify=0 ;;
      --commit) commit=1 ;;
      --yes) yes=1 ;;
      -h|--help) sed -n '2,$ { /^#/!q; s/^# \{0,1\}//; p }' "${BASH_SOURCE[0]}"; exit 0 ;;
      *) echo "Unknown argument: ${arg}" >&2; exit 2 ;;
    esac
  done

  cd "$(dirname "${BASH_SOURCE[0]}")/.."

  echo "=== [1/4] Updating Go modules ==="
  run go get -u -v -t ./...
  run go mod tidy

  echo "=== [2/4] Updating bun packages ==="
  if [[ "${latest}" -eq 1 ]]; then
    run_in frontend bun update --latest "${BUN_LATEST_EXCLUDES[@]}"
  else
    run_in frontend bun update
  fi
  run_in frontend bun prune

  if [[ "${verify}" -eq 1 ]]; then
    echo "=== [3/4] Verifying Go ==="
    run go build ./...
    run go test ./...

    echo "=== [4/4] Verifying frontend ==="
    # Install exactly what bun.lock records, so the checks below run against
    # the locked versions; it also fails if bun.lock and package.json disagree.
    run_in frontend bun install --frozen-lockfile
    run_in frontend bun prune
    run_in frontend bun run check
    run_in frontend bun run build
    run_in frontend bun run test
  else
    echo "=== Skipping verification (--skip-verify) ==="
  fi

  if [[ "${commit}" -eq 1 ]]; then
    echo "=== Committing ==="
    if git diff --quiet -- "${DEP_FILES[@]}"; then
      echo "No dependency changes to commit."
    else
      local msg reply
      msg="$(mktemp)"
      # Expand now: `msg` is local and out of scope when the EXIT trap fires.
      # shellcheck disable=SC2064
      trap "rm -f '${msg}'" EXIT
      build_commit_message "${verify}" >"${msg}"

      echo
      git --no-pager diff --stat -- "${DEP_FILES[@]}"
      echo
      echo "Dependency diff (go.sum omitted; it only holds checksums):"
      git --no-pager diff -U0 -- "${DIFF_FILES[@]}"
      echo
      echo "Commit message:"
      echo "----------------------------------------"
      cat "${msg}"
      echo "----------------------------------------"

      if [[ "${yes}" -eq 0 && -t 0 ]]; then
        read -r -p "Commit these changes? [y/N] " reply
        if [[ ! "${reply}" =~ ^[Yy]$ ]]; then
          echo "Not committed. Changes are left in the working tree."
          echo "Review with: git diff -- ${DEP_FILES[*]}"
          exit 0
        fi
      fi

      # Only the dependency files are committed; other staged or modified
      # files are left untouched.
      run git add -- "${DEP_FILES[@]}"
      run git commit -F "${msg}" -- "${DEP_FILES[@]}"
    fi
  else
    echo
    echo "Done. Review changes with: git diff --stat ${DEP_FILES[*]}"
  fi
}

# Run only when executed directly, so the helpers can be sourced for testing.
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
