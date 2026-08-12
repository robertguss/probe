#!/usr/bin/env bash
# capture-baselines.sh — Section 49 / REQ-165 performance baseline harness
# (bead go-foundry-cli-j8h.1).
#
# Measures:
#   1. Write-free commands (version, catalog list, validate, plan) N times → p50/p95
#   2. generate foundry-smoke-cli warm (host module cache) and cold (empty
#      GOMODCACHE+GOCACHE) with per-stage wall timings derived from Foundry
#      progress event boundaries
#
# Outputs machine-readable JSON + human summary under docs/evidence/perf/ (or
# FOUNDRY_PERF_OUT). Does NOT invent absolute pass/fail gates.
#
# Usage (from repository root):
#   ./scripts/perf/capture-baselines.sh
#   FOUNDRY_PERF_OUT=/tmp/perf-out WRITE_FREE_N=10 ./scripts/perf/capture-baselines.sh
#   ./scripts/perf/capture-baselines.sh --quick   # fewer iterations (dev smoke)
#   ./scripts/perf/capture-baselines.sh --skip-cold  # skip cleared-cache generate
#
# Env:
#   FOUNDRY_BIN, FOUNDRY_GO_BIN, FOUNDRY_PERF_OUT
#   WRITE_FREE_N (default 20), WARM_N (default 3), COLD_N (default 1)
#   MACHINE_CLASS override (dev|ci-github|ci|…)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

QUICK=0
SKIP_COLD=0
for arg in "$@"; do
  case "$arg" in
    --quick) QUICK=1 ;;
    --skip-cold) SKIP_COLD=1 ;;
    -h|--help)
      sed -n '2,22p' "$0"
      exit 0
      ;;
    *)
      printf 'unknown arg: %s\n' "$arg" >&2
      exit 2
      ;;
  esac
done

WRITE_FREE_N="${WRITE_FREE_N:-20}"
WARM_N="${WARM_N:-3}"
COLD_N="${COLD_N:-1}"
if [[ "$QUICK" -eq 1 ]]; then
  WRITE_FREE_N=5
  WARM_N=1
  COLD_N=1
fi

OUT_DIR="${FOUNDRY_PERF_OUT:-$ROOT/docs/evidence/perf}"
mkdir -p "$OUT_DIR"
RUN_DIR="$(mktemp -d /tmp/foundry-perf-run-XXXXXX)"
# Module cache files are often mode 0444; chmod before rm (Go GOMODCACHE).
cleanup_run_dir() {
  if [[ -n "${RUN_DIR:-}" && -d "$RUN_DIR" ]]; then
    chmod -R u+w "$RUN_DIR" 2>/dev/null || true
    rm -rf "$RUN_DIR"
  fi
}
trap cleanup_run_dir EXIT

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

log() {
  printf '[perf] %s\n' "$*" >&2
}

# Prefer pinned go1.26.5 (catalog / Section 12). Probe with GOTOOLCHAIN=local.
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

GO_BIN="$(resolve_go)" || fail "pinned go1.26.5 not found; set FOUNDRY_GO_BIN (see dogfood/README.md)"
# Re-export so the Foundry child prefers the resolved pin (overrides a stale ambient FOUNDRY_GO_BIN).
export FOUNDRY_GO_BIN="$GO_BIN"
export GOROOT
GOROOT="$(cd "$(dirname "$GO_BIN")/.." && pwd)"
export PATH="$(dirname "$GO_BIN"):/usr/bin:/bin"
export GOTOOLCHAIN=local
export CGO_ENABLED=0

ms_now() {
  if date +%s%3N 2>/dev/null | grep -Eq '^[0-9]+$'; then
    date +%s%3N
  else
    python3 -c 'import time; print(int(time.time()*1000))'
  fi
}

iso_utc() {
  date -u +%Y-%m-%dT%H:%M:%SZ
}

detect_machine_class() {
  if [[ -n "${MACHINE_CLASS:-}" ]]; then
    echo "$MACHINE_CLASS"
    return
  fi
  if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
    echo "ci-github"
  elif [[ "${CI:-}" == "true" ]]; then
    echo "ci"
  else
    echo "dev"
  fi
}

log "building foundry"
FOUNDRY_BIN="${FOUNDRY_BIN:-$RUN_DIR/foundry}"
if [[ ! -x "$FOUNDRY_BIN" ]]; then
  "$GO_BIN" build -o "$FOUNDRY_BIN" ./cmd/foundry
