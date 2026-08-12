#!/usr/bin/env python3
"""Compare a fresh benchmark run against a stored baseline.

Usage:
    python3 scripts/perf/bench-capture.py -count=5 > current.json
    python3 scripts/perf/bench-compare.py baseline.json current.json

Statistical method:
    - Use the median of the stored baseline values as the reference.
    - Use the median of the current run values as the observed value.
    - A benchmark regresses when the observed median is greater than the
      reference median multiplied by (1 + REGRESSION_THRESHOLD).

The threshold is intentionally conservative to reduce CI noise; it can be
overridden with FOUNDRY_BENCH_REGRESSION_THRESHOLD.
"""

import json
import os
import statistics
import sys

REGRESSION_THRESHOLD = float(os.environ.get("FOUNDRY_BENCH_REGRESSION_THRESHOLD", "0.20"))


def median(values):
    if not values:
        return 0.0
    return statistics.median(values)


def load(path):
    with open(path, "r", encoding="utf-8") as fh:
        return json.load(fh)


def main():
    if len(sys.argv) != 3:
        print("usage: bench-compare.py <baseline.json> <current.json>", file=sys.stderr)
        sys.exit(2)
    baseline = load(sys.argv[1])
    current = load(sys.argv[2])

    base_by_key = {
        f"{b['pkg']}:{b['name']}": b["values"]
        for b in baseline.get("benchmarks", [])
    }
    cur_by_key = {
        f"{c['pkg']}:{c['name']}": c["values"]
        for c in current.get("benchmarks", [])
    }

    regressions = []
    missing = []
    lines = []
    lines.append(f"# Benchmark regression report (threshold={REGRESSION_THRESHOLD:.0%})")
    lines.append("")

    for key in sorted(base_by_key):
        if key not in cur_by_key:
            missing.append(key)
            continue
        base_med = median(base_by_key[key])
        cur_med = median(cur_by_key[key])
        if base_med == 0:
            ratio = 0.0
        else:
            ratio = (cur_med - base_med) / base_med
        status = "ok"
        if ratio > REGRESSION_THRESHOLD:
            status = "REGRESSED"
            regressions.append({
                "benchmark": key,
                "base_median_ns": base_med,
                "current_median_ns": cur_med,
                "ratio": round(ratio, 4),
            })
        lines.append(
            f"{key}: base={base_med:.2f} current={cur_med:.2f} ratio={ratio:+.2%} [{status}]"
        )

    if missing:
        lines.append("")
        lines.append("# Missing benchmarks in current run")
        for key in missing:
            lines.append(f"- {key}")

    if regressions:
        lines.append("")
        lines.append(f"# {len(regressions)} regression(s) detected")
        for r in regressions:
            lines.append(
                f"- {r['benchmark']}: +{r['ratio']:.2%} "
                f"(base={r['base_median_ns']:.2f} current={r['current_median_ns']:.2f})"
            )
    else:
        lines.append("")
        lines.append("# No regressions detected")

    report = "\n".join(lines) + "\n"
    sys.stdout.write(report)

    json_path = os.environ.get("FOUNDRY_BENCH_COMPARE_JSON")
    if json_path:
        with open(json_path, "w", encoding="utf-8") as fh:
            json.dump({"regressions": regressions, "missing": missing, "summary_text": report}, fh, indent=2)

    if regressions or missing:
        sys.exit(1)


if __name__ == "__main__":
    main()
