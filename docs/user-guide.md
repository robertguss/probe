# Foundry User Guide

Foundry is a command-line tool that generates ready-to-use, production-grade
Go **CLI** or **TUI** project repositories from a small TOML file you write
(a **Project Specification**). Point it at a spec, and it produces a
complete, already-tested Go module on disk (with isolated `git init`, but
**no** initial commit) — with CI, docs, and a working command tree or terminal
UI — in seconds.

This guide covers Foundry from an **end-user** point of view: installing it,
writing a spec, and running the commands. It does not cover Foundry's own
internals or how it's built.

> Everything in this guide was verified by building the `foundry` binary
> from this repository and running each command shown below.

## Contents

- [How it works, in one picture](#how-it-works-in-one-picture)
- [Installing Foundry](#installing-foundry)
- [Quickstart](#quickstart)
- [Writing a Project Specification](#writing-a-project-specification)
- [Field reference](#field-reference)
- [Archetypes: `cli` and `tui`](#archetypes-cli-and-tui)
- [Profiles: `distribution`](#profiles-distribution)
- [Commands](#commands)
  - [`foundry init`](#foundry-init)
  - [`foundry validate`](#foundry-validate)
  - [`foundry plan`](#foundry-plan)
  - [`foundry generate`](#foundry-generate)
  - [`foundry catalog list` / `foundry catalog show`](#foundry-catalog-list--foundry-catalog-show)
  - [`foundry doctor`](#foundry-doctor)
  - [`foundry version`](#foundry-version)
- [Global flags](#global-flags)
- [What a generated project looks like](#what-a-generated-project-looks-like)
- [Errors and exit codes](#errors-and-exit-codes)
- [Troubleshooting](#troubleshooting)
- [After generation: adding config or persistence](#after-generation-adding-config-or-persistence)

## How it works, in one picture

```
your-spec.toml  --validate-->  ok / error
                --plan------->  Generation Plan (exactly what will be written; no writes yet)
                --generate--->  a real Go repository on disk, git-initialized (no commit), verified
```

Foundry never writes anything to disk unless you run `generate`, and it
**never overwrites an existing destination** — if the target directory
already exists, `generate` refuses and tells you.

## Installing Foundry

Foundry has not yet published tagged binary releases, so install it by
building from source. You need Go **1.26.5** (the exact toolchain pin the
project uses) to build Foundry and to run `generate`.

```bash
git clone https://github.com/robertguss/go-foundry-cli.git
cd go-foundry-cli
CGO_ENABLED=0 go build -o foundry ./cmd/foundry
./foundry version
./foundry doctor   # checks catalog pin + FOUNDRY_GO_BIN guidance
```

Put the resulting `foundry` binary somewhere on your `PATH` (e.g.
`mv foundry /usr/local/bin/`) to run it as `foundry` from anywhere.

`go install github.com/robertguss/go-foundry-cli/cmd/foundry@latest` also
works once the module is reachable from your machine, but building from a
cloned checkout is the most reliable path today.

### The Go toolchain Foundry needs to *run* `generate`

`foundry generate` shells out to `go` (for `go mod tidy`, `gofmt`, etc.) and
requires the **exact** Go version `go1.26.5`. Nearby versions fail closed with
`tool.wrong_version`. `validate` and `plan` never invoke tools, so they work
regardless of your installed Go version — but `version` and `doctor` still
surface the pin and remediation early.

**Before your first generate**, either put `go1.26.5` first on `PATH` or set:

```bash
export FOUNDRY_GO_BIN=/path/to/go1.26.5/bin/go
```

Examples: module-cache toolchain from `cmd/go`, or
`mise install go@1.26.5 && mise use go@1.26.5`. Run `foundry doctor` anytime
for the pin, `FOUNDRY_GO_BIN`, and a concrete path when one is nearby.

## Quickstart

```bash
# 1. Scaffold a Project Spec (explicit --out; or copy examples/minimal-cli.toml)
foundry init --out my-cli.toml --name my-cli --module github.com/yourname/my-cli \
  --description "My new command-line tool"

# 2. Validate it — no project files are written
foundry validate --spec my-cli.toml

# 3. Plan it — human-readable summary (use --output json for the full plan)
foundry plan --spec my-cli.toml
foundry plan --spec my-cli.toml --verbose   # expands digests / tools / argv

# 4. Generate it — the only command that writes a Generated Project
foundry generate --spec my-cli.toml

# 5. Commit and use your new project (Foundry ran git init only — no commits)
cd my-cli
git add . && git commit -m "Initial commit"
go test ./...
```

The [`examples/`](../examples/) directory has ready-to-copy specs for both
archetypes, plus intentionally-broken examples that demonstrate the exact
error you'll see for common mistakes.

## Writing a Project Specification

A Project Specification is a strict TOML file. A few rules apply to every
spec:

- **Unknown fields are errors.** There's no way to add extra data to a spec;
  an unrecognized key or table fails validation immediately (`spec.unknown_field`).
- **No secrets, no environment variable interpolation, no includes/imports.**
  The file is plain, self-contained data.
- **UTF-8 only, no BOM, 1 MiB max size.**
- Field order in the file doesn't matter.

Minimal valid example (a private CLI):

```toml
schema = 1
name = "minimal-cli"
module = "github.com/example/minimal-cli"
description = "Minimal Foundry CLI example"
archetype = "cli"
destination = "./minimal-cli"
profiles = []
```

## Field reference

| Field | Required | Type | Rules |
| --- | --- | --- | --- |
| `schema` | yes | integer | Must be exactly `1`. |
| `name` | yes | string | Lowercase kebab-case: `[a-z][a-z0-9]*(-[a-z0-9]+)*`, 1–63 bytes. |
| `module` | yes | string | A valid Go module path. Its **final path segment must equal `name`**. No `/vN` semantic-import-version suffix. |
| `description` | yes | string | Single line (no newline), non-empty after trimming, ≤200 characters. |
| `archetype` | yes | string | Exactly `"cli"` or `"tui"`. |
| `destination` | yes | string | Filesystem path where the project will be written. **Its basename must equal `name`.** No `~`, `$`, `${...}`, `..` components, or NUL bytes. Must not already exist when you `generate`. |
| `binary` | no | string | Name of the compiled binary. Same character rules as `name`. Default: `name`. |
| `visibility` | no | string | `"private"` or `"public"`. Default: `"private"`. Required to be `"public"` for the `distribution` profile. |
| `profiles` | no | array of strings | Optional capability profiles to add. Default: `[]`. No duplicates. See [Profiles](#profiles-distribution) — currently the only real profile ID is `distribution`. |
| `[git] init` | no | bool | Whether Foundry runs `git init` on the generated project. Default: `true`. |
| `[git] initial_branch` | no | string | Initial branch name. Same character rules as `name`. Default: `"main"`. |

Anything not in this table (e.g. `configuration`, `local-persistence` as
`profiles` entries, or made-up top-level keys) is rejected. Configuration and
local persistence are documented **recipes** you apply by hand after
generating — see [After generation](#after-generation-adding-config-or-persistence).

## Archetypes: `cli` and `tui`

Every project is exactly one archetype:

- **`cli`** — a [Cobra](https://github.com/spf13/cobra)-based command-line
  tool with root help, `version`, and shell completion already wired up.
- **`tui`** — a [Bubble Tea](https://github.com/charmbracelet/bubbletea)-based
  terminal UI with a working help/quit screen, `--version`, `--no-color`, and
  `--debug-log <name>` flags, and a single well-defined lifecycle owner for
  signal handling (`q` → exit 0, Ctrl-C/signal → exit 130).

Both archetypes come with: a tested `go.mod`/`go.sum`, GitHub Actions CI
(`gofmt`, `go vet`, `go test`, `staticcheck`), Dependabot config, an
`AGENTS.md` file for AI coding agents, and human-facing docs
(`README.md`, `docs/architecture.md`, `docs/commands.md`, `docs/testing.md`).

### Why Cobra + pinned tools (house CLI baseline)

Foundry stays **stdlib-first** for application logic, but the generated CLI
shell uses Cobra because production Go CLIs need a battle-tested command tree
(nested commands, `--help`/`-h`, shell completion, consistent usage errors)
without each project reinventing flag parsing. Cobra is confined to
`internal/cli`; domain packages must not import it.

Pinned `staticcheck` and `govulncheck` (via `go tool` / catalog lock) are the
house quality baseline: formatting and tests catch many mistakes, but
staticcheck finds the rest of the common Go footguns, and govulncheck (strict
/ weekly) keeps dependency CVEs visible. There is no separate "minimal /
no-tools" profile — tiny personal tools still get this baseline so the first
`generate` output is production-shaped rather than a second half-broken mode.

## Profiles: `distribution`

`profiles = []` (the default) is correct for most projects. The only
optional profile currently implemented is `distribution`, which adds
release automation: a GoReleaser config, a release GitHub Actions workflow,
a dependency-review workflow, `CONTRIBUTING.md`, `SECURITY.md`, and
`docs/releasing.md`.

To use it, your spec must satisfy all of:

- `visibility = "public"`
- `module` hosted at exactly `github.com/<owner>/<name>` (no nested path
  segments, no other host)
- `archetype` is `"cli"` or `"tui"` (both are compatible)

```toml
schema = 1
name = "repo-map"
module = "github.com/yourname/repo-map"
description = "Repository inventory CLI with text and JSON output"
archetype = "cli"
destination = "./repo-map"
visibility = "public"
profiles = ["distribution"]
```

Any other `profiles` entry (including `"configuration"` or
`"local-persistence"`, which are documentation-only recipes, not real
profiles) fails with `resolve.unknown_profile` and lists the profiles that
actually exist.

## Commands

Foundry's public commands are: `init`, `validate`, `plan`, `generate`,
`catalog list`, `catalog show`, `doctor`, `version`. Run `foundry --help` or
`foundry <command> --help` any time for the built-in reference.

Every spec-consuming command requires an explicit `--spec <path>` — Foundry
never looks for a `foundry.toml` in your current directory. Use
`--spec -` to pipe a spec in on stdin.

### `foundry init`

Writes one Project Specification TOML to an explicit `--out` path (flags
only; no prompts; no cwd discovery). Refuses if `--out` already exists.
Does **not** generate a project — next steps are still validate → plan →
generate.

```bash
foundry init --out my-cli.toml --name my-cli --module github.com/you/my-cli
foundry init --out app.toml --name app --module github.com/you/app --archetype tui
```

Flags: `--out` (required), `--name` (required), `--module` (required),
`--archetype cli|tui` (default `cli`), `--description`, `--destination`
(default `./<name>`), `--visibility private|public`, `--profile`
(repeatable; only `distribution` today).

### `foundry validate`

Runs the full pipeline (parse → field checks → catalog resolution → plan
construction → read-only destination check) and reports success or the
first error. Nothing is written to disk. Use this in scripts/CI to check a
spec is generation-ready without producing the whole plan.

```bash
foundry validate --spec my-cli.toml
# validate: ok (spec=my-cli.toml plan_sha256=f028f6...)
```

Flags: `--spec <path|->` (required), `--dest <path>` (optional override of
the spec's `destination`, for a quick "what if I generated it here"
check — this does not write anything).

### `foundry plan`

The authoritative dry run. Builds the same Generation Plan `generate` would
execute, without writing anything, running any subprocess, or touching the
network. There is no separate `--dry-run` flag; `plan` **is** the dry run.

**Text mode** (default) prints a human-readable summary: project/destination,
verify checks, file paths, dependencies, external steps (with `network=`),
network reasons, and git init policy (init only — no commits). `--verbose`
expands content digests, tools, and external-step argv.

**`--output json`** emits the complete versioned plan document (machine-
complete: every hash, env allowlist, step timeout, …).

```bash
foundry plan --spec my-cli.toml                      # human-readable summary
foundry plan --spec my-cli.toml --verbose             # expanded text summary
foundry plan --spec my-cli.toml --output json         # full machine-readable plan
foundry plan --spec my-cli.toml --verify strict       # plan including strict verification steps
```

Flags: `--spec` (required), `--dest`, `--verify default|strict` (which
verification step list generate will run — see below; default is
`gofmt` + tests + module-mutation checks, `strict` adds `staticcheck` and
`govulncheck`).

### `foundry generate`

The **only** command that writes a Generated Project. It builds the same plan
as `plan`, then: stages the files in a temporary location, runs `go mod tidy`,
renders every file, verifies the result (formatting, build, tests, and
optionally staticcheck/govulncheck), runs isolated `git init` (no commits, no
git identity), and finally moves the finished tree into `destination` —
atomically, and only if `destination` does not already exist.

```bash
foundry generate --spec my-cli.toml
foundry generate --spec my-cli.toml --verify strict
foundry generate --spec my-cli.toml --quiet     # suppress progress lines; errors/summary still shown
```

Sample output:

```
progress: read-parse-spec
progress: validate-spec
...
network disclosure:
  go-mod-tidy: module resolution during go mod tidy on cold caches
...
generate: wrote destination=/path/to/my-cli plan_sha256=f028f6e3e856...
generate: git initialized (no commits); run: git add . && git commit
```

The word `wrote` means the filesystem transaction succeeded (exclusive
rename into `destination`). It is **not** a git commit.

The `network disclosure` line is Foundry telling you upfront which step(s)
may reach the network (only `go mod tidy`, and only if your module cache
doesn't already have the needed modules). There is no `--offline` flag.

If `destination` already exists, `generate` refuses immediately with
`fs.destination_exists` — Foundry never overwrites, merges into, or deletes
an existing directory. If generation fails partway through, Foundry never
silently deletes your work-in-progress; if a staged/temporary tree is left
behind, the error message tells you exactly where it is so you can inspect
or remove it yourself.

Flags: `--spec` (required), `--dest`, `--verify default|strict`.

### `foundry catalog list` / `foundry catalog show`

Inspect what Foundry can generate, without touching the network or the
filesystem.

```bash
foundry catalog list
# cli            archetype  Conventional CLI project archetype (Cobra command tree)
# core           core       Core toolchain, module layout, docs, CI, Dependabot, and AGENTS for every Generated Project
# distribution   profile    Complete public binary-release scaffolding (post-MVP)
# tui            archetype  Conventional TUI project archetype (Bubble Tea stack)

foundry catalog show cli
# id=cli kind=archetype schema=1
# description=Conventional CLI project archetype (Cobra command tree)
# files:
#   cmd/{{binary}}/main.go render=template ...
# dependencies:
#   github.com/spf13/cobra v1.10.2 scope=runtime
```

Note: appearing in `catalog list` doesn't necessarily mean a unit is
selectable in a spec today — `core` and archetypes are always applied
automatically based on your `archetype` field; `distribution` is the only
profile you can request via `profiles = [...]`.

### `foundry doctor`

Advisory toolchain check: catalog Go pin, this binary's Go version,
`FOUNDRY_GO_BIN`, `PATH`'s `go`, and whether a pinned binary is discoverable
for `generate`. Always exits 0 on success of the probe itself (warnings are
reported in the body). May run closed `go version` probes; does not write
files or use the network.

```bash
foundry doctor
foundry doctor --output json
```

### `foundry version`

Prints the Foundry binary version, commit, the Go toolchain it was built
with, the digest of its embedded catalog, the catalog Go pin, and
`FOUNDRY_GO_BIN` guidance (useful before the first `generate`).

```bash
foundry version
foundry version --output json
```

## Global flags

Available on every command:

| Flag | Values | Notes |
| --- | --- | --- |
| `--output` | `text` (default), `json` | JSON mode is for scripts/agents: it owns stdout exclusively and rejects `--quiet`, `--verbose`, and an explicit `--color`. |
| `--quiet` | flag | Suppress non-essential progress output (text mode only). Errors and final results are still shown. Mutually exclusive with `--verbose`. |
| `--verbose` | flag | Extra diagnostic detail on stderr (text mode only). |
| `--color` | `auto` (default), `always`, `never` | Text mode only. |

## What a generated project looks like

A generated `cli` project (`profiles = []`) looks like:

```
my-cli/
├── AGENTS.md                # instructions for AI coding agents working on this repo
├── README.md
├── docs/
│   ├── architecture.md
│   ├── commands.md
│   └── testing.md
├── cmd/my-cli/main.go
├── internal/
│   ├── cli/                 # Cobra command tree, version command, completion, tests
│   └── version/
├── .github/
│   ├── workflows/ci.yml     # required PR/default job: gofmt, vet, test, staticcheck
│   ├── workflows/strict.yml # weekly/manual: govulncheck, race, extra platforms
│   └── dependabot.yml
├── go.mod / go.sum
├── .gitignore
└── .git/                    # isolated git init only — no commits yet
```

It builds and passes tests immediately. Create the first commit yourself:

```bash
cd my-cli
git add . && git commit -m "Initial commit"
go build ./...
go test -count=1 ./...
```

Adding `profiles = ["distribution"]` (with `visibility = "public"` and a
`github.com/<owner>/<name>` module) additionally adds `.goreleaser.yaml`,
`.github/workflows/release.yml`, `.github/workflows/dependency-review.yml`,
`CONTRIBUTING.md`, `SECURITY.md`, and `docs/releasing.md`.

A `tui` project additionally has `internal/tui/` (the Bubble Tea
model/update/view) and `docs/ui-architecture.md`.

## Errors and exit codes

Every failure is reported as a single machine-readable id in the form
`domain.reason`, always with a `remediation` line telling you how to fix it.
In `--output json` mode the same information is structured:

```json
{
  "schema": 1,
  "command": "validate",
  "ok": false,
  "result": null,
  "error": {
    "error_id": "spec.unknown_field",
    "message": "unknown field or table \"author\" (strict decoding; not in Section 14.3)",
    "remediation": "Remove the unknown field or table named in the error, or correct its spelling.",
    "path": "my-cli.toml",
    "line": 11,
    "col": 1,
    "exit_code": 2
  },
  "warnings": []
}
```

Exit codes:

| Code | Meaning |
| --- | --- |
| `0` | Success. |
| `1` | Runtime/tool/verification failure (e.g. `generate` failed after starting to write). |
| `2` | Input error — bad spec, bad flags, bad destination. |
| `130` | You cancelled it (Ctrl-C / SIGTERM) before anything was committed. |

Common error ids you may see:

| Error id | What it means |
| --- | --- |
| `spec.unknown_field` | Your TOML has a key/table Foundry doesn't recognize. |
| `spec.invalid_field` | A field's value breaks its rule (e.g. `name` isn't kebab-case, `module`'s last segment doesn't match `name`, `destination`'s basename doesn't match `name`). |
| `spec.duplicate_profile` | The same profile ID appears twice in `profiles`. |
| `resolve.unknown_profile` | A `profiles` entry isn't a real, selectable profile (lists the available ones). |
| `resolve.profile_constraint` | A requested profile's requirements aren't met (e.g. `distribution` without `visibility = "public"` or a `github.com/<owner>/<name>` module). |
| `fs.destination_exists` | `generate`'s target directory already exists; Foundry refuses to touch it. |
| `tool.wrong_version` | The `go` binary Foundry found isn't the exact pinned version; see [Installing Foundry](#the-go-toolchain-foundry-needs-to-run-generate). |
| `tool.missing` | A required tool (e.g. `git`, `go`) isn't on `PATH`. |
| `verify.failed` | Generated code failed one of the verification steps (format/build/test/etc.). |
| `usage.invalid` | A command-line usage problem (bad flag value, missing `--spec`, etc.). |

## Troubleshooting

**"specification file not found"** — check the `--spec` path; Foundry never
guesses a default file.

**`generate` says the destination already exists** — pick a new,
non-existent path, or move/delete the old one yourself after inspecting it.
Foundry will not merge into or overwrite it for you.

**`tool.wrong_version`** — you have a `go` on `PATH` that isn't exactly
`go1.26.5`. Run `foundry doctor` for a concrete path when one is nearby, set
`FOUNDRY_GO_BIN` to a `go1.26.5` binary, or install that exact version.

**A profile I want isn't accepted** — run `foundry catalog list` to see
what's actually implemented today. `distribution` is currently the only
selectable profile, and it requires `visibility = "public"` plus a
`github.com/<owner>/<name>` module path.

## After generation: adding config or persistence

Foundry deliberately does **not** offer "configuration" or
"local-persistence" as generation-time profiles — trying to select them by
name fails with `resolve.unknown_profile`. Instead, once you have a
generated project, follow the hand-applied recipes:

- [Configuration recipe](recipes/configuration.md)
- [Local persistence recipe](recipes/local-persistence.md)

These are short guides for adding a config layer (flags → env → optional
TOML) or local storage to your generated project yourself, in a way that
matches the project's existing conventions.
