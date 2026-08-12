#!/usr/bin/env bash
# E3 live macOS / Darwin race evidence harness (go-foundry-cli-o60.1).
#
# Run on a macOS host with Xcode Command Line Tools (clang). Captures machine
# metadata, runs the full E3 preflight + known-race matrix, and writes logs
# under docs/evidence/e3-logs/darwin/.
#
# Usage (from repo root or any cwd):
#   ./integration/hostile/e3/scripts/e3-darwin-race.sh [go-test-args...]
#
# Optional env:
#   E3_LOG_DIR   override log directory (default: docs/evidence/e3-logs/darwin)
#   GOTOOLCHAIN  default go1.26.5
#
# Expected outcomes on a well-equipped Darwin host:
#   - Preflight prefers clang (then gcc, then cc) and probe-compiles a .c file
#   - FOUNDRY_KNOWN_RACE=1 go test -race ./…/knownrace/ reports DATA RACE
#   - Full package suite exits 0 (known-race failure is asserted inside the parent test)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
LOG_DIR="${E3_LOG_DIR:-$ROOT/docs/evidence/e3-logs/darwin}"
GOTOOLCHAIN="${GOTOOLCHAIN:-go1.26.5}"
export GOTOOLCHAIN

mkdir -p "$LOG_DIR"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "error: this harness must run on macOS/Darwin (got $(uname -s))" >&2
  echo "On Linux the E3 matrix is already recorded under docs/evidence/e3-logs/." >&2
  echo "Cross-compile only: GOOS=darwin GOARCH=arm64 go test -c ./integration/hostile/e3/" >&2
  exit 2
fi

echo "E3 Darwin race evidence host: $(hostname)"
echo "uname: $(uname -a)"
echo "go: $(go version 2>/dev/null || true)"

# Resolve clang/gcc for the machine snapshot (preflight does the real probe).
CC_PATH=""
CC_NAME=""
for c in clang gcc cc; do
  if p="$(command -v "$c" 2>/dev/null)"; then
    CC_PATH="$p"
    CC_NAME="$c"
    break
  fi
done

{
  echo "date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "hostname: $(hostname)"
  echo "uname: $(uname -a)"
  if command -v sw_vers >/dev/null 2>&1; then
    echo "sw_vers:"
    sw_vers | sed 's/^/  /'
  fi
  echo "go: $(go version)"
  echo "GOTOOLCHAIN=$GOTOOLCHAIN"
  echo "GOOS/GOARCH: $(go env GOOS)/$(go env GOARCH)"
  echo "default CGO_ENABLED: $(go env CGO_ENABLED)"
  echo "default CC: $(go env CC)"
  echo "preflight_compiler_order: clang gcc cc (darwin)"
  if [[ -n "$CC_PATH" ]]; then
    echo "first_cc_on_path: $CC_NAME -> $CC_PATH"
    echo "cc_version:"
    ("$CC_PATH" --version 2>&1 || "$CC_PATH" -v 2>&1 || true) | head -3 | sed 's/^/  /'
  else
    echo "first_cc_on_path: (none — race preflight will skip with notice)"
  fi
  if command -v xcode-select >/dev/null 2>&1; then
    echo "xcode-select -p: $(xcode-select -p 2>/dev/null || echo unavailable)"
  fi
  echo "which clang gcc cc:"
  command -v clang gcc cc 2>/dev/null | sed 's/^/  /' || true
  echo "FOUNDRY_KNOWN_RACE note: parent e3 tests arm knownrace with FOUNDRY_KNOWN_RACE=1"
} | tee "$LOG_DIR/machine.txt"

echo "running E3 package tests on Darwin..."
set +e
(
  cd "$ROOT"
  go test ./integration/hostile/e3/ -count=1 -v -timeout 180s "$@"
) 2>&1 | tee "$LOG_DIR/go-test.txt"
TEST_EXIT=${PIPESTATUS[0]}
set -e

# Standalone known-race proof (must FAIL with DATA RACE when CLT present).
echo "running knownrace fixture under -race (expect DATA RACE / non-zero)..."
set +e
(
  cd "$ROOT"
  CGO_ENABLED=1 FOUNDRY_KNOWN_RACE=1 \
    go test -race -count=1 -timeout 60s ./integration/hostile/e3/knownrace/
) 2>&1 | tee "$LOG_DIR/knownrace-race.txt"
KNOWN_EXIT=${PIPESTATUS[0]}
set -e

{
  echo "date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "e3_package_test_exit=$TEST_EXIT"
  echo "knownrace_race_exit=$KNOWN_EXIT"
  echo "preflight_lines:"
  grep -E 'E3PROBE.*preflight|PATH lookup clang|PATH lookup gcc|compiler preflight' \
    "$LOG_DIR/go-test.txt" | head -40 | sed 's/^/  /' || true
  echo "data_race_lines:"
  grep -E 'DATA RACE|WARNING: DATA RACE|expect_data_race|known_race' \
    "$LOG_DIR/go-test.txt" "$LOG_DIR/knownrace-race.txt" 2>/dev/null | head -40 | sed 's/^/  /' || true
  echo "pass_fail:"
  grep -E '--- (PASS|FAIL)|PASS:|FAIL:|ok  |FAIL\t|E3 matrix complete' \
    "$LOG_DIR/go-test.txt" | sed 's/^/  /' || true
  # Acceptance checks for the evidence doc.
  HAS_PREFLIGHT=0
  HAS_DATA_RACE=0
  if grep -qE 'compiler preflight ok|PATH lookup clang|PATH lookup gcc' "$LOG_DIR/go-test.txt"; then
    HAS_PREFLIGHT=1
  fi
  if grep -q 'DATA RACE' "$LOG_DIR/go-test.txt" "$LOG_DIR/knownrace-race.txt" 2>/dev/null; then
    HAS_DATA_RACE=1
  fi
  echo "acceptance_preflight_ok=$HAS_PREFLIGHT"
  echo "acceptance_data_race_seen=$HAS_DATA_RACE"
  if [[ "$TEST_EXIT" -eq 0 && "$HAS_PREFLIGHT" -eq 1 && "$HAS_DATA_RACE" -eq 1 && "$KNOWN_EXIT" -ne 0 ]]; then
    echo "acceptance=PASS"
  else
    echo "acceptance=FAIL"
  fi
} | tee "$LOG_DIR/summary.txt"

echo "test_exit=$TEST_EXIT knownrace_exit=$KNOWN_EXIT"
echo "logs: $LOG_DIR/machine.txt $LOG_DIR/go-test.txt $LOG_DIR/knownrace-race.txt $LOG_DIR/summary.txt"

# Exit non-zero if package suite failed or acceptance markers missing.
if [[ "$TEST_EXIT" -ne 0 ]]; then
  exit "$TEST_EXIT"
fi
if ! grep -q 'acceptance=PASS' "$LOG_DIR/summary.txt"; then
  echo "error: Darwin E3 acceptance markers not satisfied (preflight + DATA RACE)" >&2
  exit 1
fi
exit 0
