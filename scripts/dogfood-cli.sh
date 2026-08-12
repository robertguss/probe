#!/usr/bin/env bash
# dogfood-cli.sh — generate foundry-smoke-cli into a temp parent and exercise it
# (Section 52 / REQ-242 / REQ-247, bead go-foundry-cli-vu8).
#
# Complements integration/dogfood Go tests and does NOT replace j8h.2 e2e matrix.
#
# Usage (from repository root):
#   ./scripts/dogfood-cli.sh
#   FOUNDRY_DOGFOOD_ARTIFACT_DIR=docs/evidence/dogfood-cli-logs ./scripts/dogfood-cli.sh
#   ./scripts/dogfood-cli.sh --with-repo-map
#   ./scripts/dogfood-cli.sh --verify strict
#
# Strict verify runs go-staticcheck and go-govulncheck; govulncheck may require
# outbound network access to fetch the vulnerability database.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

STEP=0
step() {
  STEP=$((STEP + 1))
  local name="$1"
  shift
  printf 'step=%02d name=%s %s\n' "$STEP" "$name" "$*"
}

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

WITH_REPO_MAP=0
VERIFY=default
while [[ $# -gt 0 ]]; do
  case "$1" in
    --with-repo-map) WITH_REPO_MAP=1 ;;
    --verify)
      [[ $# -ge 2 ]] || fail "--verify requires an argument (default|strict)"
      VERIFY="$2"
      shift
      ;;
    --verify=*) VERIFY="${1#*=}" ;;
    -h|--help)
      sed -n '2,12p' "$0"
      exit 0
      ;;
    *) fail "unknown arg: $1" ;;
  esac
  shift
done

if [[ "$VERIFY" != "default" && "$VERIFY" != "strict" ]]; then
  fail "--verify must be 'default' or 'strict' (got $VERIFY)"
fi

# Prefer pinned go1.26.5 (catalog / Section 12). Nearby versions fail tool preflight.
# Probe with GOTOOLCHAIN=local so ambient auto-toolchain does not mask mise 1.26.4.
resolve_go() {
  local goos goarch ver_dir
  goos="$(uname -s | tr '[:upper:]' '[:lower:]')"
  goarch="$(uname -m)"
  case "$goarch" in
    x86_64) goarch=amd64 ;;
    aarch64|arm64) goarch=arm64 ;;
  esac
  ver_dir="${HOME}/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.5.${goos}-${goarch}/bin/go"
  local candidates=(
    "${FOUNDRY_GO_BIN:-}"
    "$ver_dir"
    "${HOME}/.local/share/mise/installs/go/1.26.5/bin/go"
    "/usr/local/go/bin/go"
    "$(command -v go || true)"
  )
  local c
  for c in "${candidates[@]}"; do
    [[ -z "$c" || ! -x "$c" ]] && continue
    if env -u GOTOOLCHAIN GOTOOLCHAIN=local GOENV=off "$c" version 2>/dev/null | grep -q 'go1\.26\.5'; then
      echo "$c"
      return 0
    fi
  done
  return 1
}

GO_BIN="$(resolve_go)" || fail "pinned go1.26.5 not found; set FOUNDRY_GO_BIN to absolute go1.26.5 (see dogfood/README.md) or install toolchain go1.26.5"
# Re-export so the Foundry child prefers the resolved pin (overrides a stale ambient FOUNDRY_GO_BIN).
export FOUNDRY_GO_BIN="$GO_BIN"
export GOROOT
GOROOT="$(cd "$(dirname "$GO_BIN")/.." && pwd)"
export PATH="$(dirname "$GO_BIN"):/usr/bin:/bin"
export GOTOOLCHAIN=local
export CGO_ENABLED=0

ARTIFACT_DIR="${FOUNDRY_DOGFOOD_ARTIFACT_DIR:-}"
if [[ -n "$ARTIFACT_DIR" ]]; then
  mkdir -p "$ARTIFACT_DIR"
  exec > >(tee -a "$ARTIFACT_DIR/dogfood-cli-run.txt") 2>&1
fi

step machine "go=$("$GO_BIN" version | tr -d '\n') goroot=$GOROOT os=$(uname -s) arch=$(uname -m)"
step git "git=$(git --version 2>/dev/null || echo missing)"
step host "hostname=$(hostname 2>/dev/null || echo unknown)"

step build_foundry
FOUNDRY_BIN="${FOUNDRY_BIN:-/tmp/foundry-dogfood-bin}"
"$GO_BIN" build -o "$FOUNDRY_BIN" ./cmd/foundry
step foundry_version "$("$FOUNDRY_BIN" version 2>&1 | tr '\n' ' ')"

# Custody-safe private parent (Section 31.3): sticky /tmp + 0700 child.
PARENT="$(mktemp -d /tmp/foundry-dogfood-XXXXXX)"
chmod 700 "$PARENT"
trap 'rm -rf "$PARENT"' EXIT
step parent "path=$PARENT mode=$(stat -c '%a' "$PARENT" 2>/dev/null || echo 700)"

SMOKE_SPEC="$ROOT/integration/fixtures/foundry-smoke-cli/foundry.toml"
SMOKE_DEST="$PARENT/foundry-smoke-cli"

