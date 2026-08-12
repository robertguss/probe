#!/usr/bin/env python3
"""Lint foundry testscript fixtures.

Checks performed:
1. Every custom command registered in `cmd/foundry/script_test.go` is used at
   least once in a testscript file.
2. Custom commands are never invoked with negation (`!`).
3. `! exec foundry ...` is reported as a warning; prefer `assert_exit_code`
   for exact exit-code coverage (informational until scripts are updated).
4. Scripts that invoke foundry without any `assert_exit_code` are reported as
   a warning.
5. Command words that are neither testscript built-ins nor registered custom
   commands are reported as likely typos.

Exit code is 1 when hard errors (unused commands, negated custom commands,
unknown commands) are found. Warnings about exit-code assertions are printed
but do not fail by default.

Usage:
    python3 scripts/lint-testscripts.py [--strict]
"""

import argparse
import os
import re
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
SCRIPT_TEST = REPO / "cmd" / "foundry" / "script_test.go"
TESTDATA = REPO / "cmd" / "foundry" / "testdata"

# Testscript built-ins. Anything else is either a registered custom command or
# a likely typo.
BUILTINS = {
    "cd", "chmod", "cmp", "cmpenv", "cp", "env", "exec", "exists",
    "grep", "mkdir", "mv", "rm", "skip", "stdout", "stderr", "stdin",
    "stop", "symlink", "touch", "wait",
}

FILE_BLOCK_RE = re.compile(r"^--\s+\S+\s+--$")
CONDITION_RE = re.compile(r"^(\[[^\]]+\]\s+)*")


def registered_commands():
    text = SCRIPT_TEST.read_text(encoding="utf-8")
    cmds = set()
    for m in re.finditer(r'"([a-z_][a-z0-9_]*)":\s*func\s*\(', text):
        cmds.add(m.group(1))
    return cmds


def find_scripts():
    return sorted(p for p in TESTDATA.rglob("*.txt") if p.is_file())


def command_word(line):
    # Strip leading testscript condition markers, e.g. [unix] or [root].
    s = CONDITION_RE.sub("", line).lstrip()
    neg = False
    if s.startswith("!"):
        neg = True
        s = s[1:].lstrip()
    parts = s.split(None, 1)
    if not parts:
        return None, neg
    return parts[0], neg


def lint_script(path, registered):
    errors = []
    warnings = []
    text = path.read_text(encoding="utf-8")
    lines = text.splitlines()
    in_file_block = False
    has_assert_exit_code = False
    has_foundry = False
    foundry_lines = []

    for i, raw in enumerate(lines, 1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue

        if FILE_BLOCK_RE.match(line):
            in_file_block = True
            continue
        if in_file_block:
            continue

        word, neg = command_word(line)
        if word is None:
            continue

        if word == "assert_exit_code":
            has_assert_exit_code = True
            continue

        if "foundry" in line and word in ("exec", "assert_exit_code"):
            has_foundry = True
            if word == "exec":
                foundry_lines.append((i, line))

        if word in registered:
            if neg:
                errors.append((i, f"custom command '{word}' must not be negated with '!'"))
            continue

        if word == "exec" and neg:
            rest = CONDITION_RE.sub("", line)
            rest = rest[1:].lstrip() if rest.startswith("!") else rest
            rest = rest[len("exec"):].lstrip()
            if rest.startswith("foundry"):
                warnings.append((i, "prefer assert_exit_code <code> foundry ... over '! exec foundry ...'"))

        if word not in BUILTINS:
            first = CONDITION_RE.sub("", line).split()[0].split("=")[0]
            if first not in BUILTINS and first not in registered:
                errors.append((i, f"unknown command '{word}'"))

    if has_foundry and not has_assert_exit_code:
        warnings.append((0, "script invokes foundry but has no assert_exit_code for exact exit-code coverage"))

    return errors, warnings


def main():
    parser = argparse.ArgumentParser(description="Lint foundry testscript fixtures")
    parser.add_argument("--strict", action="store_true",
                        help="treat exit-code assertion warnings as errors")
    args = parser.parse_args()

    registered = registered_commands()
    if not registered:
        print("error: could not find custom commands in script_test.go", file=sys.stderr)
        return 1

    scripts = find_scripts()
    usage = {cmd: 0 for cmd in registered}

    all_errors = []
    all_warnings = []
    for path in scripts:
        rel = str(path.relative_to(REPO))
        errors, warnings = lint_script(path, registered)
        for i, msg in errors:
            all_errors.append(f"{rel}:{i}: {msg}")
        for i, msg in warnings:
            loc = f"{rel}:{i}" if i else rel
            all_warnings.append(f"{loc}: {msg}")
        for cmd in registered:
            if cmd in path.read_text(encoding="utf-8"):
                usage[cmd] += 1

    for cmd, count in sorted(usage.items()):
        if count == 0:
            all_errors.append(f"{SCRIPT_TEST.relative_to(REPO)}: custom command '{cmd}' is never used")

    if all_warnings:
        print("testscript warnings:")
        for w in all_warnings:
            print(f"  {w}")

    if all_errors:
        print("testscript errors:")
        for e in all_errors:
            print(f"  {e}")

    if all_errors or (args.strict and all_warnings):
        return 1

    print("testscript lint: ok")
    print(f"  scripts: {len(scripts)}")
    print(f"  registered custom commands: {len(registered)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
