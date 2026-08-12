#!/usr/bin/env python3
"""Validate that fenced command examples in docs still run and that error IDs
referenced in docs exist in the source.

Usage:
    python3 scripts/validate-docs.py

Command validation:
  - Scans `docs/recipes/*.md`, `docs/dev/*.md`, and `README.md`.
  - Extracts `bash`/`sh`/`shell` fenced blocks.
  - Runs any line starting with `foundry ` or `./foundry ` against a freshly
    built binary (built at /tmp/foundry-docs-check).
  - Placeholders such as `<path>`, `./project.toml`, `path/to/your.toml`, etc.
    are replaced with `examples/minimal-cli.toml`.
  - Lines still containing unknown placeholders are skipped (logged).
  - A command is expected to succeed unless its line ends with a comment
    containing `expect: non-zero` or `expect: fail`.

Error-ID validation:
  - Extracts strings that look like Foundry diagnostic IDs
    (e.g., `spec.invalid_field`) from the same markdown files.
  - Compares them against the constants in `internal/diagnostic/ids.go`.
  - Any ID referenced in docs that does not exist in the source fails the run.
"""

import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
BINARY = Path("/tmp/foundry")

SPEC = REPO / "examples" / "minimal-cli.toml"
REPLACEMENTS = {
    "<path>": str(SPEC),
    "<path|->": str(SPEC),
    "<spec>": str(SPEC),
    "./project.toml": str(SPEC),
    "path/to/your.toml": str(SPEC),
}


def project_name_from_spec(spec_path):
    text = Path(spec_path).read_text(encoding="utf-8")
    m = re.search(r'^\s*name\s*=\s*"([^"]+)"', text, re.MULTILINE)
    if m:
        return m.group(1)
    return "project"

ID_PATTERN = re.compile(
    r"\b(?:spec|resolve|plan|fs|render|tool|verify|git|report|catalog|usage|internal)"
    r"(?:\.[a-z_][a-z0-9_]*)+\b"
)


def build_binary():
    print(f"building {BINARY} ...")
    res = subprocess.run(
        ["go", "build", "-o", str(BINARY), "./cmd/foundry"],
        cwd=REPO,
        capture_output=True,
        text=True,
    )
    if res.returncode != 0:
        print(res.stderr, file=sys.stderr)
        sys.exit(f"build failed: {res.returncode}")


def go_bin_dir():
    go = shutil.which("go")
    if go:
        return str(Path(go).parent)
    return "/usr/local/go/bin"


def valid_ids():
    ids = set()
    ids_path = REPO / "internal" / "diagnostic" / "ids.go"
    text = ids_path.read_text(encoding="utf-8")
    for m in re.finditer(r'Identifier\s*=\s*"([^"]+)"', text):
        ids.add(m.group(1))
    return ids


def markdown_files():
    files = []
    for subdir in ("docs/recipes", "docs/dev"):
        d = REPO / subdir
        if d.exists():
            files.extend(d.glob("*.md"))
    readme = REPO / "README.md"
    if readme.exists():
        files.append(readme)
    examples = REPO / "examples" / "README.md"
    if examples.exists():
        files.append(examples)
    return sorted(set(files))


def extract_code_blocks(text):
    pattern = re.compile(r"```(?:bash|sh|shell)\s*\n(.*?)```", re.IGNORECASE | re.DOTALL)
    return pattern.findall(text)


def normalize_line(line):
    s = line.strip()
    if not s:
        return None
    if s.startswith(("#", "//", "$ ", "> ")):
        return None
    if "#" in s:
        s = s.split("#", 1)[0].rstrip()
    return s


def is_foundry_command(line):
    return line.startswith("foundry ") or line.startswith("./foundry ")


def rewrite_command(cmd, temp_parent, label):
    # Use the built binary regardless of how the example invoked it.
    cmd = re.sub(r"^\./foundry\b", "foundry", cmd)

    # README examples use a positional spec path; translate to --spec.
    for verb in ("validate", "plan", "generate"):
        cmd = re.sub(
            rf"^foundry\s+{verb}\s+(?!--)",
            rf"foundry {verb} --spec ",
            cmd,
        )

    # Placeholder substitutions.
    for old, new in REPLACEMENTS.items():
        cmd = cmd.replace(old, new)

    # If the example generates into a fixed/relative dest, replace it with a
    # private temp dir whose basename matches the project name in the spec.
    # If no --dest is supplied, append one so the default destination from the
    # spec does not collide with an existing working-directory entry.
    if "foundry generate" in cmd:
        name = project_name_from_spec(SPEC)
        parent = Path(temp_parent) / label
        parent.mkdir(parents=True, exist_ok=True)
        dest = parent / name
        if "--dest" in cmd:
            cmd = re.sub(r"--dest\s+\S+", f"--dest {dest}", cmd)
        else:
            cmd = f"{cmd} --dest {dest}"

    return cmd


def run_command(cmd, timeout=120):
    print(f"  run: {cmd}")
    env = os.environ.copy()
    env["PATH"] = f"{BINARY.parent}:{go_bin_dir()}:{env.get('PATH', '')}"
    res = subprocess.run(
        cmd,
        shell=True,
        cwd=REPO,
        env=env,
        capture_output=True,
        text=True,
        timeout=timeout,
    )
    if res.returncode != 0:
        print(f"  FAIL exit={res.returncode}", file=sys.stderr)
        if res.stderr:
            print(res.stderr[:500], file=sys.stderr)
        return False
    return True


def validate_commands(files):
    failures = []
    skipped = []
    with tempfile.TemporaryDirectory(prefix="foundry-docs-") as temp_parent:
        counter = 0
        for path in files:
            text = path.read_text(encoding="utf-8")
            blocks = extract_code_blocks(text)
            for block in blocks:
                for raw in block.splitlines():
                    line = normalize_line(raw)
                    if not line or not is_foundry_command(line):
                        continue
                    counter += 1
                    cmd = rewrite_command(line, temp_parent, f"run-{counter:03d}")
                    if any(c in cmd for c in "<>"):
                        skipped.append((path.name, raw.strip()))
                        continue
                    if "expect: non-zero" in raw or "expect: fail" in raw:
                        continue
                    if not run_command(cmd):
                        failures.append((path.name, cmd))
    if skipped:
        print(f"\nSkipped {len(skipped)} commands with unresolved placeholders:")
        for name, cmd in skipped:
            print(f"  [{name}] {cmd}")
    return failures


def validate_ids(files, valid):
    failures = []
    for path in files:
        text = path.read_text(encoding="utf-8")
        found = set(ID_PATTERN.findall(text))
        for id_ in sorted(found):
            if id_ not in valid:
                failures.append((str(path.relative_to(REPO)), id_))
    return failures


def main():
    build_binary()
    files = markdown_files()
    print(f"scanning {len(files)} markdown file(s)")

    cmd_failures = validate_commands(files)
    if cmd_failures:
        print("\nCommand failures:", file=sys.stderr)
        for name, cmd in cmd_failures:
            print(f"  [{name}] {cmd}", file=sys.stderr)

    valid = valid_ids()
    id_failures = validate_ids(files, valid)
    if id_failures:
        print("\nUnknown error IDs in docs:", file=sys.stderr)
        for rel, id_ in id_failures:
            print(f"  {rel}: {id_}", file=sys.stderr)

    ok = not cmd_failures and not id_failures
    print(f"\nresult: {'ok' if ok else 'FAILED'}")
    sys.exit(0 if ok else 1)


if __name__ == "__main__":
    main()
