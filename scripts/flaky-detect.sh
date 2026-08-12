#!/usr/bin/env bash
# Flaky-test detector: run slower suites with -count=5 and parse go-test JSON
# to flag tests that fail intermittently or have high duration variance.
# Exit code is 1 when any test is flagged. CI runs this non-blocking so it
# reports without blocking merges while the suite is being stabilized.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

ARTIFACT_DIR="${FOUNDRY_FLAKY_ARTIFACT_DIR:-$(mktemp -d /tmp/foundry-flaky-XXXXXX)}"
mkdir -p "$ARTIFACT_DIR"

echo "artifact_dir=$ARTIFACT_DIR"

# Suite label -> go test invocation. Keep invocations finite (avoiding unbounded
# network or hostile mounts); the focus is the unit, integration/generate, and
# cmd/foundry testscript surfaces.
declare -a SUITES=(
  "unit:go test -count=5 -json ./..."
  "generate:go test -count=5 -json ./integration/generate/"
  "testscripts:go test -count=5 -json ./cmd/foundry -run 'TestWriteFree|TestGenerateE2E'"
)

JSON_ARGS=()
for entry in "${SUITES[@]}"; do
  label="${entry%%:*}"
  cmd="${entry#*:}"
  out="$ARTIFACT_DIR/${label}.json"
  start_ms=$(date +%s%3N)
  echo "=== $label start ==="
  # shellcheck disable=SC2086
  eval "$cmd" > >(tee "$out") 2> >(tee "$ARTIFACT_DIR/${label}.stderr" >&2)
  rc=$?
  stop_ms=$(date +%s%3N)
  echo "=== $label done rc=$rc elapsed_ms=$((stop_ms - start_ms)) ==="
  echo "{\"label\":\"$label\",\"exit_code\":$rc,\"elapsed_ms\":$((stop_ms - start_ms))}" > "$ARTIFACT_DIR/${label}.meta.json"
  JSON_ARGS+=("$label:$out")
done

report_json="$ARTIFACT_DIR/flaky-report.json"
report_txt="$ARTIFACT_DIR/flaky-report.txt"
FOUNDRY_FLAKY_REPORT_JSON="$report_json" python3 "$ROOT/scripts/flaky-report.py" "${JSON_ARGS[@]}" > "$report_txt"
rc=$?

cat "$report_txt"

echo "report_json=$report_json"
echo "report_txt=$report_txt"

exit "$rc"
