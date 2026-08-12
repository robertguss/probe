#!/usr/bin/env python3
"""Parse `go test -count=N -json` outputs and flag flaky / high-variance tests.

Usage:
    python3 scripts/flaky-report.py <label:path.json>... > flaky-report.txt

Each argument is a suite label and the path to a go-test JSON stream,
separated by a colon, e.g.:

    unit:unit.json generate:generate.json

Exit status is 1 when any test is flagged (flaky or high variance). The same
report is also written to a JSON sidecar if FOUNDRY_FLAKY_REPORT_JSON is set.
"""

import json
import math
import os
import sys
from collections import defaultdict

CV_THRESHOLD = float(os.environ.get("FOUNDRY_FLAKY_CV_THRESHOLD", "0.50"))


def cv(values):
    if len(values) < 2:
        return 0.0
    mean = sum(values) / len(values)
    if mean == 0:
        return 0.0
    variance = sum((x - mean) ** 2 for x in values) / (len(values) - 1)
    return math.sqrt(variance) / mean


def top_level(test):
    if not test:
        return "(package)"
    return test.split("/")[0]


def parse(path):
    events = []
    with open(path, "r", encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            try:
                events.append(json.loads(line))
            except json.JSONDecodeError:
                continue
    return events


def summarize(label, events):
    by_test = defaultdict(lambda: {"pass": 0, "fail": 0, "skip": 0, "durations": []})
    start = None
    stop = None
    for ev in events:
        action = ev.get("Action")
        if action == "start" and "Package" in ev and start is None:
            start = ev.get("Time")
        if action == "pass" and "Package" in ev and "Test" not in ev:
            stop = ev.get("Time")
        test = ev.get("Test")
        if not test:
            continue
        tl = top_level(test)
        rec = by_test[tl]
        if action == "pass":
            rec["pass"] += 1
            rec["durations"].append(ev.get("Elapsed", 0.0))
        elif action == "fail":
            rec["fail"] += 1
        elif action == "skip":
            rec["skip"] += 1
    return label, dict(by_test), start, stop


def main():
    if len(sys.argv) < 2:
        print("usage: flaky-report.py <label:path.json>...", file=sys.stderr)
        sys.exit(2)

    summaries = []
    for arg in sys.argv[1:]:
        if ":" not in arg:
            print(f"bad argument (want label:path): {arg}", file=sys.stderr)
            sys.exit(2)
        label, path = arg.split(":", 1)
        events = parse(path)
        summaries.append(summarize(label, events))

    flagged = []
    lines = []
    lines.append("# Flaky-test detection report")
    lines.append("")

    for label, by_test, start, stop in summaries:
        lines.append(f"## {label}")
        total = len(by_test)
        flaky = []
        high_var = []
        for name, rec in sorted(by_test.items()):
            runs = rec["pass"] + rec["fail"] + rec["skip"]
            if rec["fail"] > 0:
                flaky.append((name, rec["pass"], rec["fail"], rec["skip"]))
            elif rec["durations"]:
                c = cv(rec["durations"])
                if c > CV_THRESHOLD:
                    high_var.append((name, c, rec["durations"]))
        lines.append(f"- top-level tests observed: {total}")
        lines.append(f"- flaky (>=1 failure): {len(flaky)}")
        lines.append(f"- high duration variance (CV>{CV_THRESHOLD:.0%}): {len(high_var)}")
        if flaky:
            lines.append("")
            lines.append("### Flaky tests")
            for name, p, f, s in flaky:
                lines.append(f"- {name}: pass={p} fail={f} skip={s}")
                flagged.append({"suite": label, "test": name, "kind": "flaky", "pass": p, "fail": f, "skip": s})
        if high_var:
            lines.append("")
            lines.append("### High-variance tests")
            for name, c, durations in high_var:
                times = ", ".join(f"{d:.3f}s" for d in durations)
                lines.append(f"- {name}: CV={c:.2%} durations=[{times}]")
                flagged.append({"suite": label, "test": name, "kind": "variance", "cv": round(c, 4), "durations": [round(d, 4) for d in durations]})
        lines.append("")

    if flagged:
        lines.append(f"## Result: {len(flagged)} issue(s) flagged")
    else:
        lines.append("## Result: no flaky or high-variance tests flagged")

    report = "\n".join(lines) + "\n"
    sys.stdout.write(report)

    json_path = os.environ.get("FOUNDRY_FLAKY_REPORT_JSON")
    if json_path:
        with open(json_path, "w", encoding="utf-8") as fh:
            json.dump({"flagged": flagged, "summary_text": report}, fh, indent=2)

    if flagged:
        sys.exit(1)


if __name__ == "__main__":
    main()