fi

GO_VER="$("$GO_BIN" version | tr -d '\n')"
GIT_VER="$(git --version 2>/dev/null | awk '{print $3}' || echo unknown)"
HOST_NAME="$(hostname 2>/dev/null || echo unknown)"
OS_NAME="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH_NAME="$(uname -m)"
case "$ARCH_NAME" in
  x86_64) ARCH_NAME=amd64 ;;
  aarch64) ARCH_NAME=arm64 ;;
esac
KERNEL="$(uname -r 2>/dev/null || echo unknown)"
MACHINE_CLASS="$(detect_machine_class)"
FOUNDRY_VER="$("$FOUNDRY_BIN" version 2>&1 | head -1 | tr -d '\n' || true)"
CAPTURED_AT="$(iso_utc)"
SMOKE_SPEC="$ROOT/integration/fixtures/foundry-smoke-cli/foundry.toml"
[[ -f "$SMOKE_SPEC" ]] || fail "missing smoke spec: $SMOKE_SPEC"

log "host machine_class=$MACHINE_CLASS go=$GO_VER os=$OS_NAME/$ARCH_NAME"
log "iterations write_free=$WRITE_FREE_N warm=$WARM_N cold=$COLD_N skip_cold=$SKIP_COLD"

SAMPLES_DIR="$RUN_DIR/samples"
mkdir -p "$SAMPLES_DIR"

# --- Write-free samples -------------------------------------------------------

run_write_free_once() {
  local start end
  start="$(ms_now)"
  set +e
  "$@" >/dev/null 2>&1
  local ec=$?
  set -e
  end="$(ms_now)"
  printf '%s\n' "$((end - start))"
  return "$ec"
}

measure_write_free() {
  local name="$1"
  shift
  local out="$SAMPLES_DIR/wf_${name}.ms"
  : >"$out"
  local i ec=0
  for ((i = 1; i <= WRITE_FREE_N; i++)); do
    set +e
    local ms
    ms="$(run_write_free_once "$@")"
    local once_ec=$?
    set -e
    printf '%s\n' "$ms" >>"$out"
    if [[ "$once_ec" -ne 0 ]]; then
      ec=$once_ec
      log "warn: write-free $name iteration $i exit=$once_ec ms=$ms"
    fi
  done
  return "$ec"
}

log "write-free: version ×$WRITE_FREE_N"
measure_write_free version "$FOUNDRY_BIN" version || true
log "write-free: catalog list ×$WRITE_FREE_N"
measure_write_free catalog_list "$FOUNDRY_BIN" catalog list || true
log "write-free: validate ×$WRITE_FREE_N"
measure_write_free validate "$FOUNDRY_BIN" validate --spec "$SMOKE_SPEC" || true
log "write-free: plan ×$WRITE_FREE_N"
measure_write_free plan "$FOUNDRY_BIN" plan --spec "$SMOKE_SPEC" || true

# --- Generate with stage boundary timing --------------------------------------
# Python drives the subprocess so each stdout line is timestamped as it arrives.
# Per-stage ms = wall delta between consecutive progress: lines (Section 29.2).

