#!/usr/bin/env python3
"""Capture Go benchmark results for selected packages into a JSON baseline.

Usage:
    python3 scripts/perf/bench-capture.py > baseline.json

Reads lines like:
    BenchmarkX-14    12345    6789 ns/op
from `go test -bench=. -count=N` output for the hot-path packages.
"""

import json
import re
import subprocess
import sys

PACKAGES = [
    "./internal/spec",
    "./internal/resolve",
    "./internal/plan",
    "./internal/render",
]

BENCH_RE = re.compile(r"^(Benchmark\S+)\s+\d+\s+([\d.]+)\s+ns/op")


def run(count: str, benchtime: str) -> dict:
    out: dict[str, list[float]] = {}
    for pkg in PACKAGES:
        cmd = [
            "go", "test", pkg,
            "-run=^$",
            f"-bench=.",
            f"-count={count}",
            f"-benchtime={benchtime}",
        ]
        proc = subprocess.run(cmd, capture_output=True, text=True)
        if proc.returncode != 0:
            print(proc.stderr, file=sys.stderr)
            raise SystemExit(f"benchmark failed for {pkg}: exit {proc.returncode}")
        for line in proc.stdout.splitlines():
            m = BENCH_RE.match(line)
            if not m:
                continue
            name = m.group(1)
            ns_op = float(m.group(2))
            key = f"{pkg}:{name}"
            out.setdefault(key, []).append(ns_op)
    benchmarks = []
    for key, values in sorted(out.items()):
        pkg, name = key.split(":", 1)
        benchmarks.append({
            "pkg": pkg,
            "name": name,
            "unit": "ns/op",
            "values": values,
        })
    return {"benchmarks": benchmarks}


if __name__ == "__main__":
    count = sys.argv[1] if len(sys.argv) > 1 else "1"
    benchtime = sys.argv[2] if len(sys.argv) > 2 else "200ms"
    print(json.dumps(run(count, benchtime), indent=2))
