#!/usr/bin/env bash
# E1 live APFS / Darwin evidence harness (go-foundry-cli-962).
#
# Run on a macOS host with APFS (typical boot volume). Captures machine
# metadata, confirms exclusiveRename uses RenameatxNp(RENAME_EXCL|RENAME_NOFOLLOW_ANY),
# runs the full E1 probe suite, and writes logs under docs/evidence/e1-logs/.
#
# Usage (from repo root or any cwd):
#   ./integration/hostile/e1/scripts/e1-darwin-apfs.sh [go-test-args...]
#
# Optional env:
#   E1_LOG_DIR   override log directory (default: docs/evidence/e1-logs)
#   E1_FS_ROOTS  override matrix roots (default: auto-detect apfs under private dir)
#   GOTOOLCHAIN  default go1.26.5
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
LOG_DIR="${E1_LOG_DIR:-$ROOT/docs/evidence/e1-logs}"
GOTOOLCHAIN="${GOTOOLCHAIN:-go1.26.5}"
export GOTOOLCHAIN

mkdir -p "$LOG_DIR"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "error: this harness must run on macOS/Darwin (got $(uname -s))" >&2
  echo "On Linux use scripts/e1-mount-matrix.sh for the ext4/xfs/btrfs matrix." >&2
  exit 2
fi

echo "E1 Darwin/APFS evidence host: $(hostname)"
echo "uname: $(uname -a)"
echo "go: $($GOTOOLCHAIN go version 2>/dev/null || go version)"

# Private 0700 work dir on the default volume (APFS for standard Macs).
# Resolve through /private/... so the O_NOFOLLOW parent walk never hits the
# /tmp and /var directory-symlink aliases (Darwin: /tmp → /private/tmp).
if [[ -n "${E1_WORK_DIR:-}" ]]; then
  WORK="$E1_WORK_DIR"
  mkdir -p "$WORK"
else
  WORK="$(mktemp -d -t e1-apfs)"
fi
WORK="$(cd "$WORK" && /bin/pwd -P)"
# Ensure ownership chain is private for custody checks.
chmod 0700 "$WORK" 2>/dev/null || true
cleanup() {
  set +e
  # Harness scratch only — never a Foundry stage delete API.
  rm -rf "$WORK"
}
trap cleanup EXIT

# Detect FS type via df/mount (cross-check of Go Statfs label).
FS_LABEL="apfs"
if command -v df >/dev/null 2>&1; then
  # macOS df: last line, first field is device; match in mount table.
  DEV="$(df -P "$WORK" 2>/dev/null | tail -1 | awk '{print $1}')"
  if [[ -n "$DEV" ]]; then
    MLINE="$(mount | grep -F "$DEV" | head -1 || true)"
    if [[ "$MLINE" == *"(apfs"* ]] || [[ "$MLINE" == *"apfs,"* ]]; then
      FS_LABEL="apfs"
    elif [[ -n "$MLINE" ]]; then
      # Extract type inside first parentheses token.
      FS_LABEL="$(printf '%s' "$MLINE" | sed -n 's/.*(\([^,)]*\).*/\1/p' | tr '[:upper:]' '[:lower:]')"
      FS_LABEL="${FS_LABEL:-unknown}"
    fi
  fi
fi

export E1_FS_ROOTS="${E1_FS_ROOTS:-${FS_LABEL}:${WORK}}"
echo "E1_FS_ROOTS=$E1_FS_ROOTS"

# Machine / OS / APFS snapshot
{
  echo "date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "hostname: $(hostname)"
  echo "uname: $(uname -a)"
  if command -v sw_vers >/dev/null 2>&1; then
    echo "sw_vers:"
    sw_vers | sed 's/^/  /'
  fi
  echo "go: $(go version)"
  echo "x/sys: $(cd "$ROOT" && go list -m golang.org/x/sys)"
  echo "E1_FS_ROOTS=$E1_FS_ROOTS"
  echo "work: $WORK"
  echo "df:"
  df -h "$WORK" | sed 's/^/  /'
  echo "mount:"
  mount | grep -E ' on / |apfs' | head -20 | sed 's/^/  /' || true
  if command -v diskutil >/dev/null 2>&1; then
    echo "diskutil info / (filesystem):"
    diskutil info / 2>/dev/null | grep -E 'File System|Type \(Bundle\)|Volume Name|Mounted|APFS' | sed 's/^/  /' || true
  fi
  echo "rename_adapter: commit_darwin.go exclusiveRename -> RenameatxNp(RENAME_EXCL|RENAME_NOFOLLOW_ANY)"
} | tee "$LOG_DIR/darwin-apfs-machine.txt"

echo "running E1 tests on Darwin/APFS..."
set +e
(
  cd "$ROOT"
  go test ./integration/hostile/e1/ -count=1 -v -timeout 180s "$@"
) 2>&1 | tee "$LOG_DIR/darwin-apfs-go-test.txt"
TEST_EXIT=${PIPESTATUS[0]}
set -e

# Extract pass/fail summary lines for the evidence doc.
{
  echo "date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "test_exit=$TEST_EXIT"
  echo "rename_syscall_lines:"
  grep -E 'RenameatxNp|renameatx_np|syscall=Rename' "$LOG_DIR/darwin-apfs-go-test.txt" | head -40 | sed 's/^/  /' || true
  echo "probe_outcomes:"
  # BSD grep treats leading --- as options unless -- is used.
  grep -E -- '--- (PASS|FAIL)' "$LOG_DIR/darwin-apfs-go-test.txt" | sed 's/^/  /' || true
  grep -E -- 'PASS:|FAIL:' "$LOG_DIR/darwin-apfs-go-test.txt" | sed 's/^/  /' || true
} | tee "$LOG_DIR/darwin-apfs-summary.txt"

echo "test_exit=$TEST_EXIT"
echo "logs: $LOG_DIR/darwin-apfs-machine.txt $LOG_DIR/darwin-apfs-go-test.txt $LOG_DIR/darwin-apfs-summary.txt"
exit "$TEST_EXIT"