run_generate_sample() {
  local label="$1"
  local cache_gomod="$2"   # host_warm | cleared
  local cache_go="$3"      # host_warm | cleared
  local sample_id="$4"
  local parent dest out_raw out_ts out_meta out_err
  parent="$(mktemp -d "$RUN_DIR/parent-XXXXXX")"
  chmod 700 "$parent"
  dest="$parent/foundry-smoke-cli"
  out_raw="$SAMPLES_DIR/${sample_id}.raw"
  out_ts="$SAMPLES_DIR/${sample_id}.ts"
  out_meta="$SAMPLES_DIR/${sample_id}.meta"
  out_err="$SAMPLES_DIR/${sample_id}.err"
  : >"$out_raw"
  : >"$out_ts"
  : >"$out_err"

  local gomodcache_path=""
  local gocache_path=""
  if [[ "$cache_gomod" == "cleared" || "$cache_go" == "cleared" ]]; then
    gomodcache_path="$RUN_DIR/empty-gomodcache-${sample_id}"
    gocache_path="$RUN_DIR/empty-gocache-${sample_id}"
    mkdir -p "$gomodcache_path" "$gocache_path"
    find "$gomodcache_path" -mindepth 1 -maxdepth 1 -exec rm -rf {} + 2>/dev/null || true
    find "$gocache_path" -mindepth 1 -maxdepth 1 -exec rm -rf {} + 2>/dev/null || true
  fi

  local result ec elapsed
  set +e
  result="$(
    FOUNDRY_BIN="$FOUNDRY_BIN" \
    SMOKE_SPEC="$SMOKE_SPEC" \
    DEST="$dest" \
    OUT_RAW="$out_raw" \
    OUT_TS="$out_ts" \
    OUT_ERR="$out_err" \
    CACHE_GOMOD="$cache_gomod" \
    CACHE_GO="$cache_go" \
    GOMODCACHE_PATH="$gomodcache_path" \
    GOCACHE_PATH="$gocache_path" \
    python3 <<'PY'
import os, subprocess, shutil, time

foundry = os.environ["FOUNDRY_BIN"]
spec = os.environ["SMOKE_SPEC"]
dest = os.environ["DEST"]
out_raw = os.environ["OUT_RAW"]
out_ts = os.environ["OUT_TS"]
out_err = os.environ["OUT_ERR"]
cache_gomod = os.environ["CACHE_GOMOD"]
cache_go = os.environ["CACHE_GO"]

env = os.environ.copy()
if cache_gomod == "cleared" or cache_go == "cleared":
    env["GOMODCACHE"] = os.environ["GOMODCACHE_PATH"]
    env["GOCACHE"] = os.environ["GOCACHE_PATH"]

cmd = [foundry, "generate", "--spec", spec, "--dest", dest, "--verify", "default"]
stdbuf = shutil.which("stdbuf")
if stdbuf:
    cmd = [stdbuf, "-oL", "-eL"] + cmd

start = int(time.time() * 1000)
with open(out_raw, "w", encoding="utf-8") as raw, \
     open(out_ts, "w", encoding="utf-8") as ts, \
     open(out_err, "w", encoding="utf-8") as err:
    proc = subprocess.Popen(
        cmd,
        stdout=subprocess.PIPE,
        stderr=err,
        env=env,
        text=True,
        bufsize=1,
    )
    assert proc.stdout is not None
    for line in proc.stdout:
        now = int(time.time() * 1000)
        raw.write(line)
        raw.flush()
        ts.write(f"{now}\t{line.rstrip(chr(10))}\n")
        ts.flush()
    ec = proc.wait()
end = int(time.time() * 1000)
print(f"{ec}\t{end - start}")
PY
  )"
  set -e
  ec="$(printf '%s' "$result" | cut -f1)"
  elapsed="$(printf '%s' "$result" | cut -f2)"
  [[ -n "$ec" ]] || ec=1
  [[ -n "$elapsed" ]] || elapsed=0

  {
    echo "label=$label"
    echo "sample_id=$sample_id"
    echo "timestamp_utc=$(iso_utc)"
    echo "cache_gomod=$cache_gomod"
    echo "cache_go=$cache_go"
    echo "exit_code=$ec"
    echo "elapsed_ms=$elapsed"
    echo "command=foundry generate --spec integration/fixtures/foundry-smoke-cli/foundry.toml --dest <parent>/foundry-smoke-cli --verify default"
    echo "dest_basename=foundry-smoke-cli"
  } >"$out_meta"

  log "generate sample=$sample_id label=$label exit=$ec elapsed_ms=$elapsed"
  if [[ "$ec" -eq 0 ]]; then
    rm -rf "$parent"
  else
    log "warn: preserved parent=$parent (generate failed)"
  fi
  return "$ec"
}

# Warm: host module + build caches (Section 49 warm baseline).
for ((i = 1; i <= WARM_N; i++)); do
  log "generate warm $i/$WARM_N"
  run_generate_sample "warm_host_cache" "host_warm" "host_warm" "warm_$(printf '%02d' "$i")" || true
done

# Cold: empty GOMODCACHE + GOCACHE (true cold-module-cache case, labeled).
if [[ "$SKIP_COLD" -eq 0 ]]; then
  for ((i = 1; i <= COLD_N; i++)); do
    log "generate cold (cleared module+build cache) $i/$COLD_N"
    run_generate_sample "cold_gomodcache_cleared" "cleared" "cleared" "cold_$(printf '%02d' "$i")" || true
  done
else
  log "skipping cold cleared-cache generate (--skip-cold)"
fi

# Fresh parent + warm host caches (matches dogfood seed labeling).
log "generate cold_parent with warm host caches"
run_generate_sample "cold_parent_warm_cache" "host_warm" "host_warm" "cold_parent_01" || true