step generate_smoke_cold "spec=$SMOKE_SPEC dest=$SMOKE_DEST"
START=$(date +%s%3N)
set +e
"$FOUNDRY_BIN" generate --spec "$SMOKE_SPEC" --dest "$SMOKE_DEST" --verify "$VERIFY"
GEN_EC=$?
set -e
END=$(date +%s%3N)
COLD_MS=$((END - START))
step generate_smoke_cold_done "exit=$GEN_EC elapsed_ms=$COLD_MS"
[[ "$GEN_EC" -eq 0 ]] || fail "smoke generate failed exit=$GEN_EC"
[[ -f "$SMOKE_DEST/go.mod" ]] || fail "missing go.mod after generate"

PLAN_SHA="$(grep -oE 'plan_sha256=[0-9a-f]+' <<<"$(tail -n 5 "$ARTIFACT_DIR/dogfood-cli-run.txt" 2>/dev/null || true)" | head -1 | cut -d= -f2 || true)"
# Re-run plan for identity if needed (dest already exists — use text from last lines via re-generate warm sibling).
step plan_sha256 "value=${PLAN_SHA:-see_generate_output}"

step smoke_go_test
(
  cd "$SMOKE_DEST"
  "$GO_BIN" test -count=1 ./...
)
step smoke_go_test_done "ok"

step smoke_build
(
  cd "$SMOKE_DEST"
  "$GO_BIN" build -o foundry-smoke-cli ./cmd/foundry-smoke-cli
  ./foundry-smoke-cli --help >/dev/null
  ./foundry-smoke-cli version
)
step smoke_exercise_done "help+version ok"

# Warm sibling generate (Section 49 seed).
WARM_PARENT="$(mktemp -d /tmp/foundry-dogfood-warm-XXXXXX)"
chmod 700 "$WARM_PARENT"
trap 'rm -rf "$PARENT" "$WARM_PARENT"' EXIT
step generate_smoke_warm "dest=$WARM_PARENT/foundry-smoke-cli"
START=$(date +%s%3N)
set +e
"$FOUNDRY_BIN" generate --spec "$SMOKE_SPEC" --dest "$WARM_PARENT/foundry-smoke-cli" --verify "$VERIFY"
GEN_EC=$?
set -e
END=$(date +%s%3N)
WARM_MS=$((END - START))
step generate_smoke_warm_done "exit=$GEN_EC elapsed_ms=$WARM_MS"
[[ "$GEN_EC" -eq 0 ]] || fail "warm smoke generate failed"

if [[ "$WITH_REPO_MAP" -eq 1 ]]; then
  REPO_SPEC="$ROOT/dogfood/repo-map/foundry.toml"
  REPO_DEST="$PARENT/repo-map"
  OVERLAY="$ROOT/dogfood/repo-map/testdata/overlay"
  step generate_repo_map "spec=$REPO_SPEC dest=$REPO_DEST"
  START=$(date +%s%3N)
  set +e
  "$FOUNDRY_BIN" generate --spec "$REPO_SPEC" --dest "$REPO_DEST" --verify "$VERIFY"
  GEN_EC=$?
  set -e
  END=$(date +%s%3N)
  REPO_MS=$((END - START))
  step generate_repo_map_done "exit=$GEN_EC elapsed_ms=$REPO_MS"
  [[ "$GEN_EC" -eq 0 ]] || fail "repo-map generate failed"

  step apply_inventory_overlay
  # Copy overlay files.
  (cd "$OVERLAY" && tar cf - .) | (cd "$REPO_DEST" && tar xf -)
  # One-line growth wire (same edit owners apply by hand).
  ROOT_GO="$REPO_DEST/internal/cli/root.go"
  if grep -q 'newInventoryCmd()' "$ROOT_GO"; then
    step wire_inventory "already_wired"
  else
    # Portable sed: insert after newCompletionCmd(),
    python3 - <<'PY' "$ROOT_GO"
import sys
path = sys.argv[1]
text = open(path).read()
old = "newVersionCmd(),\n\t\tnewCompletionCmd(),"
new = "newVersionCmd(),\n\t\tnewCompletionCmd(),\n\t\tnewInventoryCmd(),"
if old not in text:
    raise SystemExit("could not wire newInventoryCmd")
open(path, "w").write(text.replace(old, new, 1))
PY
    step wire_inventory "ok"
  fi

  step repo_map_go_test
  (
    cd "$REPO_DEST"
    "$GO_BIN" test -count=1 ./...
    "$GO_BIN" build -o repo-map ./cmd/repo-map
    mkdir -p _sample && echo hi >_sample/a.txt
    ./repo-map inventory _sample --output text | grep -q a.txt
    ./repo-map inventory _sample --output json | grep -q a.txt
  )
  step repo_map_exercise_done "inventory text+json ok"
fi

step summary "cold_ms=$COLD_MS warm_ms=$WARM_MS with_repo_map=$WITH_REPO_MAP"
step done "smoke-cli dogfood green"

if [[ -n "$ARTIFACT_DIR" ]]; then
  {
    echo "cold_generate_ms=$COLD_MS"
    echo "warm_generate_ms=$WARM_MS"
    echo "go=$("$GO_BIN" version | tr -d '\n')"
    echo "os=$(uname -s) arch=$(uname -m)"
  } >"$ARTIFACT_DIR/timings.env"
fi
