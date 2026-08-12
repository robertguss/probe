#!/usr/bin/env python3
"""Compare warm-host-cache generate baseline against a current capture.

Usage:
    python3 scripts/perf/compare-perf-baseline.py <baseline.json> <current.json>

Compares the `generate.aggregates.warm_host_cache.p50_ms` value. A regression is
flagged when the current p50 exceeds the baseline p50 by more than the threshold
(default 10%). Cold-run data and other labels are reported for evidence only.
"""

import json
import os
import sys

THRESHOLD = float(os.environ.get("FOUNDRY_PERF_REGRESSION_THRESHOLD", "0.10"))
LABEL = "warm_host_cache"


def load(path):
    with open(path, "r", encoding="utf-8") as fh:
        return json.load(fh)


def p50(doc):
    return doc.get("generate", {}).get("aggregates", {}).get(LABEL, {}).get("p50_ms")


def n(doc):
    return doc.get("generate", {}).get("aggregates", {}).get(LABEL, {}).get("n")


def main():
    if len(sys.argv) != 3:
        print("usage: compare-perf-baseline.py <baseline.json> <current.json>", file=sys.stderr)
        sys.exit(2)
    base = load(sys.argv[1])
    cur = load(sys.argv[2])

    base_p50 = p50(base)
    cur_p50 = p50(cur)
    cur_n = n(cur)

    lines = []
    lines.append(f"# Performance regression comparison (threshold={THRESHOLD:.0%})")
    lines.append(f"- baseline {LABEL} p50_ms: {base_p50}")
    lines.append(f"- current {LABEL} p50_ms: {cur_p50}")
    lines.append(f"- current {LABEL} n: {cur_n}")

    if cur_n is None or cur_n == 0:
        lines.append("- result: FAIL (no successful current warm runs)")
        print("\n".join(lines))
        sys.exit(1)
    if base_p50 is None or base_p50 == 0:
        lines.append("- result: SKIP (no baseline p50)")
        print("\n".join(lines))
        sys.exit(0)

    ratio = (cur_p50 - base_p50) / base_p50
    lines.append(f"- ratio: {ratio:+.2%}")
    if ratio > THRESHOLD:
        lines.append(f"- result: REGRESSED (>{THRESHOLD:.0%})")
        print("\n".join(lines))
        sys.exit(1)
    lines.append("- result: ok")
    print("\n".join(lines))


if __name__ == "__main__":
    main()