# --- Aggregate to JSON + summary ----------------------------------------------
log "aggregating JSON"

export PERF_OUT_DIR="$OUT_DIR"
export PERF_SAMPLES_DIR="$SAMPLES_DIR"
export PERF_CAPTURED_AT="$CAPTURED_AT"
export PERF_HOST_NAME="$HOST_NAME"
export PERF_MACHINE_CLASS="$MACHINE_CLASS"
export PERF_OS="$OS_NAME"
export PERF_ARCH="$ARCH_NAME"
export PERF_KERNEL="$KERNEL"
export PERF_GO_VER="$GO_VER"
export PERF_GIT_VER="$GIT_VER"
export PERF_FOUNDRY_VER="$FOUNDRY_VER"
export PERF_WRITE_FREE_N="$WRITE_FREE_N"
export PERF_WARM_N="$WARM_N"
export PERF_COLD_N="$COLD_N"
export PERF_SKIP_COLD="$SKIP_COLD"
export PERF_SMOKE_SPEC="integration/fixtures/foundry-smoke-cli/foundry.toml"
export PERF_BEAD="go-foundry-cli-j8h.1"

python3 <<'PY'
import json, os, statistics
from pathlib import Path

samples_dir = Path(os.environ["PERF_SAMPLES_DIR"])
out_dir = Path(os.environ["PERF_OUT_DIR"])
out_dir.mkdir(parents=True, exist_ok=True)

def percentile(sorted_vals, p):
    if not sorted_vals:
        return None
    if len(sorted_vals) == 1:
        return sorted_vals[0]
    # Nearest-rank style used for stable small-N baselines.
    k = max(0, min(len(sorted_vals) - 1, int(round((p / 100.0) * (len(sorted_vals) - 1)))))
    return sorted_vals[k]

def stats_from_ms_file(path):
    vals = []
    if path.exists():
        for line in path.read_text(encoding="utf-8").splitlines():
            line = line.strip()
            if not line:
                continue
            vals.append(int(line))
    vals_sorted = sorted(vals)
    return {
        "n": len(vals),
        "samples_ms": vals,
        "min_ms": min(vals) if vals else None,
        "max_ms": max(vals) if vals else None,
        "p50_ms": percentile(vals_sorted, 50),
        "p95_ms": percentile(vals_sorted, 95),
        "mean_ms": round(statistics.mean(vals), 2) if vals else None,
    }

write_free = {}
for name in ("version", "catalog_list", "validate", "plan"):
    write_free[name] = stats_from_ms_file(samples_dir / f"wf_{name}.ms")

def parse_meta(path):
    meta = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        if "=" in line:
            k, v = line.split("=", 1)
            meta[k] = v
    return meta

def stages_from_ts(ts_path):
    """Derive per-stage ms from progress: event boundary timestamps."""
    if not ts_path.exists():
        return []
    progress = []  # (ms, stage_id)
    for line in ts_path.read_text(encoding="utf-8").splitlines():
        if "\t" not in line:
            continue
        ms_s, text = line.split("\t", 1)
        text = text.strip()
        if text.startswith("progress: "):
            stage = text[len("progress: "):].strip()
            try:
                progress.append((int(ms_s), stage))
            except ValueError:
                continue
    stages = []
    for i, (ms, stage) in enumerate(progress):
        if i + 1 < len(progress):
            elapsed = progress[i + 1][0] - ms
        else:
            elapsed = 0
        stages.append({"id": stage, "elapsed_ms": max(0, elapsed)})
    return stages

generate_samples = []
for meta_path in sorted(samples_dir.glob("*.meta")):
    meta = parse_meta(meta_path)
    sid = meta.get("sample_id", meta_path.stem)
    ts_path = samples_dir / f"{sid}.ts"
    stages = stages_from_ts(ts_path)
    try:
        elapsed = int(meta.get("elapsed_ms", "0"))
    except ValueError:
        elapsed = 0
    try:
        exit_code = int(meta.get("exit_code", "1"))
    except ValueError:
        exit_code = 1
    generate_samples.append({
        "timestamp_utc": meta.get("timestamp_utc"),
        "label": meta.get("label"),
        "sample_id": sid,
        "cache_state": {
            "gomodcache": meta.get("cache_gomod"),
            "gocache": meta.get("cache_go"),
            "parent": "fresh",
        },
        "command": meta.get("command"),
        "exit_code": exit_code,
        "elapsed_ms": elapsed,
        "stages": stages,
    })

def agg(samples):
    vals = sorted(s["elapsed_ms"] for s in samples if s.get("exit_code") == 0)
    return {
        "n": len(vals),
        "samples_ms": vals,
        "min_ms": min(vals) if vals else None,
        "max_ms": max(vals) if vals else None,
        "p50_ms": percentile(vals, 50),
        "p95_ms": percentile(vals, 95),
        "mean_ms": round(statistics.mean(vals), 2) if vals else None,
    }

warm = [s for s in generate_samples if s.get("label") == "warm_host_cache"]
cold = [s for s in generate_samples if s.get("label") == "cold_gomodcache_cleared"]
cold_parent = [s for s in generate_samples if s.get("label") == "cold_parent_warm_cache"]

doc = {
    "schema_version": 1,
    "purpose": "Section 49 / REQ-165 performance baselines. NOT an absolute gate.",
    "bead": os.environ.get("PERF_BEAD", "go-foundry-cli-j8h.1"),
    "captured_at_utc": os.environ["PERF_CAPTURED_AT"],
    "host": {
        "hostname": os.environ["PERF_HOST_NAME"],
        "machine_class": os.environ["PERF_MACHINE_CLASS"],
        "os": os.environ["PERF_OS"],
        "arch": os.environ["PERF_ARCH"],
        "kernel": os.environ["PERF_KERNEL"],
        "go": os.environ["PERF_GO_VER"],
        "git": os.environ["PERF_GIT_VER"],
        "foundry_version": os.environ["PERF_FOUNDRY_VER"],
    },
    "config": {
        "write_free_iterations": int(os.environ["PERF_WRITE_FREE_N"]),
        "generate_warm_iterations": int(os.environ["PERF_WARM_N"]),
        "generate_cold_iterations": int(os.environ["PERF_COLD_N"]),
        "skip_cold": os.environ["PERF_SKIP_COLD"] == "1",
        "smoke_spec": os.environ["PERF_SMOKE_SPEC"],
        "stage_timing_method": "wall_clock_delta_between_progress_events",
    },
    "write_free": write_free,
    "generate": {
        "samples": generate_samples,
        "aggregates": {
            "warm_host_cache": agg(warm),
            "cold_gomodcache_cleared": agg(cold),
            "cold_parent_warm_cache": agg(cold_parent),
        },
    },
    "gates": {
        "absolute_thresholds": None,
        "note": (
            "No absolute pass/fail millisecond gates. Capture and format only "
            "(REQ-165 / Section 49). Thresholds require multi-machine history."
        ),
    },
}

stamp = os.environ["PERF_CAPTURED_AT"].replace(":", "").replace("-", "")
json_name = f"baselines-{stamp}-{os.environ['PERF_MACHINE_CLASS']}.json"
json_path = out_dir / json_name
latest_path = out_dir / "baselines-latest.json"
payload = json.dumps(doc, indent=2, sort_keys=False) + "\n"
json_path.write_text(payload, encoding="utf-8")
latest_path.write_text(payload, encoding="utf-8")

def fmt_stats(s):
    if not s or s.get("n", 0) == 0:
        return "n/a"
    return f"n={s['n']} p50={s['p50_ms']}ms p95={s['p95_ms']}ms min={s['min_ms']} max={s['max_ms']}"

lines = []
lines.append("# Performance baselines (Section 49 / REQ-165)")
lines.append("")
lines.append(f"- **Bead:** `{doc['bead']}`")
lines.append(f"- **Captured (UTC):** {doc['captured_at_utc']}")
lines.append(f"- **Machine class:** `{doc['host']['machine_class']}`")
lines.append(
    f"- **Host:** `{doc['host']['hostname']}` {doc['host']['os']}/{doc['host']['arch']} "
    f"kernel `{doc['host']['kernel']}`"
)
lines.append(f"- **Go:** `{doc['host']['go']}`")
lines.append(f"- **Git:** `{doc['host']['git']}`")
lines.append(f"- **Foundry:** `{doc['host']['foundry_version']}`")
lines.append(
    f"- **Machine JSON:** [`{json_name}`]({json_name}) · "
    f"[`baselines-latest.json`](baselines-latest.json)"
)
lines.append("")
lines.append("## Absolute gates")
lines.append("")
lines.append(
    "**None.** This capture does not invent pass/fail thresholds "
    "(Section 49 / REQ-165)."
)
lines.append(
    "P2.8 exit review cites this pack as evidence that baselines were measured."
)
lines.append("")
lines.append("## Write-free commands (tool-free; SHOULD feel immediate)")
lines.append("")
lines.append("| Command | n | p50 (ms) | p95 (ms) | min | max |")
lines.append("| ------- | - | -------- | -------- | --- | --- |")
for key, label in (
    ("version", "version"),
    ("catalog_list", "catalog list"),
    ("validate", "validate"),
    ("plan", "plan"),
):
    s = write_free[key]
    lines.append(
        f"| `{label}` | {s['n']} | {s['p50_ms']} | {s['p95_ms']} | "
        f"{s['min_ms']} | {s['max_ms']} |"
    )
lines.append("")
lines.append("## Generate `foundry-smoke-cli` (default verify)")
lines.append("")
lines.append("| Label | Cache state | Aggregate |")
lines.append("| ----- | ----------- | --------- |")
for key, label in (
    ("warm_host_cache", "warm host GOMODCACHE/GOCACHE"),
    ("cold_gomodcache_cleared", "cold empty GOMODCACHE+GOCACHE"),
    ("cold_parent_warm_cache", "fresh parent, warm host caches"),
):
    lines.append(
        f"| `{key}` | {label} | {fmt_stats(doc['generate']['aggregates'][key])} |"
    )
lines.append("")
lines.append("### Samples")
lines.append("")
for s in generate_samples:
    lines.append(
        f"- `{s['sample_id']}` label=`{s['label']}` exit={s['exit_code']} "
        f"elapsed_ms={s['elapsed_ms']} gomodcache={s['cache_state']['gomodcache']} "
        f"stages={len(s.get('stages') or [])}"
    )
    stages = sorted(s.get("stages") or [], key=lambda x: -x.get("elapsed_ms", 0))
    top = [st for st in stages if st.get("elapsed_ms", 0) > 0][:8]
    if top:
        bits = ", ".join(f"{st['id']}={st['elapsed_ms']}ms" for st in top)
        lines.append(f"  - top stages: {bits}")
lines.append("")
lines.append("### Stage timing method")
lines.append("")
lines.append("Per-stage `elapsed_ms` is the wall-clock delta between consecutive")
lines.append("`progress: <stage-id>` lines emitted by Foundry (Section 29.2 / 36.2).")
lines.append("This uses Foundry's own progress events as boundaries; it is not a")
lines.append("substitute for internal step-logger durations, but is stable for baselines.")
lines.append("")
lines.append("## Reproduce")
lines.append("")
lines.append("```bash")
lines.append("./scripts/perf/capture-baselines.sh")
lines.append("# optional: WRITE_FREE_N=20 WARM_N=3 COLD_N=1")
lines.append("# optional: FOUNDRY_PERF_OUT=/tmp/perf-out ./scripts/perf/capture-baselines.sh --quick")
lines.append("```")
lines.append("")
lines.append(
    "Optional CI: `.github/workflows/perf-baselines.yml` "
    "(`workflow_dispatch` only; not a required gate)."
)
lines.append("")

summary_path = out_dir / "summary.md"
summary_path.write_text("\n".join(lines) + "\n", encoding="utf-8")

schema = {
    "schema_version": 1,
    "description": "Foundry Section 49 performance baseline capture format (REQ-165).",
    "required_top_level": [
        "schema_version", "purpose", "bead", "captured_at_utc",
        "host", "config", "write_free", "generate", "gates",
    ],
    "required_host": [
        "hostname", "machine_class", "os", "arch", "go", "git", "foundry_version",
    ],
    "required_sample": [
        "timestamp_utc", "label", "cache_state", "command", "exit_code",
        "elapsed_ms", "stages",
    ],
    "required_stage": ["id", "elapsed_ms"],
    "labels": [
        "warm_host_cache",
        "cold_gomodcache_cleared",
        "cold_parent_warm_cache",
    ],
    "write_free_commands": ["version", "catalog_list", "validate", "plan"],
    "gates_policy": "absolute_thresholds MUST be null until multi-machine history exists",
}
(out_dir / "schema.json").write_text(json.dumps(schema, indent=2) + "\n", encoding="utf-8")

print(f"wrote {json_path}")
print(f"wrote {latest_path}")
print(f"wrote {summary_path}")
PY

log "done → $OUT_DIR"
ls -la "$OUT_DIR" >&2
