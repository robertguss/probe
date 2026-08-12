# Go Foundry — Revised Definitive Specification

## 1. Artifact Metadata

| Field                     | Value                                                                                                                                                                                                                                                                            |
| ------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Artifact name             | Go Foundry — Revised Definitive Specification                                                                                                                                                                                                                                    |
| Artifact ID               | `SPEC-FOUNDRY-002`                                                                                                                                                                                                                                                               |
| Version                   | `2.0`                                                                                                                                                                                                                                                                            |
| Status                    | **Accepted — implementation authority** (Phase 1 may begin now; Phase 2 and Phase 3 entries are gated on the recorded evidence items in Section 51.2)                                                                                                                            |
| Program                   | Go Foundry Research Program                                                                                                                                                                                                                                                      |
| Stage                     | Stage 6 — Revised Definitive Specification                                                                                                                                                                                                                                       |
| Owner                     | Robert Guss                                                                                                                                                                                                                                                                      |
| Revision date             | 2026-07-30 (America/New_York)                                                                                                                                                                                                                                                    |
| Supersedes                | `docs/specifications/01-definitive-foundry-specification.md` (`SPEC-FOUNDRY-001`) in full                                                                                                                                                                                        |
| Review input              | `docs/reviews/01-definitive-foundry-specification-adversarial-review.md` (`REVIEW-FOUNDRY-001`)                                                                                                                                                                                  |
| Governing authorities     | `docs/00-program-blueprint.md` (`DEC-001`–`DEC-016`), `docs/01-research-charter.md`; no separate accepted `DEC-*.md` files were supplied                                                                                                                                         |
| Stage 5 findings          | 19 total: 2 Critical, 10 High, 7 Medium, 0 Low                                                                                                                                                                                                                                   |
| Finding dispositions      | 16 Accepted; 3 Accepted with modification (`FND-002`, `FND-011`, `FND-017` — each strengthened beyond the review's proposed remedy); 0 Rejected; 0 Deferred; 0 Not applicable                                                                                                    |
| Normative requirements    | 125 active; no identifier retired, renumbered, or reused (`REQ-073`–`REQ-075` remain active as exclusion-and-recipe requirements)                                                                                                                                                |
| Risks                     | `RSK-300`–`RSK-312` (revised) plus `RSK-400`–`RSK-403` (inherited from Stage 5)                                                                                                                                                                                                  |
| Open questions            | `OQ-300` (Phase 4 only); `OQ-400` (architecturally resolved in this revision; executable confirmation is the mandatory E1 Phase 2 entry gate)                                                                                                                                    |
| Implementation permission | Phase 1 (pure specification, catalog, resolution, and planning — no filesystem writes, tools, or network) may begin immediately. Phase 1 exit and Phases 2–3 entries MUST NOT proceed until the corresponding Section 51.2 evidence records exist; a contradicting record forces a reviewed revision of this artifact before the gated phase proceeds |

This artifact is standalone. A reader does not need the Stage 4 specification or
the Stage 5 review to understand what should be built. The Stage 4 document is
retained in the repository as history only; where the two disagree, this
artifact controls.

## 2. Executive Decision Summary

The Foundry is a deterministic, non-interactive, local project compiler,
implemented in Go, that generates complete, verified, Git-initialized,
agent-legible Go CLI and TUI repositories for macOS and Linux from a declarative
TOML Project Specification. Generated Projects are ordinary independent Go
repositories with no runtime or build dependency on the Foundry.

Final selections, after integrating all nineteen Stage 5 findings:

| Decision area            | Final selection                                                                                                                                                                                                                                                             |
| ------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Implementation language  | Go 1.26.5 exactly, for the Foundry and as the generated toolchain pin (`go 1.26.0` directive, `toolchain go1.26.5`). Go 1.26.5 is additionally required because it fixes an `os.Root` symlink escape (CVE-2026-39822) that the transaction contract depends on.               |
| Command surface          | Commands: `init`, `validate`, `plan`, `generate`, `catalog list`, `catalog show`, `doctor`, `version`. `generate` is the only command that writes a Generated Project; `init` may write one Project Spec TOML to an explicit `--out`; `doctor` is advisory and may probe `go version`. One output selector `--output text\|json`; JSON mode owns stdout exclusively and rejects human-mode flags. There is no `--offline` flag and no whole-process network-isolation claim.                        |
| Project Specification    | Strict TOML, integer `schema = 1`, position-independent fields, product intent only, secret-free.                                                                                                                                                                             |
| Archetypes               | Exactly two: `cli` (Cobra v1.10.2) and `tui` (Bubble Tea v2.0.8, Bubbles v2.1.1, Lip Gloss v2.0.5). Exactly one per project.                                                                                                                                                  |
| Generated vertical slice | Minimal compiling, tested shells: CLI owns root help/version/completion; TUI owns one static help/quit screen with a total single-owner lifecycle. No `greet` demo service and no synthetic asynchronous initialization; rich examples live in Foundry integration fixtures. |
| Capability Profiles      | Flat exact-ID model. The MVP implements **no** optional profile. `distribution` is the only initial catalog profile and is implemented post-MVP. `configuration` and `local-persistence` are retired from the generated catalog and preserved as recipes (Section 21).       |
| Profile composition      | No transitive requires, conflict graph, capability registry, helper-binary schema, cycles, or provenance closure. Direct archetype/visibility predicates only.                                                                                                                |
| Rendering                | Three mechanisms only: static copy, restricted complete-file `text/template`, and typed `go.mod` generation via `golang.org/x/mod/modfile` plus `go/format` for Go source. The limited-substitution token language and all typed YAML/GoReleaser emitters are deleted.        |
| Generation transaction   | Descriptor-relative throughout: no-follow component walk with an explicit namespace-custody check; retained parent handle before the first write; stage created, rendered, inspected, and committed relative to retained handles; external tools started from the retained stage descriptor, never a re-resolved pathname; exclusive no-replace commit with post-syscall identity classification; fail closed when unsupported or ambiguous. |
| Stage preservation       | **The Foundry contains no automatic stage deletion.** Once a stage directory is created, every uncommitted outcome — render, tool, verification, Git, cancellation, destination conflict, ambiguity — preserves it and reports its exact location. No production code path unlinks or recursively removes a created stage.                                    |
| Commit outcome           | One result-dominant matrix: commit success fixes exit 0 and cannot be overridden by cancellation or reporting failure (SIGPIPE is caught process-wide so a broken pipe cannot kill the Foundry); a destination conflict preserves the losing stage and exits 2; every other commit failure or ambiguous outcome preserves the verified stage and exits 1.     |
| Verification             | Default: `gofmt` conformance, `go mod tidy` with mutation-set validation, `go mod verify`, `go test -count=1 -buildvcs=false ./...`, `go vet -buildvcs=false ./...`, and a final complete non-`.git` tree conformance check after every external tool. Strict adds Staticcheck and govulncheck. Race is a host-capability/CI gate.        |
| Tool environments        | Constructed from an empty base plus exact allowlists. `GOENV=off`, empty `GOFLAGS`/`GOCACHEPROG`, `GOTOOLCHAIN=local`, `GOWORK=off`, controlled module/VCS/auth policy; Git init is isolated from system/global config and uses an empty owned template directory.            |
| CGO                      | Product, tests, analysis, and release builds use `CGO_ENABLED=0`. The race detector is an explicit development exception using `CGO_ENABLED=1` with compiler preflight; it is never a default-generation requirement.                                                        |
| Generated CI             | One required fast Linux PR/default job (format, uncached tests, vet, Staticcheck) plus one weekly/manual strict workflow (govulncheck, race when prerequisites exist, representative macOS, bounded fuzz when targets exist). Distribution owns release-time validation.      |
| MVP                      | Phase 3: both archetypes generate, verify, and dogfood with `profiles = []`; reduced generated CI; agent acceptance passes.                                                                                                                                                   |
| Phasing                  | No implementation Phase 0 and no circular authority: Phase 1 (pure planning) is authorized now; the five bounded evidence items (Section 51.2) are mandatory phase-entry gates (E5 before Phase 1 exit; E1–E3 before Phase 2; E4 before Phase 3) that verify — and may never silently amend — this architecture. Phases 1–4 follow (planning → CLI transaction/dogfood → TUI/MVP → distribution/release). |
| Largest rejections       | Verification bypass, update/sync/plugins/remote catalogs, `--offline`, generic profile framework, typed workflow emitters, generated demo features, automatic stage deletion, persistent provenance, Windows, Claude-specific conventions.                                    |
| Largest remaining risk   | Descriptor-relative transaction portability until the E1 evidence record exists (`RSK-400`), cold-cache verification (`RSK-301`), TUI lifecycle in real terminals (`RSK-305`), preserved-stage debris burden (`RSK-310`).                                                     |

## 3. Revision Summary

This section summarizes every material change from the Stage 4 specification so
that the reader does not need to inspect the superseded document.

### 3.1 Filesystem transaction is now descriptor-relative (from FND-001, FND-002, FND-003, FND-018)

Stage 4 validated the destination parent with pathname `Lstat` walks and then
created staging with `os.MkdirTemp(parentPath, ...)` and cleaned it with
path-validated recursive removal. Both are time-of-check/time-of-use defects: a
concurrent rename or symlink swap between check and use can place the stage
outside the validated parent or redirect recursive deletion at unrelated data.

The revision replaces the entire pathname model with object ownership. The
Foundry acquires the immediate destination parent through a no-follow
component walk with an explicit namespace-custody check (Section 31.3),
retains its descriptor before the first write, creates the stage exclusively
relative to that handle, renders through an identity-verified rooted writer,
starts external tools from the retained stage descriptor rather than a
re-resolved pathname (Section 34.4), and commits with a kernel-enforced
exclusive no-replace rename relative to the retained parent, classifying the
outcome by post-syscall identity inspection. Path-based `MkdirTemp`,
`RemoveAll`, or equivalent re-resolution of checked pathnames is prohibited in
transaction code.

The highest-impact correction goes further than the review's proposed remedy:
**automatic stage deletion is deleted entirely.** Stage 4 cleaned failed or
losing stages with recursive removal; descriptor-relative recursive deletion
would still be the most dangerous operation in the product and would require
its own hostile test surface. Instead, once a stage directory is created,
every uncommitted outcome preserves it — the Foundry contains no stage
unlink, no recursive removal, no scavenger, and no cleanup command. The stage
is created directly under the destination parent as a hidden `0700` directory
named `.foundry-<name>-<random>`, and every failure report states its exact
identity-confirmed location so the owner can inspect and remove it manually.
A single result-dominant commit matrix now defines every preserve/exit
outcome, replacing Stage 4's contradictory preserve-versus-clean prose. The
synthetic Unicode case-fold sibling scan is deleted; the host filesystem and
the exclusive commit are authoritative for name equivalence.

### 3.2 External tool execution is now closed (from FND-004, FND-005, FND-006, FND-007)

Stage 4 said the Foundry invokes only `go` and `git` from a "narrowly filtered
host environment," globally forced `CGO_ENABLED=0`, ran cached `go test ./...`,
placed its last tree-conformance check before the test/vet/analysis steps, and
offered a `generate --offline` flag whose only enforcement was `GOPROXY=off`.

The revision constructs each tool environment from an empty base plus an exact
allowlist that disables the persisted Go environment file, implicit flags,
external cache programs, toolchain switching, VCS fallback, private-module
routing, and credential helpers, and isolates `git init` from system/global
configuration and host template directories. Two boundary gaps that Stage 4
never addressed are closed explicitly. First, child processes are started with
their working directory bound to the retained stage descriptor — the Foundry
temporarily changes its own working directory through the descriptor
(`fchdir`), starts the child without any pathname `Dir` field, and restores
the original working directory, all inside a serialized critical section — so
a pathname swap between validation and child start cannot redirect tool
execution (Section 34.4). Second, the Foundry registers a drained SIGPIPE
handler at startup so a broken stdout/stderr surfaces as an `EPIPE` write
error instead of killing the process, which is what makes the
commit-dominates-reporting exit contract enforceable; children spawned via
fork/exec retain default SIGPIPE behavior (Section 36.5).

Generation tests run uncached (`go test -count=1 -buildvcs=false`), and a
final complete non-`.git` path/type/mode/byte conformance check runs after
every external tool and after Git scratch removal, so no tool or generated test
can silently mutate the committed tree. The CGO ban is split: product and
release remain pure Go, while the race detector is an explicit
`CGO_ENABLED=1` development exception with compiler preflight, used by CI and
explicit maintainer verification rather than ordinary strict generation. The
`--offline` flag and every whole-process offline claim are deleted; generation
discloses its possible network steps before staging.

### 3.3 The capability framework is deleted (from FND-008, FND-009, FND-015)

Stage 4 shipped three initial profiles (`configuration`, `local-persistence`,
`distribution`), a generic composition engine (transitive requires, conflicts,
cycles, capability registry, exclusive/additive providers, helper-binary
schema, provenance closure, topological ordering), four rendering modes, and
seven typed structured-file generators.

The revision removes the `configuration` and `local-persistence` profiles from
the generated catalog because the Project Specification carries no application
intent from which useful configuration or persistence code can be generated;
their evidence is preserved as post-generation recipes (Section 21), and
`REQ-073`–`REQ-075` remain active requirements that normatively state the
exclusion and the recipe boundary rather than being retired. The MVP
implements no optional profile. `distribution` remains the only initial catalog
profile, implemented post-MVP, because it owns complete repository-level
release artifacts that do not depend on application domain fields. Profile
composition is now a flat exact-ID set with direct archetype/visibility
predicates. Rendering is reduced to static copy, restricted complete-file
templates, and typed `go.mod` generation; the limited-substitution token
language and the typed `.gitignore`/Dependabot/workflow/GoReleaser emitters are
deleted, and those files become complete owned static files or templates.

### 3.4 Generated output is minimal and its lifecycle is total (from FND-010, FND-014, FND-016, FND-017)

Stage 4 generated a `greet` demonstration service in every CLI and an
artificial asynchronous initialization sequence in every TUI, gave every
private project a heavy CI matrix, left TUI signal/effect/panic ownership
ambiguous, and opened debug logs unsafely.

The revision generates minimal compiling shells (CLI: root/help/version/
completion; TUI: one static help/quit screen), moving rich examples into
Foundry-owned integration fixtures. The TUI has exactly one lifecycle owner:
`main` owns SIGINT/SIGTERM through `signal.NotifyContext`, constructs the
program with `tea.WithContext(appCtx)` and `tea.WithoutSignalHandler()`, keeps
Bubble Tea's default panic recovery enabled, cancels the application context on
every quit path before returning `tea.Quit`, and requires every future effect
to be context-cooperative with observed completion in tests. The generated TUI
shell contains no `effects.go` or `messages.go`: a shell with no asynchronous
behavior gets no empty effect scaffolding, and the effect rules live in
`docs/ui-architecture.md` for the first real feature to follow. `--debug-log`
accepts only a safe basename (no separators, no traversal), resolves it
relative to a directory descriptor captured at startup, creates only a new
regular `0600` file with `O_CREATE|O_EXCL|O_NOFOLLOW`, and refuses every
existing or non-regular target. Generated private CI is reduced to one fast
required Linux job plus one scheduled/manual strict workflow.

### 3.5 Command, schema, result, and release semantics are corrected (from FND-012, FND-013, FND-019)

`schema = 1` is required but position-independent (Stage 4 required it to be
the "first semantic field" while also declaring field order irrelevant). Once
the exclusive commit succeeds, generation returns exit 0; a post-commit report
stream failure is best-effort diagnostic information and the JSON
"error with committed=true" state is deleted. The distribution profile no
longer states an impossible generation-time license precondition: generation
requires only public visibility and explicit selection, and the owner-added
license is a conspicuous post-generation publication prerequisite verified by
release dogfood, not by the generator.

### 3.6 Phasing and authority are no longer circular (from FND-011)

Stage 4 defined an implementation Phase 0 whose mandatory spikes could force
changes to the already-accepted final authority. The revision deletes
implementation Phase 0 and resolves the circularity without blocking work that
carries no filesystem risk. The foundational contracts — descriptor-relative
transaction primitives, namespace custody, stage preservation, descriptor-
bound child startup, exact Go/Git environments, and the SIGPIPE/output
contract — are fixed **in this document** with primary-source verification
(Section 11.3). The five bounded evidence items (descriptor-relative
transaction, exact Go/Git environment isolation, race prerequisites, Bubble
Tea lifecycle, and version/action lock) become **mandatory phase-entry
gates** (Section 51.2): E5 must be recorded before Phase 1 exit, E1–E3 before
any Phase 2 transaction or tool code is written, and E4 before Phase 3 TUI
work. Each gate verifies the architecture selected here; a contradicting
result forces a reviewed revision of this artifact before the gated phase
proceeds — it never authorizes implementation to improvise a pathname
fallback, automatic stage deletion, or a new subsystem. Because Phase 1 is
pure (no writes, tools, or network), it depends on no gate and is authorized
immediately.

### 3.7 What did not change

Product identity, locked decisions, the command surface, strict TOML
schema 1, the immutable Generation Plan, the Git-visible embedded catalog,
one-owner ordinary files, verification-before-placement with no bypass, narrow
Git behavior, no persistent provenance, macOS/Linux-only scope, generated
independence, the named dogfood projects, agent acceptance, and the
deferred/rejected work registers all survive intact (Section 6 lists the
preserved strengths explicitly).

## 4. Stage 5 Finding Disposition Ledger

Every Stage 5 finding is dispositioned here. `FND-###` identifiers appear only
in this ledger and the correction ledger (Section 5); findings are not
normative implementation requirements. "Blocking" reproduces the Stage 5
implementation-blocking status.

### FND-001 — Pathname preflight does not secure staging creation

| Field                 | Value                                                                                                                                                                                                                                                                              |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Severity / Blocking   | Critical / Blocks entire project                                                                                                                                                                                                                                                     |
| Disposition           | **Accepted**                                                                                                                                                                                                                                                                         |
| Rationale             | Verified TOCTOU defect: `Lstat` walk plus later `os.MkdirTemp(parentPath, ...)` re-resolves a mutable pathname; `os.OpenRoot` follows symlinks in its own path argument, so opening the stage as a root afterward is too late. The correction is required, narrow, and implementable. |
| Sections changed      | 15, 29, 31, 34, 43, 45, 46; Appendix A/C/D                                                                                                                                                                                                                                           |
| Requirements changed  | `REQ-044`, `REQ-124`, `REQ-125`, `REQ-129`, `REQ-154`, `REQ-184`, `REQ-213`, `REQ-220`                                                                                                                                                                                               |
| Verification added    | Barrier-controlled parent-swap race tests on macOS/Linux; static prohibition of path-based staging creation; identity verification of the rendering root against the retained handle; a namespace-custody check rejecting shared-writable non-sticky parent components; descriptor-bound child working directory so tool startup never re-resolves a pathname. Bounded evidence spike E1 is the Phase 2 entry gate. |
| Residual risk         | A privileged actor, or any principal already authorized to write the parent chain, mutating mounts/storage below the process-visible namespace remains out of scope; the custody check makes this trusted-host boundary explicit. Exact primitive shape is confirmed by the E1 record (`RSK-400`).                                                    |

### FND-002 — Path-based recursive cleanup can delete a swapped directory

| Field                 | Value                                                                                                                                                                                                                       |
| --------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Severity / Blocking   | Critical / Blocks entire project                                                                                                                                                                                              |
| Disposition           | **Accepted with modification** (stronger than the proposed remedy)                                                                                                                                                            |
| Rationale             | Recursive deletion is the highest-impact operation in the product; a prefix/inode check followed by path-based `RemoveAll` leaves a race that can delete unrelated data. The review proposed handle-relative cleanup; this revision removes the operation class entirely: no created stage is ever automatically deleted, so no deletion race can exist. |
| Sections changed      | 29, 31, 38, 43, 45, 46; Appendix D                                                                                                                                                                                            |
| Requirements changed  | `REQ-125`, `REQ-130`, `REQ-131`, `REQ-184`, `REQ-213`, `REQ-220`                                                                                                                                                              |
| Verification added    | A static audit that no production code path unlinks or recursively removes a created stage; hostile swap tests with surviving sentinels around every failure path; preservation reporting tests asserting the exact preserved location appears in text and JSON output.        |
| Residual risk         | Preserved stages accumulate as `0700` hidden debris until manually removed; the burden is tracked by `RSK-310` with an explicit revisit trigger (Section 55) before any reviewed deletion design could be reconsidered.       |

### FND-003 — Final-placement failure states contradict each other

| Field                 | Value                                                                                                                                                                                                          |
| --------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Severity / Blocking   | High / Blocks Phase 2                                                                                                                                                                                             |
| Disposition           | **Accepted**                                                                                                                                                                                                      |
| Rationale             | Stage 4 simultaneously required preserving and cleaning a losing stage and let cancellation timing change outcomes. Totality requires one commit-result state machine.                                            |
| Sections changed      | 29, 31.8, 31.9, 38; Appendix D                                                                                                                                                                                    |
| Requirements changed  | `REQ-129`, `REQ-130`, `REQ-131`, `REQ-158`                                                                                                                                                                        |
| Verification added    | Table-driven state-machine test injecting every commit result with and without concurrent cancellation; real two-process `EEXIST` races on macOS and Linux asserting exact winner/loser/exit/stage states; post-syscall identity classification tests including injected ambiguous rename acknowledgments. |
| Residual risk         | The losing stage is preserved by design (Section 31); a contradictory identity observation stops all further mutation and reports ambiguity rather than guessing.                                                 |

### FND-004 — Strict race verification conflicts with the global CGO ban

| Field                 | Value                                                                                                                                                                                                                 |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Severity / Blocking   | High / Blocks Phase 2                                                                                                                                                                                                     |
| Disposition           | **Accepted**                                                                                                                                                                                                              |
| Rationale             | The Go race detector requires cgo and a C compiler on the supported platforms; a globally forced `CGO_ENABLED=0` makes the Stage 4 strict gate unexecutable. `DEC-013` bans CGO in the product, not in dev instrumentation. |
| Sections changed      | 12, 32, 34, 35, 45, 47; Appendix D/E                                                                                                                                                                                      |
| Requirements changed  | `REQ-004`, `REQ-135`, `REQ-151`, `REQ-154`, `REQ-217`, `REQ-222`, `REQ-223`                                                                                                                                               |
| Verification added    | Compiler-preflighted race jobs in Foundry and generated scheduled CI; a known-race fixture proving instrumentation is active; release-build matrix proving `CGO_ENABLED=0` artifacts.                                     |
| Residual risk         | Race instrumentation covers only executed paths; it is evidence, not proof.                                                                                                                                               |

### FND-005 — External-tool boundary permits undeclared helpers and Git templates

| Field                 | Value                                                                                                                                                                                                                                            |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Severity / Blocking   | High / Blocks Phase 2                                                                                                                                                                                                                                 |
| Disposition           | **Accepted**                                                                                                                                                                                                                                          |
| Rationale             | Verified: `cmd/go` honors `GOENV`, `GOFLAGS`, `GOCACHEPROG`, `GOAUTH`, `GOVCS`, and toolchain switching; `git init` copies host template directories and reads system/global config. Executable allowlisting without configuration isolation is false. |
| Sections changed      | 32, 34, 39, 45, 46; Appendix D/E                                                                                                                                                                                                                      |
| Requirements changed  | `REQ-135`, `REQ-153`, `REQ-154`, `REQ-160`, `REQ-185`, `REQ-214`, `REQ-220`, `REQ-221`                                                                                                                                                                |
| Verification added    | Hostile fixtures setting every relevant Go/Git environment/config/template variable to sentinel helpers, asserting no sentinel executes and no template file is copied; process-tree comparison against the plan's external-step list.                |
| Residual risk         | The trusted `go`/`git` binaries and the race-check C compiler remain part of the local trust base.                                                                                                                                                    |

### FND-006 — Verification can pollute staging after the last conformance check

| Field                 | Value                                                                                                                                                                                                        |
| --------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Severity / Blocking   | High / Blocks Phase 2                                                                                                                                                                                             |
| Disposition           | **Accepted**                                                                                                                                                                                                      |
| Rationale             | Generated tests run with write access to staging and Go caches successful test results; without `-count=1` and a final tree comparison, committed bytes can differ from planned bytes and from run to run.        |
| Sections changed      | 28, 29, 32, 35; Appendix C/D                                                                                                                                                                                      |
| Requirements changed  | `REQ-121`, `REQ-126`, `REQ-127`, `REQ-150`, `REQ-151`, `REQ-212`, `REQ-215`                                                                                                                                       |
| Verification added    | A deliberately mutating generated test must fail generation with new error `verify.unplanned_mutation`; a cached-success test changed to fail must still execute; repeated warm/cold generations byte-compare.    |
| Residual risk         | Tests can still have side effects outside staging; the guarantee is that they cannot alter the committed repository. Whole-host sandboxing is not claimed.                                                        |

### FND-007 — `--offline` does not enforce offline execution

| Field                 | Value                                                                                                                                                                     |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Severity / Blocking   | High / Blocks Phase 2                                                                                                                                                       |
| Disposition           | **Accepted**                                                                                                                                                                |
| Rationale             | `GOPROXY=off` controls only the module downloader; generated tests and other tool behavior can still use the network. A flag named `--offline` asserts an unenforced property. |
| Sections changed      | 13, 14, 28, 30, 32, 34, 35, 37; all examples                                                                                                                                |
| Requirements changed  | `REQ-034`, `REQ-121`, `REQ-135`, `REQ-154`, `REQ-156`, `REQ-214`, `REQ-223`, `REQ-243`                                                                                      |
| Verification added    | Help/JSON/plan schemas and acceptance criteria contain no `offline` claim; write-free commands tested on network-disabled hosts; generate reports planned network-capable steps before staging. |
| Residual risk         | Generated tests may access the network; the plan discloses only Foundry/tool network intent.                                                                                |

### FND-008 — Configuration and persistence profiles are under-specified application scaffolds

| Field                 | Value                                                                                                                                                                                                                          |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Severity / Blocking   | High / Blocks Phases 3–4                                                                                                                                                                                                          |
| Disposition           | **Accepted**                                                                                                                                                                                                                      |
| Rationale             | The Project Specification carries no application configuration/persistence intent; both profiles could emit only invented or empty scaffolds users must replace, and the configuration `--config` flag contradicted the TUI startup contract. |
| Sections changed      | 2, 9, 14, 19, 20, 21, 22, 23, 24, 50, 51; Appendix A/B/F                                                                                                                                                                          |
| Requirements changed  | `REQ-011`, `REQ-045`, `REQ-077`, `REQ-078`, `REQ-216`, `REQ-243`, `REQ-244`; `REQ-073`–`REQ-075` remain active with the same subjects, now normatively stating the exclusion and the recipe boundary                              |
| Verification added    | Schema-1 catalog lists only `cli`, `tui`, and (post-MVP) `distribution`; MVP acceptance runs with `profiles = []`; dogfood measures retained manual configuration/persistence patterns as evidence for any future profile.        |
| Residual risk         | Early projects repeat some manual config/persistence work; that repetition is deliberate evidence collection (`RSK-401`).                                                                                                          |

### FND-009 — Schema-1 profile composition implements an unused framework

| Field                 | Value                                                                                                                                                                                   |
| --------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Severity / Blocking   | High / Blocks Phase 1                                                                                                                                                                       |
| Disposition           | **Accepted**                                                                                                                                                                                |
| Rationale             | No initial profile requires, conflicts with, or provides an alternative to another, and no helper binary exists; the DAG/capability/helper machinery is speculative framework prohibited by the Charter's burden-of-proof rule. |
| Sections changed      | 19, 23, 24, 26, 27, 28, 42, 43, 45; Appendix C/G                                                                                                                                            |
| Requirements changed  | `REQ-077`, `REQ-092`, `REQ-098`, `REQ-099`, `REQ-100`, `REQ-101`, `REQ-121`, `REQ-183`, `REQ-187`, `REQ-216`, `REQ-241`                                                                     |
| Verification added    | Catalog/plan schemas contain no `requires`/`conflicts`/`capabilities`/`provider`/`helper_binary`/provenance fields; flat resolver tests replace graph property suites.                      |
| Residual risk         | A future profile needing composition semantics triggers an explicit pre-1.0 schema revision; no generated project depends on the internal schema.                                           |

### FND-010 — TUI lifecycle has multiple signal owners and unjoined effects

| Field                 | Value                                                                                                                                                                                                                |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Severity / Blocking   | High / Blocks Phase 3                                                                                                                                                                                                    |
| Disposition           | **Accepted**                                                                                                                                                                                                             |
| Rationale             | Verified against Bubble Tea v2: without `tea.WithContext` and `tea.WithoutSignalHandler()` there are two signal owners; command goroutines are not joined by the framework; a top-level panic guard cannot catch them.    |
| Sections changed      | 18, 45, 46; Appendix A                                                                                                                                                                                                   |
| Requirements changed  | `REQ-036`, `REQ-068`, `REQ-069`, `REQ-070`, `REQ-071`, `REQ-072`, `REQ-188`, `REQ-215`                                                                                                                                    |
| Verification added    | Effect start/finish tracking across `q`/Ctrl-C/SIGINT/SIGTERM/startup failure/effect error paths asserting zero active effects at return; PTY restoration for quit, signal, and framework-recovered panic; static check for `WithContext`/`WithoutSignalHandler` and absence of `WithoutCatchPanics`. Bounded lifecycle spike E4 is the Phase 3 entry gate. |
| Residual risk         | Owners can later add non-cooperative effects manually; generated docs/tests make the rule explicit but cannot prevent post-generation violations.                                                                        |

### FND-011 — Phase 0 creates a circular specification-authority gate

| Field                 | Value                                                                                                                                                                                                              |
| --------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Severity / Blocking   | High / Blocks entire project                                                                                                                                                                                            |
| Disposition           | **Accepted with modification** (evidence gates at phase entries instead of a global acceptance block)                                                                                                                   |
| Rationale             | A final implementation authority cannot depend on a post-acceptance implementation phase empowered to rewrite it. The circularity is removed by fixing every foundational contract in this document and demoting the spikes to verification: each evidence item gates entry to the first phase that depends on it, and a contradicting result forces a reviewed artifact revision, never implementation-time improvisation. Blocking pure planning (which has no filesystem, tool, or network dependence) on a filesystem spike would be ceremony, not safety. |
| Sections changed      | 1, 3.6, 50, 51, 56, 60; Appendix F/I                                                                                                                                                                                    |
| Requirements changed  | `REQ-240`, `REQ-248`; phase allocations of all `REQ-###` previously assigned to "Phase 0"                                                                                                                               |
| Verification added    | The phase table starts at pure planning and contains no phase authorized to amend the specification; Section 51.2 defines the exact gate-to-phase mapping, the evidence-record format, and the rule that no gate may be waived or silently reinterpreted.                                                          |
| Residual risk         | Implementation can still reveal ordinary defects, handled through normal artifact revision; no known load-bearing uncertainty is scheduled after the phase that depends on it.                                           |

### FND-012 — Post-commit reporting failure is misclassified as generation failure

| Field                 | Value                                                                                                                                                                 |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Severity / Blocking   | High / Blocks Phase 2                                                                                                                                                      |
| Disposition           | **Accepted**                                                                                                                                                               |
| Rationale             | After the irreversible exclusive rename, exit 1 invites a retry that must fail with `fs.destination_exists`; exit status must describe the filesystem transaction, not the output pipe. The contract is made mechanically enforceable: the Foundry registers a drained SIGPIPE handler at startup so a broken pipe surfaces as `EPIPE` instead of killing the process before it can exit 0. |
| Sections changed      | 29, 30, 36, 37, 38                                                                                                                                                         |
| Requirements changed  | `REQ-123`, `REQ-129`, `REQ-155`, `REQ-156`, `REQ-158`, `REQ-159`                                                                                                           |
| Verification added    | Real broken-pipe process tests (not only injected writer errors) immediately before and after commit: before commit blocks placement and preserves any created stage; after commit exits 0 with best-effort fallback diagnostics; JSON schema contains no `status=error` with a committed repository; child tools are proven to retain default SIGPIPE disposition. |
| Residual risk         | A caller whose output channel fails may not receive the destination path; the explicit destination in the specification remains authoritative.                             |

### FND-013 — Project Specification both requires and ignores field order

| Field                 | Value                                                                                                            |
| --------------------- | ------------------------------------------------------------------------------------------------------------------ |
| Severity / Blocking   | Medium / Non-blocking                                                                                                |
| Disposition           | **Accepted**                                                                                                         |
| Rationale             | A readability convention was converted into contradictory semantics; strict decoding validates schema regardless of position. |
| Sections changed      | 14.2; Appendix B                                                                                                     |
| Requirements changed  | `REQ-038`, `REQ-039`, `REQ-040`                                                                                      |
| Verification added    | Permutation tests placing `schema` before, between, and after other fields produce identical normalized specifications and plans. |
| Residual risk         | Examples still place `schema` first; documentation labels this style, not validity.                                  |

### FND-014 — Generated demo features pollute every new project

| Field                 | Value                                                                                                                                                     |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Severity / Blocking   | Medium / Non-blocking                                                                                                                                          |
| Disposition           | **Accepted**                                                                                                                                                   |
| Rationale             | The `greet` service and synthetic TUI initialization are Foundry tests disguised as user code; they distort deletion metrics and agent authority perception.   |
| Sections changed      | 16, 17, 18, 45, 52; Appendix A                                                                                                                                 |
| Requirements changed  | `REQ-060`, `REQ-064`, `REQ-065`, `REQ-066`, `REQ-068`, `REQ-069`, `REQ-072`, `REQ-215`, `REQ-242`, `REQ-243`, `REQ-247`                                        |
| Verification added    | Fresh CLI/TUI shells compile, run, and pass tests with no fake domain output; Foundry integration fixtures separately prove the extension paths; dogfood records zero mandatory demo deletion. |
| Residual risk         | Minimal shells are less visually impressive; real dogfood applications supply the compelling behavior.                                                         |

### FND-015 — Rendering and structured generation have become a mini-framework

| Field                 | Value                                                                                                                                                   |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Severity / Blocking   | Medium / Non-blocking                                                                                                                                        |
| Disposition           | **Accepted**                                                                                                                                                 |
| Rationale             | Only `go.mod` has multiple semantic contributors; the limited-substitution language duplicated templates and the typed YAML/GoReleaser emitters had single owners with no merge need. |
| Sections changed      | 24, 25, 26, 28, 42, 43; Appendix C                                                                                                                           |
| Requirements changed  | `REQ-092`, `REQ-093`, `REQ-095`, `REQ-096`, `REQ-097`, `REQ-121`, `REQ-184`, `REQ-212`                                                                       |
| Verification added    | Plan/render schemas expose only `static`, `template`, and `gomod` strategies; generated workflow and GoReleaser files parse under their native tools; no custom token scanner exists. |
| Residual risk         | Complete templates may duplicate small CI structure between archetypes (`RSK-402`); files remain visible and independently testable.                         |

### FND-016 — Generated Core CI is disproportionate for private personal tools

| Field                 | Value                                                                                                                                     |
| --------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| Severity / Blocking   | Medium / Non-blocking                                                                                                                           |
| Disposition           | **Accepted**                                                                                                                                    |
| Rationale             | Every-PR macOS/race jobs plus weekly fuzz/cold checks in every ten-file personal tool violate the Charter's fast-feedback and proportionality standards; ignored gates are ceremonial, not strict. |
| Sections changed      | 16.6, 45, 47; Appendix E                                                                                                                        |
| Requirements changed  | `REQ-062`, `REQ-063`, `REQ-151`, `REQ-217`, `REQ-223`, `REQ-243`, `REQ-247`                                                                     |
| Verification added    | A minimal generated private CLI has exactly one required PR job and one scheduled/manual strict workflow; dogfood measures median warm PR latency and escaped-defect timing. |
| Residual risk         | Some macOS/race/vulnerability defects surface after merge rather than before (`RSK-403`); risk-based promotion of scheduled checks is documented. |

### FND-017 — TUI debug-log creation is not symlink-/overwrite-safe

| Field                 | Value                                                                                                           |
| --------------------- | ------------------------------------------------------------------------------------------------------------------ |
| Severity / Blocking   | Medium / Non-blocking                                                                                                |
| Disposition           | **Accepted with modification** (stronger than the proposed remedy)                                                   |
| Rationale             | Mode 0600 controls creation permissions only; ordinary create/truncate flags follow symlinks and truncate existing files. Beyond exclusive creation, the flag is narrowed to a safe basename resolved relative to a directory descriptor captured at startup, eliminating parent-path traversal and working-directory races entirely. |
| Sections changed      | 18.8, 46                                                                                                             |
| Requirements changed  | `REQ-071`, `REQ-072`, `REQ-159`, `REQ-215`, `REQ-221`                                                                |
| Verification added    | Startup matrix covering new file, existing regular file, symlink, directory, FIFO/socket where portable, path separators and traversal sequences in the argument, stdout/stderr aliases, mode, redaction; only a new regular file from a safe basename succeeds. |
| Residual risk         | The generated tool is not a sandbox against its own administrator; the open descriptor continues to identify the created file. |

### FND-018 — Synthetic case-fold collision checks overreach the filesystem contract

| Field                 | Value                                                                                                                                            |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| Severity / Blocking   | Medium / Non-blocking                                                                                                                                  |
| Disposition           | **Accepted**                                                                                                                                           |
| Rationale             | The scan rejects valid names on case-sensitive volumes, cannot model every filesystem normalization rule, and is still racy; native lookup plus the exclusive commit is authoritative. Accepted despite the review's Medium confidence because the deletion is safe and simplifying. |
| Sections changed      | 15, 31; Appendix D                                                                                                                                     |
| Requirements changed  | `REQ-044`, `REQ-124`, `REQ-134`, `REQ-213`                                                                                                             |
| Verification added    | Tests on representative case-sensitive and case-insensitive volumes assert native exact-name behavior and no destination replacement; no case-fold scan remains in transaction code. |
| Residual risk         | Copying a repository between differently-folding filesystems is ordinary cross-filesystem behavior outside one generation transaction.                 |

### FND-019 — Distribution treats an impossible license precondition as part of generation

| Field                 | Value                                                                                                                                       |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| Severity / Blocking   | Medium / Non-blocking                                                                                                                              |
| Disposition           | **Accepted**                                                                                                                                       |
| Rationale             | The destination is absent at generation time, so a "license has been added manually" selection criterion is unverifiable; scaffolding generation and publication readiness are distinct events. |
| Sections changed      | 20, 47, 48; Appendix B                                                                                                                             |
| Requirements changed  | `REQ-076`, `REQ-078`, `REQ-164`, `REQ-224`, `REQ-244`                                                                                              |
| Verification added    | A new public project with distribution generates successfully without a license file; generated release docs conspicuously block publication until the owner adds/reviews a license; release dogfood verifies the license before tag publication. |
| Residual risk         | The owner may still choose an inappropriate license; the Foundry gives no legal advice and never claims the repository is open source.              |

### 4.1 Disposition Totals

| Disposition                        | Count | Findings                                          |
| ---------------------------------- | ----: | ------------------------------------------------- |
| Accepted                           |    16 | All findings except those listed below            |
| Accepted with modification         |     3 | FND-002, FND-011, FND-017 (strengthened remedies) |
| Rejected                           |     0 | —                                                 |
| Deferred to bounded evidence spike |     0 | —                                                 |
| Not applicable                     |     0 | —                                                 |

Every modification strengthens the review's proposed remedy rather than
weakening it: FND-002's cleanup hardening becomes complete deletion of
automatic stage deletion, FND-011's pre-acceptance evidence becomes
non-waivable phase-entry gates, and FND-017's exclusive-create fix becomes a
descriptor-relative safe-basename contract.

Note: `OQ-400` is an open question, not a finding. The corrections for
`FND-001`/`FND-002` are fully integrated into this specification's normative
text; `OQ-400` requires only the executable macOS/Linux evidence that confirms
the selected primitives, recorded as the E1 Phase 2 entry gate (Sections 51.2
and 56).

## 5. Integrated Correction Ledger

This ledger maps accepted findings to the revised sections, requirements,
phases, tests, and removed machinery, in the coordinated groups the review
identified as sharing root causes.

| Correction group                          | Findings                               | Revised sections                          | Modified/retired requirements                                                                                                        | Modified phases                                                             | New or modified tests                                                                                                                       | Removed machinery                                                                                                              |
| ----------------------------------------- | -------------------------------------- | ----------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| Descriptor-owned transaction              | FND-001, FND-002, FND-003, FND-018     | 15, 28, 29, 31, 34, 43, 45, 46; App. C/D  | `REQ-044`, `REQ-124`, `REQ-125`, `REQ-129`–`REQ-131`, `REQ-134`, `REQ-154`, `REQ-158`, `REQ-184`, `REQ-213`, `REQ-220`                 | E1 Phase 2 entry gate; Phase 2                                               | Parent/stage object-swap races; namespace-custody tests; no-stage-delete static audit with surviving sentinels; commit-result state-machine table with identity classification; two-process `EEXIST` races; descriptor-bound child-cwd swap tests; native case tests | Path-based `MkdirTemp`/`RemoveAll` in transactions; **all automatic stage deletion**; pathname child working directories; Unicode case-fold sibling scan; contradictory preserve/clean prose |
| Closed tool execution and truthful output | FND-004, FND-005, FND-006, FND-007, FND-012 | 12, 13, 28, 29, 30, 32, 34, 35, 36, 37, 38, 39 | `REQ-004`, `REQ-034`, `REQ-121`, `REQ-123`, `REQ-126`, `REQ-127`, `REQ-135`, `REQ-150`–`REQ-160`, `REQ-185`, `REQ-214`, `REQ-217`, `REQ-222`, `REQ-223` | Phase 2                                                                       | Hostile Go/Git env/template sentinel tests; cached-test and mutation fixtures; final-conformance tests; pre/post-commit stream-failure injection; compiler-preflighted race jobs | `--offline` flag and fields; committed-error JSON state; "narrowly filtered host environment" prose; global CGO=0-with-race contradiction |
| Framework and demo deletion               | FND-008, FND-009, FND-014, FND-015     | 2, 9, 14, 16–28, 42, 43, 45, 50; App. A–C/G | `REQ-011`, `REQ-045`, `REQ-060`, `REQ-064`–`REQ-072`, `REQ-076`–`REQ-078`, `REQ-092`–`REQ-101`, `REQ-121`, `REQ-183`, `REQ-187`, `REQ-212`, `REQ-215`, `REQ-216`, `REQ-241`–`REQ-244`, `REQ-247`; `REQ-073`–`REQ-075` recast as exclusion-and-recipe requirements | Phases 1–4 (MVP has no profiles; distribution moves wholly post-MVP)          | Flat resolver tests; minimal-shell generation tests; fixture-based extension-path tests; distribution snapshots post-MVP; deletion-rate dogfood metrics | Configuration/persistence profiles; profile DAG/capabilities/helper schema/provenance; limited substitution; typed YAML/GoReleaser/`.gitignore`/Dependabot/workflow emitters; `greet`; synthetic TUI init |
| One-owner lifecycles and authority        | FND-010, FND-011, FND-017              | 1, 3.6, 18, 46, 50, 51, 56, 60; App. F/I  | `REQ-036`, `REQ-068`–`REQ-072`, `REQ-159`, `REQ-188`, `REQ-215`, `REQ-221`, `REQ-240`, `REQ-248`                                       | Implementation Phase 0 deleted; phase-entry evidence gates added; Phase 3    | Effect-completion tracking; single-signal-owner assertions; PTY quit/signal/panic restoration; safe-basename debug-log exclusive-create matrix | Implementation Phase 0; top-level cross-goroutine panic guard claim; second signal owner; unsafe debug-log open; generated empty effect/message scaffolding |
| Proportionality and consistency           | FND-013, FND-016, FND-019              | 14.2, 16.6, 20, 45, 47, 48; App. B/E      | `REQ-038`–`REQ-040`, `REQ-062`, `REQ-063`, `REQ-076`, `REQ-151`, `REQ-217`, `REQ-223`, `REQ-243`, `REQ-244`                            | Phase 3 (reduced CI), Phase 4 (license timing)                                | Schema-position permutation tests; single-required-job CI assertions; PR-latency dogfood measurement; license-free distribution generation test | "First semantic field" rule; every-PR macOS/race jobs in generated Core; generation-time license precondition                    |

## 6. Preserved Strengths

The revision intentionally preserves the following Stage 4 properties, so
corrective edits must not weaken them:

- **New projects only.** No merge, overwrite, adopt, update, migration, or
  synchronization behavior; the destination must not exist, even empty.
- **Exactly one Project Archetype** (`cli` or `tui`), one primary interaction
  model, and one primary binary per Generated Project.
- **Strict non-interactive input and a write-free inspectable plan.** `plan`
  is the authoritative dry run; there is no separate `dry-run`,
  `completion`, `publish`, or recovery command. Optional `init` writes only a
  Project Spec TOML to an explicit `--out` (flags only, no prompts). Optional
  `doctor` is an advisory toolchain probe.
- **Git-visible embedded catalog** with no runtime plugins, remote catalogs,
  user templates, hooks, or profile scripts; development loading remains
  build-tag restricted.
- **One-owner ordinary files** and fatal, fully-attributed collisions,
  including byte-identical duplicates.
- **Conventional Go everywhere:** thin `cmd/` entrypoints, `internal/`
  packages, no generic buckets, no DI framework, no task runner, wrapped
  errors with boundary-level exit mapping, `slog` as the only logging API.
- **Verification before final placement with no bypass.** `default` and
  `strict` are the only verification modes; `none` does not exist.
- **Narrow Git behavior:** optional isolated `git init -b <branch>` only; no
  staging, commit, identity, remotes, hooks, GitHub API calls, repository
  creation, push, tag, or release.
- **No persistent provenance** in Generated Projects; the plan and command
  result carry event identity; the project is authoritative after generation.
- **Bounded determinism:** byte-identical output within the explicit
  Foundry/catalog/toolchain/platform envelope, with `.git`, mtimes, and
  staging names excluded.
- **Portable root `AGENTS.md`** as the single generated instruction authority;
  no nested or vendor-specific instruction files; no Claude-specific
  conventions.
- **Early real dogfooding** with named repositories (`repo-map`,
  `worktree-status`) and measured deletion, latency, and agent-acceptance
  metrics.
- **Small exact dependency bill** with exact pins, pure-Go runtime, Dependabot,
  govulncheck, and an explicit vulnerability response path.
- **macOS and Linux only; Windows excluded.** ASCII kebab-case project names,
  LF line endings, explicit 0644/0755 modes.

## 7. Authority, Status, and Intended Use

### 7.1 Position in the Program

This is the Stage 6 output of the Go Foundry Research Program and the final
design artifact of the program's research workflow. It supersedes the Stage 4
specification in full and integrates the Stage 5 adversarial review.

### 7.2 Artifact Precedence

For any question about the Foundry, authority applies in this order:

1. Accepted superseding decision records (`DEC-*.md`; none currently exist
   beyond the Blueprint set).
2. Locked Program Blueprint decisions `DEC-001`–`DEC-016`.
3. Normative Research Charter requirements.
4. **This revised definitive specification.**
5. The Stage 5 adversarial review (context for the revision).
6. The Stage 1–3 research reports (evidence).
7. The Stage 4 specification (superseded history).
8. Prompts, the Blueprint's non-normative narrative, and informal notes.

### 7.3 Status Semantics

The current status is `Accepted — implementation authority`. This artifact is
complete as a design: every Stage 5 finding is dispositioned and integrated,
no known contradiction remains in the text, and the implementation phases are
executable. The acceptance is honest about what has and has not been executed:

- Every foundational contract (transaction primitives, custody, preservation,
  child startup, tool environments, SIGPIPE/output, lifecycle, versions) is
  fixed in this document, supported by the primary-source verification listed
  in Section 11.3. No executable spike result is claimed or fabricated here.
- The five bounded evidence items in Section 51.2 remain to be executed. They
  are **non-waivable phase-entry gates**: E5 gates Phase 1 exit, E1–E3 gate
  Phase 2 entry, and E4 gates Phase 3 entry. Each produces a short recorded
  evidence artifact through the program's artifact workflow.
- If any evidence record contradicts a normative contract here, the
  contradiction is resolved by a minimal reviewed revision of this artifact
  before the gated phase proceeds — never by implementation-time
  improvisation, a pathname fallback, automatic stage deletion, or a new
  subsystem.
- Phase 1 (pure specification, catalog, resolution, planning — no writes,
  tools, or network) is authorized immediately because it depends on no
  ungated evidence.

### 7.4 Normative Language

`MUST`, `MUST NOT`, `SHOULD`, `SHOULD NOT`, and `MAY` are used per RFC 2119
intent. Examples, trees, and illustrative JSON are not requirements unless a
requirement references them as normative.

## 8. Product Definition

### 8.1 Problem

Starting a new Go CLI or TUI tool repeatedly consumes the same foundational
decisions: repository layout, framework selection, error/exit conventions,
testing layers, CI, security posture, agent instructions, and release
mechanics. Repeating them by hand is slow and inconsistent; copying an old
repository propagates stale decisions.

### 8.2 Product

The **Foundry** is a local, deterministic, non-interactive project compiler.
It reads a declarative **Project Specification** (`foundry.toml`), resolves it
against an embedded versioned **Source Catalog**, produces an immutable
inspectable **Generation Plan**, and — only in `generate` — renders, verifies,
optionally Git-initializes, and atomically places a complete new repository.

### 8.3 User

Robert Guss, personally. The Foundry is a personal tool first (`DEC-002`).
Design choices favor one expert owner, his AI agents, and his machines over
hypothetical team or platform needs.

### 8.4 Typical Generated Projects

Small-to-medium local developer tools: repository inventory CLIs, Git worktree
dashboards, log filters, personal automation, and occasionally a deliberately
public open-source CLI.

### 8.5 Platforms

macOS and Linux on amd64/arm64. Windows is out of scope and must not distort
Unix-first design. The Foundry, Generated Projects, and all initial catalog
content are pure Go with `CGO_ENABLED=0` for product and release builds
(development race instrumentation is the sole, explicit exception —
Section 34.4).

### 8.6 Agents

Grok Build is the primary agent target; Codex and Cursor are first-class
secondary targets. Generated instruction files are portable (`AGENTS.md`
only); no Claude-specific files or conventions are generated.

## 9. Goals and Non-Goals

### 9.1 Goals

1. Generate complete, production-quality, immediately-runnable Go CLI and TUI
   repositories in one non-interactive command.
2. Make every generated repository fast for agents and humans to comprehend:
   conventional layout, concise authoritative docs, direct commands.
3. Guarantee byte-determinism within the declared envelope and full plan
   inspectability before any write.
4. Guarantee filesystem safety: no write outside the owned transaction, no
   automatic deletion of anything (including the Foundry's own created
   stages), no destination replacement, total failure/preservation paths.
5. Verify generated projects (format, tidy, module verify, uncached tests,
   vet; strict adds Staticcheck and govulncheck) before placement, with no
   bypass.
6. Keep the Core minimal, the initial profile catalog small (one profile,
   post-MVP), and every abstraction justified by real recurring use.
7. Begin dogfooding with real tools early enough to change architecture
   cheaply.

### 9.2 Non-Goals

The Foundry does not and will not (within this specification's authority):

- modify, upgrade, migrate, adopt, or synchronize existing projects;
- support plugins, remote catalogs, user-supplied templates, profile scripts,
  hooks, or a marketplace;
- create GitHub repositories, remotes, commits, tags, pushes, or releases;
- select or generate a license;
- claim whole-process network isolation or offer an `--offline` mode;
- provide interactive prompts or a wizard;
- persist Foundry facts, provenance files, or upgrade markers in output;
- support Windows;
- generate application domain code (configuration structs, persistence
  wrappers, HTTP clients) it cannot know the semantics of;
- self-update.

## 10. Locked Decisions and Global Invariants

The Program Blueprint's locked decisions remain fully in force. Their primary
implementing requirements:

| Locked decision                              | Implementing requirements                          |
| -------------------------------------------- | -------------------------------------------------- |
| `DEC-001` Go-native direction                | `REQ-002`, `REQ-180`                               |
| `DEC-002` Personal tool first                | `REQ-001`, `REQ-012`, `REQ-078`                    |
| `DEC-003` New projects only                  | `REQ-003`, `REQ-044`, `REQ-124`, `REQ-129`         |
| `DEC-004` Git-native, repository-first       | `REQ-007`, `REQ-090`, `REQ-160`                    |
| `DEC-005` Non-interactive first              | `REQ-007`, `REQ-030`, `REQ-036`                    |
| `DEC-006` Minimal Core, profile capabilities | `REQ-011`, `REQ-060`, `REQ-077`, `REQ-078`         |
| `DEC-007` Exactly one archetype              | `REQ-005`                                          |
| `DEC-008` One interaction model              | `REQ-005`, `REQ-068`                               |
| `DEC-009` One primary binary                 | `REQ-005`, `REQ-042`                               |
| `DEC-010` GitHub standardization             | `REQ-008`, `REQ-063`, `REQ-076`                    |
| `DEC-011` Private and public projects        | `REQ-045`, `REQ-063`, `REQ-076`                    |
| `DEC-012` Strict but pragmatic quality       | `REQ-010`, `REQ-150`–`REQ-152`, `REQ-223`          |
| `DEC-013` CGO avoided by default             | `REQ-004`, `REQ-135`                               |
| `DEC-014` Decisive outcomes                  | This artifact's single selected architecture       |
| `DEC-015` Conventional Go over frameworks    | `REQ-006`, `REQ-064`, `REQ-068`, `REQ-096`, `REQ-187` |
| `DEC-016` Artifact-oriented workflow         | `REQ-240`, `REQ-248`                               |

Global invariants (each restated normatively later):

1. Pure interpretation precedes side effects: parse → validate → resolve →
   plan are side-effect free; only `generate` mutates the filesystem.
2. Exactly one archetype and one primary binary per Generated Project; the
   schema reserves no helper-binary machinery.
3. The catalog is fixed at build time; there is no user extension surface.
4. Verification cannot be bypassed.
5. Every transaction mutation is descriptor-relative to retained handles;
   child tool processes start from the retained stage descriptor, never a
   re-resolved pathname.
6. Generated Projects have zero dependency on the Foundry.
7. The commit result dominates exit status; nothing after a successful commit
   can convert success into failure. The Foundry catches SIGPIPE so a broken
   output stream cannot kill the process before it reports truthfully.
8. **No created stage is ever automatically deleted.** The Foundry contains
   no stage unlink, recursive removal, scavenger, or cleanup command; every
   uncommitted created stage is preserved and its exact location reported.
9. Destination-parent custody is checked explicitly: shared-writable
   non-sticky parent components are rejected, and actors already authorized
   to write the parent chain are an explicit trusted-host boundary, not a
   defended threat.

## 11. Revision Method and Evidence

### 11.1 Inputs

The complete Stage 4 specification, the complete Stage 5 adversarial review,
the Program Blueprint, the Research Charter, `README.md`, `AGENTS.md`, the
three research reports, and the Stage 5 review prompt were read in full. No
separate accepted decision records were supplied.

### 11.2 Method

Every Stage 5 finding was dispositioned individually (Section 4), then
integrated as five coordinated correction groups (Section 5) rather than as
isolated patches, following the review's cross-finding interaction analysis.
Where the review's per-finding diffs and consolidated diff differed in detail,
the correction was derived from the finding's problem statement and failure
scenario, preferring the smallest coherent revision. Deletion was preferred
over new machinery throughout: this revision removes two profiles, the profile
composition engine, two rendering modes, six typed emitters, one command flag,
one JSON state, one synthetic namespace policy, two generated demo features,
one implementation phase, and — going beyond the review's remedy — the entire
automatic stage-deletion operation class, while adding no new subsystem beyond
the descriptor-relative transaction contract that replaces the unsafe pathname
contract.

### 11.3 Targeted verification

Narrow primary-source verification performed for this revision on 2026-07-30
(citations in Section 61):

- **[SV-01]** Go 1.26 `os.Root` provides the descriptor-relative method set
  the transaction uses (`Mkdir`, `OpenFile`, `Lstat`, `Remove`, `RemoveAll`,
  `OpenRoot`, `Rename`), resolving names within the root without following
  escaping symlinks; and Go 1.26.5 fixes CVE-2026-39822, a `Root` symlink
  escape via trailing slash, which makes the exact patch release load-bearing.
- **[SV-02]** `golang.org/x/sys/unix` exposes dirfd-relative exclusive
  no-replace rename on both supported platforms: `Renameat2(...,
  RENAME_NOREPLACE)` on Linux and `RenameatxNp(..., RENAME_EXCL)` on Darwin,
  where `RENAME_EXCL` returns `EEXIST` when the destination exists on
  supporting filesystems (APFS). Darwin additionally exposes
  `RENAME_NOFOLLOW_ANY`, combined with `RENAME_EXCL` so no component of
  either rename path may be a symbolic link.
- **[SV-03]** POSIX/Linux `openat`-family semantics: a directory descriptor
  continues to identify the same filesystem object after any pathname rename
  or replacement, which is the object-continuity property the retained-handle
  transaction depends on; `fchdir` changes the process working directory
  through a descriptor without pathname resolution and is available on both
  supported platforms.
- **[SV-04]** Go runtime SIGPIPE semantics (`os/signal` documentation): a
  write to a broken fd 1 or fd 2 raises SIGPIPE that kills the process
  unless the program subscribes with `signal.Notify(..., SIGPIPE)`, after
  which writes return `EPIPE`; fork/exec resets signal handlers so child
  processes retain default SIGPIPE disposition. This is the mechanism behind
  the commit-dominant output contract (Section 36.5).
- **[SV-05]** Unix sticky-bit semantics: within a mode `1777` directory,
  rename/unlink of another user's entry is refused, which bounds what the
  namespace-custody check (Section 31.3) must reject versus may allow.

The Stage 5 review's own targeted verifications (Go race detector cgo
requirement, `cmd/go` environment surfaces and test caching, Bubble Tea v2
context/signal/panic behavior, Git init template/config behavior) are adopted
and re-cited in Section 61. The Stage 4 version pins (TV-01–TV-18, checked
2026-07-29) remain the version evidence base and are re-confirmed by the
version/action lock evidence record E5 (Section 51.2).

### 11.4 Limitations

No executable evidence spike was run during this revision session, and none
is claimed: SV-01–SV-05 are documentary primary-source verification only. The
five bounded evidence items in Section 51.2 remain to be executed on real
macOS and Linux hosts; they are non-waivable phase-entry gates rather than
acceptance blockers because the only work they gate is the work that depends
on them. No research finding, citation, or experiment is fabricated.

## 12. Final Technology Stack

### 12.1 Foundry Implementation

| Component            | Selection                                            | Version policy   |
| -------------------- | ---------------------------------------------------- | ---------------- |
| Language/toolchain   | Go                                                   | 1.26.5 exact     |
| CLI framework        | `github.com/spf13/cobra`                             | v1.10.2 exact    |
| TOML parsing         | `github.com/BurntSushi/toml`                         | v1.6.0 exact     |
| Module modeling      | `golang.org/x/mod` (`modfile`, `module`)             | v0.38.0 exact    |
| Unix syscall adapter | `golang.org/x/sys/unix` (exclusive no-replace rename, no-follow openat walk) | v0.47.0 exact |
| Templates            | `text/template` (restricted; Section 26)             | stdlib           |
| Logging              | `log/slog` (verbose diagnostics only)                | stdlib           |
| Test comparison      | `github.com/google/go-cmp`                           | v0.7.0 exact     |
| Process tests        | `github.com/rogpeppe/go-internal/testscript`         | v1.15.0 exact    |
| Static analysis      | Staticcheck (`honnef.co/go/tools`)                   | v0.7.0 / 2026.1  |
| Vulnerability        | govulncheck (`golang.org/x/vuln`)                    | v1.6.0 exact     |
| Release              | GoReleaser OSS v2.17.1, Syft v1.44.0 (Foundry's own release and the distribution profile) | exact |

### 12.2 Generated Project Core

Go 1.26.5 toolchain pin, zero third-party runtime dependencies, `go-cmp` for
tests, Staticcheck and govulncheck as declared tool dependencies, GitHub
Actions with full-SHA pins and Dependabot.

### 12.3 CLI Archetype

Cobra v1.10.2; `testscript` v1.15.0 for process tests.

### 12.4 TUI Archetype

Bubble Tea `charm.land/bubbletea/v2` v2.0.8, Bubbles
`github.com/charmbracelet/bubbles/v2` v2.1.1, Lip Gloss
`charm.land/lipgloss/v2` v2.0.5.

### 12.5 Distribution Profile (post-MVP)

GoReleaser OSS v2.17.1, Syft v1.44.0, GitHub artifact attestations with the
action full SHA locked at the Stage 6 version/action lock.

All versions are exact catalog pins that change only in a new catalog-bearing
Foundry release. Every dependency version, tool version, and GitHub Action
full commit SHA (with its human-readable tag recorded alongside) lives in a
single machine-readable lock manifest, `catalog/versions.toml`
(Section 33.4); catalog validation fails if any lock entry is unused or any
rendered version appears outside the manifest. Evidence record E5
(Section 51.2) re-verifies each pin against primary sources before Phase 1
exit.

## 13. Foundry Command Contract

### 13.1 Command Surface

Public commands (no aliases or hidden verbs):

```text
foundry init     --out <path> --name <name> --module <module> [flags]
foundry validate --spec <path|-> [--dest <path>]
foundry plan     --spec <path|-> [--dest <path>] [--verify default|strict]
foundry generate --spec <path|-> [--dest <path>] [--verify default|strict]
foundry catalog list
foundry catalog show <core|archetype|profile-id>
foundry doctor
foundry version
```

The specification path is always explicit; there is no implicit
working-directory `foundry.toml` discovery. `init --out` is likewise
explicit (no default path). `--dest <path>` overrides the specification's
`destination` field under identical validation rules; the resolved
destination is recorded in the plan.

`init` writes exactly one Project Spec TOML (validated before write; refuse
if `--out` exists). It does not generate a project. `doctor` reports catalog
Go pin / `FOUNDRY_GO_BIN` guidance and may probe candidate `go` binaries
under `GOTOOLCHAIN=local`; it is advisory (does not fail validate/plan).

Global flags:

- `--output text|json` — exactly one output mode; default `text`. JSON mode
  owns stdout exclusively (Section 37).
- `--quiet` — text mode only; suppress non-essential human output.
- `--verbose` — text mode only; diagnostic detail to stderr. `--quiet` and
  `--verbose` together are a usage error (exit 2).
- `--color auto|always|never` — text mode only; default `auto` (color only
  when stderr is a terminal and `NO_COLOR` is unset).

Combining `--output json` with `--quiet`, `--verbose`, or an explicit
`--color` value is a usage error (exit 2): the JSON contract is exact and has
no verbosity or decoration levels.

There is no `--offline` flag (Section 3.5) and no `--force`, `--overwrite`,
or verification-bypass flag.

### 13.2 Per-Command Contract

| Command        | Reads                              | Writes                          | Tools invoked                                    | Network                                                                 | Exit codes        |
| -------------- | ---------------------------------- | ------------------------------- | ------------------------------------------------ | ----------------------------------------------------------------------- | ----------------- |
| `init`         | None (flags only)                  | One Project Spec TOML at `--out` (no overwrite) | None | None                                                                    | 0, 2, 130         |
| `validate`     | Spec file or stdin, embedded catalog, destination metadata (read-only) | None | None                                             | None                                                                    | 0, 1, 2, 130      |
| `plan`         | Spec, embedded catalog, destination metadata (read-only) | None            | None                                             | None                                                                    | 0, 1, 2, 130      |
| `generate`     | Spec, embedded catalog, filesystem | Staging, then destination       | `go` (version, mod tidy, mod verify, test, vet; strict: staticcheck, govulncheck), optional `git` | Possible for module resolution and vulnerability data; disclosed in the plan before staging | 0, 1, 2, 130      |
| `catalog list` | Embedded catalog                   | None                            | None                                             | None                                                                    | 0, 1, 130         |
| `catalog show` | Embedded catalog                   | None                            | None                                             | None                                                                    | 0, 1, 2, 130      |
| `doctor`       | Env, PATH, optional go probe       | None                            | Optional closed `go version` probes              | None                                                                    | 0, 2, 130         |
| `version`      | Embedded build info / env          | None                            | None                                             | None                                                                    | 0, 2              |

Write-free commands (`validate`, `plan`, `catalog`, `version`) MUST perform
zero filesystem writes, zero subprocess executions, and zero network access.
Read-only destination metadata observation (`lstat` of the destination and
its parent) is permitted for `validate` and `plan` and is labeled
non-binding: only `generate` re-establishes destination state authoritatively
inside the transaction. `init` may write one Project Spec TOML; `doctor` may
probe `go` binaries under a closed environment.

### 13.3 Validate and Plan Semantics

`validate` executes the complete pure pipeline — strict parse, field
validation, catalog resolution, rendering-input assembly, plan construction,
and destination observation — and discards the plan. It therefore catches
unknown profiles, file collisions, and destination problems, not merely TOML
shape errors, and there is no divergent partial-validation code path: a
specification that validates is exactly a specification that plans.

`plan` is the authoritative dry run. Text mode prints a human-readable
summary (file paths, dependencies, verify checks, network-capable steps, git
init policy); `--verbose` expands digests/tools/argv. `--output json` emits
the full versioned plan document. `--verify default|strict` selects which
verification step list the plan records, so the plan a user inspects is
byte-equal to the plan `generate` executes with the same flags. `generate`
constructs its plan through the same pure code path; plan/generate divergence
is a defect class tested explicitly.

### 13.4 Non-Interactive Contract

No command prompts, reads the terminal for input, or blocks on anything other
than an explicit `--spec -` stdin read (UTF-8, 1 MiB cap, read to EOF).
Missing or invalid input is an immediate error with remediation, never a
question.

### 13.5 Network Disclosure

Before staging, `generate` reports (human and JSON) the exact external steps
that may require network access — module resolution during `go mod tidy` when
caches are cold, and vulnerability-database access during strict
`govulncheck` — with the reason. Schema 1 makes no whole-process
network-isolation promise. When exact pinned dependencies are unavailable,
generation fails truthfully before placement; there is no fallback and no
bypass.

### 13.6 Cancellation

SIGINT/SIGTERM cancel via one `signal.NotifyContext`. Cancellation before any
stage exists exits 130 with nothing on disk. Cancellation after the stage is
created and before the commit invocation stops external tools (process-group
kill after a bounded grace period), **preserves the stage** (Section 31.6),
reports its exact location, and exits 130. Cancellation concurrent with the
commit syscall is classified by the commit result (Section 31.9); the syscall
outcome always dominates — a committed destination is success (exit 0) even
if cancellation arrived during the rename.

### 13.7 Rejected Commands

| Rejected command/flag                 | Reason                                                                  |
| ------------------------------------- | ----------------------------------------------------------------------- |
| `dry-run`                             | `plan` is the authoritative dry run                                     |
| `clean` / stage collection            | Automatic deletion is prohibited; preservation reporting is explicit    |
| `completion` (Foundry shell)          | Deferred until demand is measured                                       |
| `publish` / release                   | GitHub/release mutation is out of scope (`DEC-010`)                     |
| `update` / `upgrade` / `migrate`      | New projects only (`DEC-003`)                                           |
| `plugin` / `template`                 | No extension platform (Section 9.2)                                     |
| `--force` / `--overwrite`             | Destination replacement is prohibited                                   |
| `--offline`                           | Unenforceable claim (FND-007)                                           |

## 14. Project Specification

### 14.1 Format

The Project Specification is a single strict TOML document, canonically named
`foundry.toml`, no larger than 1 MiB, UTF-8, product intent only. It is an
input to the Foundry, not a manifest of the Generated Project; it is not
copied into output.

### 14.2 Schema Version

The specification MUST contain exactly one top-level integer field
`schema = 1`. Its physical position has no semantic effect; canonical
examples place it first for readability only. Duplicate or non-integer
`schema` fails with source location; an unsupported value fails with
`spec.unsupported_schema` naming the supported set (`1`).

### 14.3 Field Contract

| Field                | Type     | Required | Default     | Rules                                                                                                        |
| -------------------- | -------- | -------- | ----------- | ------------------------------------------------------------------------------------------------------------ |
| `schema`             | integer  | Yes      | —           | Exactly `1`; position-independent                                                                            |
| `name`               | string   | Yes      | —           | Lowercase ASCII kebab-case, 1–63 bytes, starts with a letter, ends alphanumeric (Section 15.1)               |
| `module`             | string   | Yes      | —           | Valid per `golang.org/x/mod/module.CheckPath`; final path segment MUST equal `name`; no semantic import-version suffix (`/v2`, `/v3`, …) — initial outputs are applications, not versioned libraries |
| `description`        | string   | Yes      | —           | Trimmed, non-empty, single-line UTF-8 description of at most 200 bytes (rendered into README/help text)      |
| `archetype`          | string   | Yes      | —           | Exactly `"cli"` or `"tui"`                                                                                   |
| `destination`        | string   | Yes      | —           | Absolute or relative path; no `..`, `~`, or environment expansion; basename MUST equal `name` (Section 15.3) |
| `binary`             | string   | No       | `name`      | Same character rules as `name`                                                                               |
| `visibility`         | string   | No       | `"private"` | `"private"` or `"public"`                                                                                    |
| `profiles`           | string[] | No       | `[]`        | Exact implemented built-in profile IDs only; duplicates fail; order is semantically irrelevant (Section 23)  |
| `[git] init`         | bool     | No       | `true`      | Whether to run isolated `git init` in staging                                                                |
| `[git] initial_branch` | string | No       | `"main"`    | Lowercase ASCII kebab-case (same character rule as `name`), 1–63 bytes                                       |

The only profile ID this specification defines is `distribution`
(Section 20), which is implemented post-MVP. Until it is implemented, any
non-empty `profiles` value fails with `resolve.unknown_profile` naming the
sorted available IDs. `configuration` and `local-persistence` are not valid
IDs (Section 21).

### 14.4 Strict Decoding

Unknown fields, unknown tables, and duplicate keys are fatal errors with the
offending name and source line/column. No interpolation, includes, environment
references, or expressions exist. Field order and formatting never affect
resolution.

### 14.5 Validation Order and Aggregated Errors

Validation proceeds in a fixed order: TOML syntax → schema version → unknown
fields/duplicate keys → per-field rules → cross-field rules (module segment
versus `name`, destination basename versus `name`, profile constraints).
Independent per-field errors within one stage are collected and reported
together in source order, so a specification with three invalid fields is
fixed in one round trip rather than three. Every diagnostic carries the field
name and source line/column.

### 14.6 Secrets

The schema defines no credential, token, or secret field, and validation MUST
reject none being smuggled through unknown fields (which are already fatal).
Documentation directs secrets to environment injection at runtime of the
generated tool, never into `foundry.toml`.

### 14.7 Examples

Canonical examples appear in Appendix B, including one example that places
`schema` last to demonstrate position independence. No example selects a
removed profile or uses a removed flag.

## 15. Naming and Destination Contract

### 15.1 Project Name

Lowercase ASCII kebab-case starting with a letter
(`[a-z][a-z0-9]*(-[a-z0-9]+)*`), 1–63 bytes. Starting with a letter keeps the
name valid wherever it is reused as a binary name, Go identifier fragment, or
flag-free shell argument, and prevents hidden or numeric-only basenames. The
name is the repository directory basename and the default binary name.

### 15.2 Module Path

Validated with `golang.org/x/mod/module.CheckPath`. The final segment MUST
equal `name`. The Foundry does not verify domain ownership or reachability.

### 15.3 Destination

- Absolute or relative to the working directory; normalized lexically before
  any filesystem access.
- MUST NOT contain `..` components, `~`, NUL bytes, or environment-variable
  markers (`$`, `${`); no expansion of any kind is performed.
- Basename MUST equal `name`.
- The immediate parent MUST exist and be a directory; the destination itself
  MUST NOT exist in any form (file, directory, symlink, empty directory).
- Generation into the current directory (`destination = "."`) is prohibited.

### 15.4 Parent Acquisition and Custody (normative summary)

Existence and safety of the destination parent are established by the
transaction's no-follow parent acquisition (Section 31.2), not by a separate
pathname walk. Any symbolic link among the destination's parent components is
rejected with `fs.unsafe_path`. Each acquired component is additionally
subject to the namespace-custody check (Section 31.3): a group- or
other-writable, non-sticky parent component is rejected with
`fs.namespace_not_private`. A path-only check MUST NOT be followed by a
path-based mutation.

### 15.5 Name Equivalence

The Foundry performs **no** synthetic Unicode case-fold or normalization
sibling scan. The host filesystem determines whether two differently-cased
names are distinct or equivalent; the native exact-child lookup relative to
the retained parent handle plus the exclusive no-replace commit is
authoritative. Generated content itself is ASCII-safe by the `name` rule.

### 15.6 Platform Behavior

Identifiers in generated content are ASCII; parent directories may contain
non-ASCII names. All generated text files use LF line endings. Files are
created 0644, directories 0755, staging 0700.

## 16. Generated Project Core

### 16.1 Principles

Core is everything present in every Generated Project regardless of archetype
or profile. Core has **zero third-party runtime dependencies**, no framework,
no task runner, no dead scaffolding, and no demo behavior. Every Core file
must earn its place for a minimal ten-file personal tool.

### 16.2 Core File Inventory

```text
.github/
  dependabot.yml          # Core-owned complete static file
  workflows/
    ci.yml                # Core-owned restricted template (one required job)
    strict.yml            # Core-owned restricted template (weekly/manual strict)
.gitignore                # Core-owned complete static file
AGENTS.md                 # Core-owned restricted template
README.md                 # Core-owned restricted template
docs/
  architecture.md         # Core-owned restricted template
  commands.md             # Core-owned restricted template
  testing.md              # Core-owned restricted template
go.mod                    # Typed gomod generator (sole structured file)
go.sum                    # Produced by `go mod tidy` during generation
internal/version/
  version.go              # Core-owned static/template
  version_test.go         # Core-owned static/template
```

There is no `pkg/`, `util/`, `common/`, `types/`, `interfaces/`, `scripts/`,
root `testdata/`, `testutil`, Makefile, Taskfile, `.golangci.yml`, license
file, or persistent Foundry provenance file.

### 16.3 Documentation Ownership

Each document states its role, owner, and update trigger. `README.md` is the
human entry point; `AGENTS.md` is the single portable agent instruction
authority (canonical commands, architecture map, change-completion report
contract); `docs/architecture.md`, `docs/commands.md`, and `docs/testing.md`
are role-specific and MUST NOT duplicate each other's authority. The TUI
archetype adds `docs/ui-architecture.md`.

### 16.4 Canonical Commands

Documented identically in `AGENTS.md`, `docs/commands.md`, and CI:

```bash
gofmt -l .                 # formatting check (CI); gofmt -w . to fix
go test -count=1 ./...     # uncached tests
go vet ./...
go tool staticcheck ./...
go tool govulncheck ./...  # scheduled/strict
go build ./...
```

Staticcheck and govulncheck are declared as Go tool dependencies in `go.mod`
(`tool` directives) with exact pinned versions.

### 16.5 Quality Gates

Generated projects treat formatting, uncached tests, vet, and Staticcheck as
the universal merge gate. govulncheck, race (when host prerequisites exist),
representative macOS checks, and bounded fuzz (only when fuzz targets exist)
run on the scheduled/manual strict cadence.

### 16.6 Generated CI

Two workflows, both with least permissions and full commit-SHA action pins:

- **`ci.yml` (required; pull requests and default branch):** one Linux job
  running `gofmt -l .`, `go mod verify`, `go test -count=1 ./...`,
  `go vet ./...`, and Staticcheck, with a concurrency group that cancels
  superseded runs of the same ref. Small enough that routine agent changes
  complete quickly.
- **`strict.yml` (weekly schedule and manual dispatch):** govulncheck; race
  (`CGO_ENABLED=1`, compiler preflight, skipped with an explicit notice when
  prerequisites are missing); representative macOS test/vet; bounded fuzz only
  when fuzz targets exist.

Owners MAY promote a scheduled check to a required PR check when the project's
actual concurrency, platform, or security risk justifies it; generated docs
explain how. Dependabot (weekly, grouped) is Core. Dependency-review and
release workflows belong to the distribution profile only. The Foundry's own
repository retains a larger matrix (Section 47.1) because it owns
cross-platform filesystem behavior.

### 16.7 Public and Private Variants

Private and public projects share one architecture. The distribution profile
owns the entire public-release delta (Section 20). Nothing in Core assumes
publication.

## 17. Conventional CLI Project Archetype

### 17.1 Intended Use

Command-line tools with flags/subcommands, line-oriented output, and script
composability.

### 17.2 Canonical Tree (no profiles)

```text
.github/                    # Core (Section 16)
.gitignore
AGENTS.md
README.md
cmd/<binary>/
  main.go
docs/
  architecture.md
  commands.md
  testing.md
go.mod
go.sum
internal/
  cli/
    root.go                 # root command constructor
    root_test.go
    version.go              # version subcommand
    completion.go           # hidden completion subcommand (Cobra standard)
    script_test.go          # testscript process tests
    testdata/script/
      help.txt
      version.txt
      completion.txt
  version/
    version.go
    version_test.go
```

There is **no** `internal/greet`, no demo domain service, no fake subcommand.
The generated CLI is a minimal working shell: root help, `version`, and shell
completion, each with unit and process tests. Richer examples (adding an
application service package, wiring a new subcommand, boundary-package
patterns) live in Foundry-owned integration fixtures and in
`docs/architecture.md` as a written recipe, clearly labeled as guidance rather
than generated code.

### 17.3 Process Boundary

`main.go` is thin: construct the root command, call `ExecuteContext` with a
`signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`
context, map the returned error to an exit code once, and call `os.Exit`
exactly once. No other file calls `os.Exit`.

### 17.4 Command Construction

Commands are constructed fresh by constructor functions (no package-level
command globals, no `init` registration). Dependencies are passed explicitly.
Cobra is confined to `internal/cli`; application logic added later lives in
responsibility-named `internal/` packages that do not import Cobra. The root
command sets `SilenceErrors` and `SilenceUsage` so the process boundary owns
error presentation exactly once; running the binary with no subcommand prints
deterministic help and exits 0.

### 17.5 Errors, Output, and Exit

Errors are ordinary wrapped Go errors translated once at the process boundary.
Stable exit statuses: 0 success, 1 runtime failure, 2 usage error, 130
interrupted. Human output to stdout; diagnostics to stderr. The generated
`version` subcommand accepts `--output text|json`, giving every generated CLI
a working machine-output seam from day one that future subcommands follow.

### 17.6 Side Effects and Growth

Side-effect boundaries (filesystem, subprocess via `os/exec`, HTTP) are added
as responsibility-named packages only when the project actually needs them;
none are pre-generated. Growth rules in `docs/architecture.md`: one package
per responsibility; no generic buckets; extract only at real boundaries.

### 17.7 Tests

Unit tests beside code; command tests through the constructor; `testscript`
process tests for help/version/completion exercising real stdout/stderr/exit
behavior. All documented test commands use `-count=1`.

## 18. Full-Screen TUI Project Archetype

### 18.1 Intended Use

Full-screen interactive terminal applications using the Elm-style
model/update/view cycle. The TUI is the primary interaction model; there is no
peer Cobra command tree (startup flags only).

### 18.2 Canonical Tree (no profiles)

```text
.github/                    # Core
.gitignore
AGENTS.md
README.md
cmd/<binary>/
  main.go
docs/
  architecture.md
  commands.md
  testing.md
  ui-architecture.md
go.mod
go.sum
internal/
  tui/
    app.go                  # program construction and options
    state.go                # single model type
    update.go               # sole state-transition function
    update_test.go
    view.go                 # pure rendering
    view_test.go
    keymap.go
    theme.go
    lifecycle_test.go       # signal/quit/effect-completion tests
  version/
    version.go
    version_test.go
```

One initial package (`internal/tui`) with explicit file responsibilities;
packages are extracted only when real growth proves a boundary. The generated
TUI is a minimal working shell: one static screen showing the project name,
key help (rendered with the Bubbles help component, so the Bubbles dependency
has immediate real use), and quit handling, plus resize, `--no-color`, and
small-terminal behavior. There is **no** synthetic asynchronous
initialization, no fake loading/result/error message cycle, and no artificial
long-running effect — and consequently **no generated `effects.go` or
`messages.go`**: a shell with no asynchronous behavior gets no empty effect
scaffolding. The cooperative-effect rules (Section 18.7) are documented in
`docs/ui-architecture.md` and proven by a Foundry-owned integration fixture,
so the first real feature has a worked example without every generated
repository carrying dead files.

### 18.3 Single Lifecycle Owner (normative)

The generated TUI has exactly one owner for signals, cancellation, effects,
panics, and exit:

1. `main` creates one application context and cancel function with
   `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`.
2. The program is constructed with `tea.WithContext(appCtx)` and
   `tea.WithoutSignalHandler()`. No second signal handler exists anywhere.
3. Bubble Tea's default panic recovery remains enabled (`WithoutCatchPanics`
   is never used). `main` does not claim to recover panics from effect
   goroutines; the framework owns terminal restoration on panic.
4. Every user quit path (for example the `q` key) invokes the application
   cancel function before returning `tea.Quit`.
5. Every effect (`tea.Cmd`, when the project adds one) blocks only in
   context-aware operations derived from `appCtx` and returns promptly after
   cancellation. Bubble Tea does not join command goroutines, so cooperation
   is mandatory and tested.
6. Exit mapping distinguishes the three quit classes exactly: the `q` key
   (deliberate user quit) → 0; Ctrl-C delivered as a key event inside the
   program, external SIGINT/SIGTERM cancellation, or `tea.ErrProgramKilled`
   → 130; startup or runtime failure → 1.

### 18.4 State, Update, View

One model type in `state.go`; `update.go` is the only state-transition site;
`view.go` is pure and side-effect free; styling is centralized in `theme.go`
with Lip Gloss; key bindings in `keymap.go` drive both behavior and the help
view. Typed messages are introduced in a `messages.go` only when the first
real asynchronous feature needs them. The view defines explicit minimum
dimensions and renders a deterministic "terminal too small" notice below
them, so resize behavior is total and testable.

### 18.5 Startup Flags

`main` parses exactly three flags before starting the program: `--version`,
`--no-color`, and `--debug-log <path>`. Nothing else. Profiles and future
project code MUST NOT add startup flags that contradict this contract without
updating `docs/ui-architecture.md`.

### 18.6 Terminal Restoration

The alternate screen is entered and restored by Bubble Tea. PTY tests prove
restoration on normal quit, SIGINT/SIGTERM cancellation, and
framework-recovered panic.

### 18.7 Effects and Background Work (rules for future code)

The generated shell contains no effects. When the project adds its first
asynchronous feature, effects MUST be constructed as `tea.Cmd` values that
receive `appCtx` (or a child context), block only in context-aware
operations, and report completion via typed messages; tests MUST track effect
start/finish and assert that no effect remains active when the program
returns. These rules are normative for generated documentation and are
demonstrated by a Foundry-owned integration fixture with a cancellable-effect
example, not by generated placeholder files.

### 18.8 Debug Logging

`--debug-log <name>` accepts a **safe basename only**: no path separators, no
`..`, not `.`, not empty, and not a hidden name. `main` captures a descriptor
for the startup working directory once, before any other work, and creates
the log relative to that descriptor as a **new regular file** with
`O_CREATE|O_EXCL|O_WRONLY|O_NOFOLLOW`, mode `0600`, applying `fchmod` on the
open descriptor so the umask cannot widen it, and verifying through the
descriptor that the created object is a regular file. Any existing object at
the name — regular file, symlink, directory, FIFO, socket, or device — is a
startup error naming the path and the remediation (choose a new name or
remove the old file manually). The application never truncates, appends to,
follows symlinks for, or creates parent directories for a debug log, and the
log may not alias stdout/stderr. Logging uses `slog` JSON with redaction of
known-sensitive keys. Without the flag, the TUI writes no log files.

### 18.9 Tests

Model/update unit tests; view render goldens (including no-color, minimum
dimensions, and too-small terminal sizes); lifecycle tests per Section 18.3
covering `q`, Ctrl-C-as-key, SIGINT, SIGTERM, and startup failure; PTY
restoration smoke tests; debug-log safety matrix per Section 18.8 including
separator/traversal arguments.

## 19. Capability Model and Classification

### 19.1 Definitions

A **capability** is a coherent, recurring product ability (for example
"publish signed public releases"). A **Capability Profile** is a named,
versioned catalog unit that composes one complete capability onto an
archetype. Capabilities are delivered by exactly one of: Core (universal),
Archetype (interaction-model-specific), a named Profile (optional, explicit),
a Recipe (documented post-generation guidance), or project-specific work.

### 19.2 Classification Rules

- **Core** requires near-universal benefit without slowing feedback.
- **Archetype** requires the ability to be inseparable from the interaction
  model.
- **Profile** requires a complete, recurring, domain-independent architecture:
  files, dependencies, docs, and tests that at least two real projects would
  retain unchanged. A profile that can only emit placeholders the owner must
  replace is not a capability; it is scaffolding and is prohibited.
- **Recipe** captures a sound decision pattern (libraries, precedence,
  criteria) that depends on application intent the Project Specification does
  not carry.
- Additions bear the burden of proof; deletion is preferred (Charter).

### 19.3 Classification Table

| Candidate                          | Classification            | Notes                                                                                       |
| ---------------------------------- | ------------------------- | ------------------------------------------------------------------------------------------- |
| Go toolchain, module layout, docs, CI, Dependabot, AGENTS | Core       | Section 16                                                                                  |
| Cobra command tree                 | CLI Archetype             | Section 17                                                                                  |
| Bubble Tea stack                   | TUI Archetype             | Section 18                                                                                  |
| Public release (`distribution`)    | **Profile (post-MVP)**    | Complete repository-level artifacts independent of application domain (Section 20)          |
| Configuration (env + TOML)         | **Recipe**                | Retired from the generated catalog by this revision (Section 21.1)                          |
| Local persistence (files, bbolt)   | **Recipe**                | Retired from the generated catalog by this revision (Section 21.2)                          |
| HTTP client                        | Recipe/deferred           | Domain-specific auth/retry/pagination; stdlib first                                         |
| Rate limiting/retry                | Recipe/deferred           | Idempotency-dependent                                                                       |
| Property testing (`rapid`)         | Deferred                  | Built-in fuzzing and tables cover initial risk                                              |
| Homebrew                           | Deferred                  | No demonstrated public install demand                                                       |
| Helper binary                      | Deferred                  | No initial capability has a distinct process lifecycle; no schema field is reserved         |
| Interactive prompts                | Deferred/recipe           | Must not undermine automation                                                               |
| Advanced observability             | Project-specific          | Local tools need clear errors, not telemetry stacks                                         |
| Secret storage, Viper, SQLite default, mutation testing, self-update, vendor agent files | Rejected | Section 58 |

### 19.4 Profile Admission Test (normative)

A future profile MAY be admitted to the catalog only when **all** of the
following hold, recorded in an accepted specification revision:

1. The capability is complete: it works at generation time with no
   placeholder the owner must replace.
2. It is domain-independent: it requires no application intent the Project
   Specification does not carry.
3. It is recurring: at least two real Generated Projects independently
   retained substantially the same files, dependencies, docs, and tests.
4. It has exactly one owner for every file it emits and introduces no shared
   structured-file semantics beyond typed `go.mod` contributions.
5. It composes flatly: its constraints are expressible as direct
   archetype/visibility predicates with no requires/conflicts graph.
6. It keeps the generated test and CI burden proportionate to the projects
   that select it.
7. The catalog's combination test matrix remains bounded (Section 45); any
   growth strategy uses archetype-alone, profile-alone, direct-predicate,
   pairwise, and curated maximal cases — never a hypothetical closure graph.

## 20. Initial Capability Profile: `distribution`

The only profile in the initial catalog. Implemented in Phase 4, after the
MVP.

| Field                  | Contract                                                                                                                                                                 |
| ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ID                     | `distribution` (stable)                                                                                                                                                     |
| Purpose                | Complete public binary-release scaffolding for a Generated Project                                                                                                          |
| Compatible archetypes  | `cli`, `tui`                                                                                                                                                                |
| Direct predicate       | `visibility = "public"` **and** a GitHub-hosted module path (`github.com/<owner>/<name>` exactly matching the module field); any other selection fails `resolve.profile_constraint` (exit 2) with the failed predicate named |
| Owned files            | `.github/workflows/release.yml`, `.github/workflows/dependency-review.yml`, `.goreleaser.yaml`, `docs/releasing.md`, `CONTRIBUTING.md`, `SECURITY.md` — all complete owned static files or restricted templates |
| `go.mod` contributions | None (no runtime dependency)                                                                                                                                                |
| Release targets        | darwin/linux × amd64/arm64, `CGO_ENABLED=0`, checksums, SBOM (Syft), GitHub artifact attestations, immutable-release guidance                                               |
| Security               | Tag-triggered only; least permissions; full-SHA action pins; never runs on untrusted pull requests                                                                          |
| Helper binaries        | None                                                                                                                                                                        |

**License (normative, from FND-019).** Generation requires only
`visibility = "public"`, a supported GitHub-shaped module path, and explicit
selection. The Foundry MUST NOT inspect, select, generate, or validate a
license; the destination does not exist at generation time. Generated
`docs/releasing.md`, `README.md`, and workflow comments MUST state
conspicuously that publication is blocked until the owner adds and reviews an
appropriate license and completes the documented repository settings
(immutable releases, permissions). Release dogfood — not generation — verifies
the license before the first public tag.

**Core interaction (normative).** When `distribution` is selected, Core-owned
templates (for example `README.md`) MAY render an additional
release-documentation section from the frozen boolean fact
`DistributionEnabled` (Section 26.2). Ownership never transfers: the Core
owner controls every byte of its files including the conditional branch, both
branches are covered by rendering goldens, and no profile patches, appends
to, or merges into another owner's file.

## 21. Project-Specific Recipes

These recipes preserve the Stage 1–4 evidence for capabilities whose generated
form was retired by this revision. They are documentation only: no profile ID,
no generated files, no compatibility promise, no phase gate. Each may be
proposed as a real profile only after **two real Generated Projects
independently retain substantially the same complete code/docs/test
architecture**, measured during dogfood (`REQ-247`), and only through an
explicit revision of this specification.

### 21.1 Configuration Recipe

Precedence: explicit flags > environment variables > one optional TOML file;
no automatic config-file discovery. Libraries when needed:
`github.com/caarlos0/env/v11` (v11.4.1) for typed environment decoding and
`github.com/BurntSushi/toml` (v1.6.0) for explicit file parsing. Secrets enter
only via environment injection; never generate secret fields or example
credentials. Config files created by the tool use 0600 when they may hold
sensitive values.

### 21.2 Local Persistence Recipe

Ordinary files first (atomic write-rename, explicit schema/versioning in the
file content). Adopt `go.etcd.io/bbolt` (v1.5.0) only for a demonstrated
transactional local key/value need, with domain-named buckets, explicit
encodings, and tests; never a generic wrapper or speculative metadata bucket.

## 22. Deferred and Rejected Capability Candidates

The Stage 4 deferred/rejected registers survive with these deltas:

| Item                              | Status after revision      | Trigger to revisit                                                                       |
| --------------------------------- | -------------------------- | ----------------------------------------------------------------------------------------- |
| `configuration` profile           | Retired to recipe          | Two real projects retain one complete configuration architecture                          |
| `local-persistence` profile       | Retired to recipe          | Two real projects retain one complete persistence architecture                            |
| Helper-binary schema support      | Deferred, no reserved field | A real product requires a separately supervised helper; explicit spec revision            |
| Profile composition semantics (requires/conflicts/capabilities) | Deleted from schema 1 | A concrete future profile provably cannot be expressed flat; explicit spec revision |
| `--offline` or any offline mode   | Rejected                   | None; a network sandbox is out of scope                                                   |
| HTTP client, rate limiting, `rapid`, Homebrew, prompts, observability, signing/notarization, richer report UI | Deferred as in Stage 4 | Unchanged triggers (Section 57) |

## 23. Profile Resolution and Compatibility

### 23.1 Flat Model

Schema-1 profile selection is a flat deterministic set of exact implemented
built-in profile IDs. A profile declares:

- a stable ID;
- compatible archetypes;
- an optional direct visibility predicate;
- complete owned output files;
- `go.mod` dependency/tool contributions (typed, Section 26.4); and
- profile-owned documentation and verification additions.

### 23.2 Resolution Rules

- Unknown ID → `resolve.unknown_profile` with sorted available IDs (exit 2).
- Duplicate ID → `spec.duplicate_profile` naming both indexes (exit 2).
- Incompatible archetype or failed visibility predicate →
  `resolve.profile_constraint` (exit 2).
- Selection order is semantically irrelevant; resolution output is sorted by
  profile ID.
- Output-path collisions and dependency-version conflicts across
  Core/archetype/profiles are fatal (Section 25.2, `plan.file_collision`).

### 23.3 Explicitly Absent

Schema 1 has **no** transitive profile requirements, requirement closure,
profile-to-profile conflict relation, cycle detection, capability registry,
capability providers (exclusive or additive), helper-binary fields,
provenance chains, or topological ordering. The resolver contains no DFS and
no graph. Any future need for such semantics requires an explicit revision of
this specification demonstrating that the flat model cannot express a concrete
profile.

## 24. Source Catalog

### 24.1 Character

The catalog is Git-visible source in the Foundry repository, embedded into
release binaries via `go:embed`, versioned with the Foundry, and not
user-extensible. A SHA-256 digest of the embedded catalog is computed
deterministically, reported by `foundry version` and in every plan, and
compared in CI against the repository tree (release gate).

### 24.2 Tree

```text
catalog/
  core/
    files/...                 # static files and .tmpl templates
    manifest.toml
  archetypes/
    cli/
      files/...
      manifest.toml
    tui/
      files/...
      manifest.toml
  profiles/
    distribution/             # present in the tree; enabled post-MVP
      files/...
      manifest.toml
  schemas/
    catalog-manifest.md       # human-readable manifest field contract
  testdata/                   # hostile and synthetic fixtures (not embedded)
```

### 24.3 Manifests

Strict TOML manifests declare, per unit: ID, description, output files
(path, mode, render mode `static|template`, source), `go.mod` contributions
(module, version, scope `runtime|test|tool`), compatible archetypes and
visibility predicate (profiles only), documentation additions, and
verification additions. Manifests contain **no** `requires`, `conflicts`,
`capabilities`, `provides`, `helper_binary`, or render modes beyond
`static`/`template`. `go.mod` is not listed as an owned file; it is produced
by the typed generator from the declared contributions.

### 24.4 Validation

Catalog validation (build time, CI, and on load) rejects: unsafe or absolute
output paths, `..`, symlinks, non-regular files, duplicate output ownership,
unparsable templates, unknown manifest fields, undeclared source files, and
binary assets. Development filesystem loading exists only under the
`foundrydev` build tag, applies the same validation, fails closed, and is not
a production extension surface.

## 25. File Contribution and Ownership

### 25.1 One Owner

Every ordinary output file has exactly one owner: Core, one archetype, or one
profile. Any duplicate output path — including byte-identical content — fails
with `plan.file_collision` reporting every claiming owner. Parent-file versus
child-path conflicts are collisions. There is no override, layering, or
last-writer-wins.

### 25.2 The Only Shared File

`go.mod` is the only file with multiple semantic contributors (Core toolchain
and tool directives, archetype dependencies, profile dependencies). It is
produced by the typed `gomod` generator (Section 26.4). Version conflicts for
the same module across contributors are fatal. `go.sum` is produced solely by
the `go mod tidy` external step; no unit contributes to it directly.

### 25.3 Modes, Directories, Assets

Files 0644, directories 0755 (created implicitly for owned files), staging
0700. No empty directories, no binary assets, no symlinks in output, no
executable files in the initial catalog.

## 26. Rendering

### 26.1 Mechanisms

Exactly three rendering mechanisms exist:

1. **Static copy** — invariant bytes copied verbatim.
2. **Restricted template** — complete-file `text/template` rendering.
3. **Typed `go.mod` generation** — Section 26.4.

The Stage 4 limited-substitution token language (`@{...}`) and all typed
YAML/GoReleaser/`.gitignore`/Dependabot/workflow emitters are deleted. Every
non-`go.mod` output is a complete owned file a maintainer can read and edit
directly in the catalog.

### 26.2 Restricted Template Contract

- Delimiters `[[` and `]]`; `missingkey=error`; frozen typed data (no maps
  with nondeterministic iteration — slices are pre-sorted).
- Function map contains exactly `join` and `quote`.
- No partials, includes, nested template definitions, reflection, environment,
  filesystem, time, randomness, network, or command execution.
- Template inputs come only from the validated specification and catalog
  constants.

### 26.3 Go Source

All rendered Go source is parsed and formatted with `go/format`; a file that
fails to parse fails generation (`render.failed`).

### 26.4 Typed `go.mod`

Built with `golang.org/x/mod/modfile` from the aggregated typed contributions:
module path, `go 1.26.0`, `toolchain go1.26.5`, exact-version requires, and
tool directives. Deterministic ordering; conflicting versions fatal. The
post-`go mod tidy` `go.mod`/`go.sum` bytes become the approved expected values
for final conformance (Section 35.3).

### 26.5 Prohibited

Shell commands, arbitrary functions, network access, timestamps, host or user
data, and text patches/fragments are prohibited in all rendering paths.

## 27. Resolver

Pure computation: no filesystem, environment, subprocess, clock, logger, or
output. Inputs are the `ValidatedSpecification` and `ValidatedCatalog`;
outputs are an immutable `ResolvedProject` or an immutable `FoundryError`.

Steps, in order:

1. Select the archetype by exact ID (`cli` or `tui`); unknown fails.
2. Validate each selected profile ID against implemented built-ins; unknown or
   duplicate fails.
3. Enforce each profile's direct archetype and visibility predicates.
4. Collect file contributions and `go.mod` contributions from Core, the
   archetype, and selected profiles.
5. Detect output-path collisions and dependency-version conflicts.
6. Sort everything deterministically (paths, profile IDs, modules).

No graph traversal, cycle detection, capability mapping, or provenance-chain
construction exists (Section 23.3).

## 28. Generation Plan

### 28.1 Purpose

The Generation Plan is the immutable, inspectable contract between pure
interpretation and side effects. `plan` prints it without writes; `generate`
executes exactly it. The plan is not executable content and contains no
secrets, timestamps, usernames, or hostnames.

### 28.2 Normative Fields (JSON schema 1)

| Field                      | Content                                                                                                                                      |
| -------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `schema`                   | Integer plan schema, `1`                                                                                                                        |
| `foundry`                  | Foundry version, commit (when embedded), Go version, catalog digest                                                                             |
| `specification`            | Source (`path` or `stdin`) of the input specification                                                                                           |
| `project`                  | name, binary, module, description, archetype, visibility                                                                                        |
| `destination`              | Absolute normalized destination path, parent, basename, and the non-binding read-only observation (`absent`\|`exists`\|`parent-missing`) at plan time (plan identity only; never generated content) |
| `profiles`                 | Sorted selected flat profile IDs                                                                                                                |
| `files`                    | Every planned output: path, owner, mode, render mechanism (`static`\|`template`\|`gomod`), catalog source, catalog source digest, and planned rendered-content digest (SHA-256) where the mechanism is content-deterministic |
| `dependencies`             | Aggregated modules: module, exact version, scope, owner                                                                                         |
| `tools`                    | Declared tool dependencies with exact versions and owner                                                                                        |
| `external_steps`           | Exact planned subprocess steps: id, binary, argv, cwd (`stage-descriptor`), mutates, network (`no`\|`may`), timeout, output cap, and the exact environment allowlist |
| `tool_outputs`             | Files external steps may create/modify (`go.sum`; `go.mod`/`go.sum` for tidy)                                                                  |
| `verification`             | Mode (`default`\|`strict`) and ordered checks, including the final-conformance step                                                             |
| `network`                  | `may_be_required` boolean plus reason(s)                                                                                                        |
| `git`                      | init boolean, initial branch, and `isolated: true`                                                                                              |
| `plan_digest`              | SHA-256 of the canonical plan serialization (excluding this field), for plan/generate equality checks and provenance                            |
| `commit_result_model`      | Reference to the Section 31.9 result matrix (documented enum of outcomes)                                                                       |
| `warnings`                 | Sorted stable warnings                                                                                                                          |

Removed relative to Stage 4: profile closure/provenance, capability list,
helper-binary implications, generic structured-contribution classes, and every
`offline` field. Runtime descriptors (parent/stage handles) are transaction
state, never serialized.

### 28.3 Determinism and Immutability

Identical validated inputs produce byte-identical plan JSON (stable field
order, sorted collections). The plan value is immutable after construction:
defensive copies in, read-only access out. Plan validation proves every
planned path is representable, collision-free, and safe before `generate`
proceeds.

### 28.4 Illustrative Example

Appendix C contains a complete illustrative plan. The normative field
contract is this section.

## 29. Generation Lifecycle

### 29.1 State Machine

The post-plan lifecycle is a total state machine with named states:

```text
planned → parent-acquired → stage-created → rendered → normalized(tidy)
        → frozen → verified → conformed → git-initialized (optional)
        → committing → committed | conflicted | failed-preserved | ambiguous
```

Every state transition emits a typed event consumed by reporting
(Section 42.2); the state machine itself contains no output encoding. There
is no transition from any post-`stage-created` failure to a deleted stage:
failure and cancellation states preserve the stage (Section 31.6).

### 29.2 Ordered Stages

`generate` executes this total sequence. Every stage names its failure
behavior; every mutation is descriptor-relative (Section 31).

| #  | Stage                          | Side effects                            | Failure behavior                                                                    |
| -- | ------------------------------ | --------------------------------------- | ------------------------------------------------------------------------------------ |
| 1  | Read and parse specification   | None                                    | Exit 2 with source location                                                           |
| 2  | Validate specification         | None                                    | Exit 2 (independent field errors aggregated)                                          |
| 3  | Load and validate catalog      | None                                    | Exit 1 (`catalog.invalid`)                                                            |
| 4  | Resolve                        | None                                    | Exit 2 (selection errors) or 1 (catalog defects)                                      |
| 5  | Build and validate plan        | None                                    | Exit 1 (`plan.file_collision`, internal)                                              |
| 6  | Tool preflight (locate `go`/`git`, exact `go version` check) | None      | Exit 1 (`tool.missing`, `tool.wrong_version`); nothing written                        |
| 7  | Report plan summary and network disclosure | Output only                 | Report failure before staging exits 1; nothing was written                            |
| 8  | Acquire destination parent (no-follow walk, custody check, retain handle) | Handle only | Exit 2 (`fs.unsafe_path`, `fs.namespace_not_private`, `fs.destination_exists`, missing parent) |
| 9  | Create and own stage (`.foundry-<name>-<random>`, exclusive, 0700, bounded `EEXIST` retries, relative to parent handle) | Stage created | Exit 1; the created stage (if any) is preserved and reported |
| 10 | Render all planned files through identity-verified rooted writer; format Go; `fchmod` exact modes; initial conformance | Stage writes | Exit 1 (`render.failed`); **stage preserved** |
| 11 | `go mod tidy` (declared network-may step; child cwd bound to stage descriptor) | `go.mod`/`go.sum` in stage | Exit 1 (`tool.failed`); **stage preserved**             |
| 12 | Freeze approved post-tidy tree (paths, types, modes, bytes); validate tidy mutation set (`go.mod`/`go.sum` only, exact pins reparse, no replace/exclude directives) | None | Exit 1 (`verify.module_mutation`); **stage preserved** |
| 13 | `go mod verify`, `go test -count=1 -buildvcs=false ./...`, `go vet -buildvcs=false ./...`; strict adds `go tool staticcheck ./...`, `go tool govulncheck ./...` | Tool caches only (intended) | Exit 1 (`tool.failed`/`verify.failed`); **stage preserved** |
| 14 | Final conformance: compare every non-`.git` path/type/mode/byte against the frozen tree | None | Exit 1 (`verify.unplanned_mutation`); **stage preserved**                 |
| 15 | Optional isolated `git init --initial-branch=<branch> --template=<scratch>` (child cwd bound to stage descriptor) | `.git` in stage | Exit 1 (`git.failed`); **stage preserved**       |
| 16 | Remove the Foundry-owned Git template scratch (which lives under the Foundry's own temporary root, never in the destination namespace); repeat non-`.git` conformance; validate `.git` semantically | Scratch removal outside the transaction namespace | Exit 1; **stage preserved** |
| 17 | Diagnostic parent reobservation (compare a fresh open of the authored parent path against the retained handle's identity) | None | Exit 1 (`fs.parent_moved`); **stage preserved** |
| 18 | Exclusive no-replace commit relative to retained parent handle; post-syscall identity classification | Destination appears atomically | Section 31.9 result matrix                     |
| 19 | Best-effort report                                              | Output only  | Post-commit stream failure never changes exit 0 (Sections 31.9, 36.5)    |

Cancellation at stages 1–8 exits 130 with nothing on disk. Cancellation at
stages 9–17 stops external tools, **preserves the stage**, reports its exact
location, and exits 130. Cancellation concurrent with stage 18 is classified
by the commit result.

## 30. Validate, Plan, Dry-Run, and Generate Semantics

| Behavior                       | `validate` | `plan`             | `generate`                       |
| ------------------------------ | ---------- | ------------------ | -------------------------------- |
| Parse and validate spec        | Yes        | Yes                | Yes                              |
| Load/validate catalog          | Yes        | Yes                | Yes                              |
| Resolve and build plan         | Yes (plan discarded) | Yes      | Yes                              |
| Observe destination (read-only, non-binding) | Yes | Yes           | Re-established inside the transaction |
| Render                         | No         | No                 | Yes (staging only)               |
| External tools                 | No         | No                 | Yes (declared steps only)        |
| Filesystem writes              | No         | No                 | Staging, then exclusive commit   |
| Network                        | No         | No                 | Possible; disclosed before staging |
| Primary output                 | Validity   | Full plan          | Result report                    |

`validate` runs the identical pure pipeline as `plan` and discards the plan,
so there is exactly one validation code path (Section 13.3). `plan` **is**
the dry run. There is no separate `dry-run` command, and `generate` has no
`--dry-run` flag.

## 31. Filesystem Transaction and Safety

This section is the single normative authority for every filesystem side
effect the Foundry performs. It integrates FND-001, FND-002, FND-003, and
FND-018: all mutating operations are **descriptor-relative**; a path-only
check is never followed by a path-based mutation.

### 31.1 Threat and Correctness Model

Protected against: concurrent renames/swaps of destination parent components,
symlink introduction at any parent component or at the destination itself,
pre-creation of the destination in any form, redirection of child-process
working directories, and unplanned mutation of staged content by external
tools. There is no staging-cleanup redirection threat because there is no
staging cleanup: deletion of the operation class removes the attack surface.

Out of scope, as an explicit **trusted-host boundary**: a privileged actor
(root), any process running as the same effective user, and any principal
already authorized to write the destination's parent chain. The
namespace-custody check (Section 31.3) exists precisely to make this boundary
checkable rather than implicit: the Foundry refuses to operate where the
boundary does not hold, instead of defending inside a namespace it cannot
own.

### 31.2 No-Follow Parent Acquisition

The destination parent is acquired by opening the first parent component and
walking each subsequent component with `O_NOFOLLOW|O_DIRECTORY` semantics
relative to the previously opened descriptor (via `os.Root` opened on the
walk's verified starting directory, or equivalent `openat` discipline). Any
symbolic link among the components fails `fs.unsafe_path` (exit 2). The
resulting **parent handle** is retained for the transaction's entire life and
is the only object against which existence checks, staging creation, child
startup, and commit are performed. A directory descriptor continues to
identify the same filesystem object regardless of subsequent pathname renames
(SV-03), which is the property the whole transaction rests on.
`os.OpenRoot`/`os.MkdirTemp` on a whole untrusted pathname MUST NOT be used
for any transactional step.

### 31.3 Namespace Custody

During the component walk, each opened component is `fstat`-checked through
its descriptor:

- owned by the current effective user (or root for standard system prefixes
  such as `/`, `/home`, `/Users`); and
- not writable by group or other, **unless** the sticky bit is set (mode
  `1777` world-writable directories such as `/tmp` are permitted because
  sticky semantics prevent other users from renaming or unlinking the
  Foundry's entries — SV-05).

A component failing custody fails `fs.namespace_not_private` (exit 2), naming
the component, its owner, and its mode. This is the classic shared-directory
attack control, and it converts the Section 31.1 trust boundary from prose
into an enforced check.

### 31.4 Destination Existence

The destination basename is checked with a no-follow exact-child lookup
relative to the parent handle. Any existing object — file, directory, symlink,
empty directory — fails `fs.destination_exists` (exit 2). This check is
advisory ordering only; the commit's exclusive no-replace semantics are the
authoritative guarantee.

### 31.5 Stage Creation and Ownership

The stage is created **relative to the parent handle**: a randomized
`.foundry-<name>-<random>` entry (the project name embedded so preserved
stages are attributable at a glance) created with mkdirat-equivalent
semantics, mode 0700, retrying on `EEXIST` a bounded number of times (16)
before failing. Success means this process created that exact directory
entry; the transaction records the stage's identity (device/inode), then
opens the stage as a root
(`openat(parent_fd, name, O_NOFOLLOW|O_DIRECTORY)` → `os.Root`) and retains
the **stage handle**. All rendering flows through this rooted writer, which
forbids absolute paths, `..`, and symlink traversal. Before rendering, the
transaction verifies identity (device/inode) between the stage handle and a
re-lookup relative to the parent handle. File modes are applied with `fchmod`
on the open descriptor before close, so the process umask can never widen or
narrow planned modes.

### 31.6 Stage Preservation — No Automatic Deletion (from FND-002)

**The Foundry contains no automatic stage deletion.** No production code path
may unlink a stage entry, recursively remove a stage tree, or delete any
object under the destination parent, under any outcome: render failure, tool
failure, verification failure, Git failure, cancellation, commit conflict,
commit failure, or ambiguity. This deletes the entire class of
deletion-redirection races that FND-002 identified, instead of hardening the
operation.

Consequences, all normative:

- Every failure or cancellation after stage creation reports the preserved
  stage's exact location (parent path as authored, stage basename, and
  identity confirmation) in both human and JSON output, with a one-line
  manual removal remediation the owner can run after inspecting it.
- Preserved stages are `0700` hidden directories named
  `.foundry-<name>-<random>`; nothing but the owner can read them, and their
  names make both origin and project obvious.
- There is no `clean` command, scavenger, startup sweep, or age-based
  collection. Manual removal is the owner's deliberate act.
- The scope of this rule is the destination-parent namespace. Foundry-owned
  scratch space created under the Foundry's own temporary root (for example
  the empty Git template directory, Section 34.2) is outside the untrusted
  namespace and is removed normally.
- If the manual-cleanup burden becomes material in dogfood, the `RSK-310`
  revisit trigger (Section 55) requires a separately reviewed deletion design
  proved safer than unconditional preservation — never an inline convenience
  patch.

The Foundry never deletes, replaces, or modifies a committed destination.

### 31.7 Exclusive No-Replace Commit

Commit renames the stage entry to the destination basename **relative to the
parent handle** with exclusive no-replace semantics:

- Linux: `unix.Renameat2(parentFd, stage, parentFd, dest, unix.RENAME_NOREPLACE)`.
- macOS: `unix.RenameatxNp(parentFd, stage, parentFd, dest, unix.RENAME_EXCL | unix.RENAME_NOFOLLOW_ANY)`,
  so no component of either rename path may be a symbolic link at commit
  time.

Both are provided by `golang.org/x/sys/unix`. `EEXIST` maps to
`fs.destination_exists`. `ENOTSUP`/`EINVAL` (filesystem without no-replace
support) maps to `fs.rename_unsupported`, a fail-closed error naming the
filesystem limitation; there is no link/unlink or check-then-rename fallback.
An unexpected `EXDEV` (the parent silently became a different filesystem) is
likewise fail-closed. Support on APFS, ext4, xfs, and btrfs is confirmed by
the E1 evidence record (`OQ-400`). The destination therefore appears
atomically, complete, verified, and never partially.

Immediately before the commit syscall, the transaction performs a diagnostic
**parent reobservation**: it re-opens the authored parent path and compares
identity with the retained handle. A mismatch (`fs.parent_moved`, exit 1)
means the namespace changed under the transaction; committing through the
retained handle would place the repository somewhere the user can no longer
name, so the Foundry preserves the stage and stops. This is a diagnostic
refinement, not a pathname fallback: the mutation itself still occurs only
through the retained handle.

### 31.8 Post-Syscall Commit Classification

The rename result is classified by **identity inspection relative to the
retained parent handle**, not by the error value alone, so ambiguous
acknowledgments (interrupted syscalls, network filesystems that report errors
after applying operations) are handled totally:

1. Syscall reports success → confirm the destination child's identity equals
   the recorded stage identity → **committed**.
2. Syscall reports failure → inspect both children relative to the parent
   handle:
   - stage entry present with recorded identity, destination absent →
     **uncommitted** (classify the error: conflict, unsupported, other);
   - stage entry absent, destination present with recorded stage identity →
     **committed** (the error was a false negative; exit 0);
   - destination present with a different identity → **conflict** (stage
     preserved, exit 2);
   - any other combination → **ambiguous**: stop all further mutation,
     preserve every reachable object, report `fs.commit_ambiguous` naming
     both paths, both observed identities, and manual inspection remediation
     (exit 1).

### 31.9 Commit Result Matrix (from FND-003, FND-012)

The exit status is dominated by the transaction result, not by cancellation
timing or post-commit reporting:

| Commit outcome                                      | Stage handling                                             | Report                                                    | Exit |
| --------------------------------------------------- | ----------------------------------------------------------- | ----------------------------------------------------------- | ---- |
| Rename succeeded (or classified committed)          | Consumed by the rename                                      | Success report                                              | 0    |
| Rename succeeded, report stream failed              | Consumed                                                    | Best-effort stderr notice; JSON never reports error         | 0    |
| Rename failed (`EEXIST` — concurrent winner)        | **Preserve** the losing stage; report its name              | `fs.destination_exists`; the preserved stage holds the loser's complete verified repository | 2    |
| Rename failed (no-replace unsupported)              | **Preserve** the verified stage; report its name            | `fs.rename_unsupported` naming the filesystem limitation and the manual move remediation | 1    |
| Rename failed (unexpected `EXDEV`)                  | **Preserve** the verified stage; report its name            | `fs.commit_failed` (fail closed)                            | 1    |
| Rename failed (other error, classified uncommitted) | **Preserve** the verified stage; report its name            | `fs.commit_failed`                                          | 1    |
| Contradictory or missing identities                 | **Preserve** every reachable object; stop all mutation      | `fs.commit_ambiguous` naming both paths and remediation     | 1    |
| Cancellation before rename issued                   | **Preserve** the stage                                      | Cancellation report with preserved location                 | 130  |
| Cancellation racing rename                          | Classified by the actual rename result per the rows above   |                                                             |      |

A preserved stage contains a complete verified repository; the reported
remediation is a manual rename (`mv <stage> <destination>`) after identity
inspection, or manual removal. Process crash before commit may orphan a
stage; orphans are discoverable by name, and no recovery process exists.

JSON `generate` output includes the commit outcome enum so agents never parse
prose to learn transaction state.

### 31.10 Durability and Concurrency

Staging lives in the destination parent (same filesystem; rename is therefore
atomic). The Foundry issues no fsync; crash-durability of a just-committed
tree follows host filesystem semantics, and the conformance-checked commit
guarantees no partial tree is ever observable at the destination path.
Concurrent Foundry invocations targeting the same destination are safe
without any lock file: at most one exclusive rename wins; the loser fails
with `fs.destination_exists` and its complete verified stage is preserved for
inspection.

## 32. Determinism and Reproducibility

### 32.1 Envelope

Given the same Foundry binary (same embedded catalog), same validated
specification, and same declared tool versions, generation is deterministic
**within** this envelope: identical rendered bytes, identical plan JSON,
identical file set/modes, identical `go.mod`. `go.sum` bytes are identical
when the module proxy state for the pinned versions is identical (the normal
case; all versions are exact). Envelope-external variation (Go patch release
differences in tidy output, proxy withdrawal) is surfaced by the final
conformance check rather than silently absorbed.

### 32.2 Enforced By

- Pure resolver and immutable plan (Sections 27–28).
- Sorted collections everywhere; no map-iteration output anywhere.
- Restricted templates with frozen typed data and two pure functions.
- Exact dependency and tool versions; no version ranges.
- Exact tool environments (Section 34) so host configuration cannot alter
  tool behavior.
- Freeze-and-conform (Section 29.2, stages 12 and 14): every byte at commit
  equals the approved post-tidy tree; the result report records the final
  conformance-baseline digest.
- `-buildvcs=false` on test/vet so an enclosing Git repository around the
  destination parent can never influence builds.
- No timestamps, usernames, hostnames, or locale-dependent content in any
  output.

### 32.3 Verified By

Double-generation byte-equality tests (same host), plan golden tests, and the
CI matrix's cross-platform generation equality checks (modulo documented
`go.sum` envelope terms).

## 33. Dependency and Tool Version Policy

Section 12 is the single normative pin table; catalog manifests MUST match it
byte-for-byte, and CI compares them. This section defines the policy around
those pins.

### 33.1 Exactness

Every dependency, tool, toolchain, and GitHub Action version is exact — no
ranges, no `latest`, no floating tags. Actions are pinned by full commit SHA
with a version comment. The `go.mod` `tool` directives in generated projects
pin Staticcheck and govulncheck at the Section 12 versions.

### 33.2 Update Discipline

Dependabot updates the Foundry repository weekly (grouped). A catalog version
bump is an ordinary reviewed commit: the catalog digest changes, CI
regenerates every golden fixture, and the full generation matrix must pass
before merge. Generated Projects update themselves through their own
Dependabot; the Foundry never touches an existing project (DEC-002).

### 33.3 Version Evidence

The Section 12 pins were verified against primary sources on 2026-07-29
(Stage 4 TV-01–TV-18) and are re-verified by evidence record E5
(Section 51.2) before Phase 1 exit, which guards against staleness between
research and implementation (RSK-311).

### 33.4 Version Lock Manifest

`catalog/versions.toml` is the single machine-readable source for every exact
dependency version, tool version, Go toolchain pin, and third-party GitHub
Action full commit SHA (recorded together with its human-readable tag).
Catalog manifests and rendered files reference these entries; catalog
validation fails when a lock entry is consumed zero times or when any version
string or action reference appears in rendered output without a corresponding
lock entry. Section 12 remains the human-readable normative table; CI
compares the two byte-for-byte so they cannot drift.

### 33.5 Notably Absent

No YAML library, template framework, plugin machinery, dependency-injection
framework, `os/exec` wrapper library, or logging framework beyond stdlib
`log/slog` appears anywhere in the Foundry or generated dependency bills
(Section 44).

## 34. External Tool Execution

Integrates FND-004, FND-005, and FND-007.

### 34.1 Closed World

`generate` executes exactly the plan's `external_steps`: `go mod tidy`,
`go mod verify`, `go test -count=1 -buildvcs=false ./...`,
`go vet -buildvcs=false ./...`, strict-mode `go tool staticcheck ./...` and
`go tool govulncheck ./...`, and optional `git init`. Nothing else, ever. No
step uses a shell. Tools are located once at startup (absolute paths from
PATH resolution, recorded in the plan); preflight runs `go version` and
`git --version` and fails before staging when a tool is missing
(`tool.missing`) or the Go version is not exactly the pinned toolchain
(`tool.wrong_version`), naming the binary and install guidance.

### 34.2 Exact Environments (normative, from FND-005)

Subprocess environments are **constructed from an empty base plus an exact
allowlist** — never inherited-minus-denylist. `cmd/go` honors `GOENV`,
`GOFLAGS`, `GOCACHEPROG`, `GOAUTH`, `GOVCS`, and toolchain switching, so each
of those surfaces is explicitly closed rather than left to inheritance.

Every `go` step runs with exactly this environment:

| Variable      | Value                                                                                  |
| ------------- | --------------------------------------------------------------------------------------- |
| `PATH`        | Host `PATH` (tool location only)                                                        |
| `HOME`        | Host `HOME` (cache roots)                                                               |
| `TMPDIR`      | Host `TMPDIR` when set                                                                  |
| `GOMODCACHE`, `GOCACHE`, `GOPATH` | Host-effective values captured once via `go env` at startup (warm host caches are deliberately reused; `go.sum`/checksum-database verification, not cache isolation, is the integrity boundary) |
| `GOPROXY`, `GOSUMDB` | Host-effective values captured once (network/proxy reachability policy stays the user's)   |
| `GOPRIVATE`, `GONOPROXY`, `GONOSUMDB`, `GOINSECURE` | `` (empty) — every catalog dependency is public; private-module routing and checksum exemptions are closed |
| `GOENV`       | `off` — the persisted Go environment file is never read                                 |
| `GOFLAGS`     | `` (empty) — mode flags like `-mod=mod`/`-mod=readonly` are passed as explicit argv     |
| `GOCACHEPROG` | `` (empty) — no external cache program                                                  |
| `GOTOOLCHAIN` | `local` — no toolchain switching or download                                            |
| `GOWORK`      | `off`                                                                                   |
| `GOVCS`       | `*:off` — no VCS fallback (all pinned dependencies are proxy-resolvable)                |
| `GOAUTH`      | `off` — no credential helpers                                                           |
| `CGO_ENABLED` | `0`                                                                                     |
| `LC_ALL`, `LANG` | `C`                                                                                  |
| `TERM`        | `dumb`                                                                                  |

Everything else — `GODEBUG`, `GOEXPERIMENT`, linker/compiler flag variables,
and all other host variables — is absent because the base is empty. The
constructed set is recorded verbatim in the plan's `external_steps`, so a
behavior difference is diagnosable from the plan alone. Per-step argv carries
`-mod=mod` for tidy and `-mod=readonly` for verify/test/vet/analysis.

`git init` runs with exactly: `PATH`, `LC_ALL=C`, `LANG=C`,
`GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_SYSTEM=/dev/null`,
`GIT_CONFIG_NOSYSTEM=1`, and
`GIT_TEMPLATE_DIR=<Foundry-owned empty scratch dir>`; `HOME`, `GIT_DIR`,
`GIT_WORK_TREE`, and `XDG_CONFIG_HOME` are absent. Invocation:
`git init --initial-branch=<branch> --template=<scratch> .` with the child's
working directory bound to the stage descriptor (Section 34.4). User/system
configuration, host template directories (with their hook scripts), and
`core.hooksPath` therefore cannot influence the created repository. The empty
scratch template directory is created under the Foundry's own temporary root
(mode 0700) — outside the destination-parent namespace — and removed normally
after use (Section 29.2, stage 16).

### 34.3 Timeouts, Output Caps, and Failure

Each step carries a plan-declared timeout and output cap:

| Step                    | Timeout | Notes                                            |
| ----------------------- | ------- | ------------------------------------------------ |
| `go version` preflight  | 1 min   | Exact pinned-version comparison                  |
| `go mod tidy`           | 10 min  | The only cold-cache network-may step             |
| `go mod verify`         | 2 min   | Local after inputs present                       |
| `go test`               | 5 min   | Uncached, `-buildvcs=false`                      |
| `go vet`                | 5 min   | `-buildvcs=false`                                |
| `go tool staticcheck`   | 5 min   | Strict only                                      |
| `go tool govulncheck`   | 10 min  | Strict only; vulnerability-database access       |
| `git --version` / init  | 1 min   | Isolated environment                             |

Stdout and stderr are captured per stream with a 4 MiB cap; truncation is
recorded explicitly (never silent) and replayed only on failure with the step
id and argv. Timeout or non-zero exit fails generation (`tool.failed` /
`tool.timeout`) with the stage preserved. Cancellation sends the context's
kill to the process group after a bounded grace period.

### 34.4 Descriptor-Bound Child Working Directory (normative)

Setting a child's working directory by pathname (`exec.Cmd.Dir = stagePath`)
re-resolves a mutable pathname at process start — the same
time-of-check/time-of-use class as FND-001, at a different boundary. It is
therefore prohibited for transactional children. Instead, for every `go` and
`git` step:

1. Acquire the process-wide child-start mutex (child startup is serialized;
   the Foundry starts no other subprocess concurrently).
2. `fchdir(stage_fd)` — change the Foundry's own working directory through
   the retained stage descriptor, with no pathname resolution.
3. Start the child with **no** `Dir` field; it inherits the descriptor-bound
   working directory.
4. `fchdir(original_fd)` — restore the working directory captured at startup.
5. Release the mutex. If restoration fails, the Foundry fails closed after
   the in-flight step completes rather than continuing with an unknown
   working directory.

The Foundry captures a descriptor for its original working directory once at
startup for step 4. A pathname swap between validation and child start
consequently cannot redirect tool execution: the child's cwd is the exact
staged object the transaction owns. This contract is part of the E1 evidence
record.

### 34.5 Network Truthfulness (from FND-007)

There is no `--offline` flag and no offline mode. `go mod tidy` is declared
`network: may` in every plan; whether it actually touches the network depends
on the user's module cache and proxy configuration, which the Foundry reports
but does not control. `plan` output and pre-staging `generate` output state
this disclosure verbatim. All other steps are declared `network: no` and are
expected to operate from local state; the Foundry does not claim to prevent a
misbehaving tool from attempting network access — the honest contract is
disclosure, not sandboxing.

## 35. Generated Project Verification (During Generation)

### 35.1 Default Gate

In staging, after tidy: gofmt conformance of every rendered Go file (asserted
internally via `go/format` idempotence, not a subprocess), tidy mutation-set
validation (tidy may change only `go.mod`/`go.sum`; the reparsed `go.mod`
must still carry the exact pinned requirements and no `replace`, `exclude`,
or workspace directive — `verify.module_mutation` otherwise), `go mod
verify`, `go test -count=1 -buildvcs=false ./...` (uncached — integrates
FND-006), and `go vet -buildvcs=false ./...`. All must pass or generation
fails with the tool's bounded output and the stage preserved.

### 35.2 Strict Mode

`--verify strict` adds `go tool staticcheck ./...` and
`go tool govulncheck ./...`. Strict is the release-fixture gate in Foundry
CI; default keeps everyday generation fast.

### 35.3 Final Conformance (integrates FND-005)

After the post-tidy freeze, verification steps are *intended* to be
read-only, but the guarantee is checked, not assumed: before commit, every
path, type, mode, and byte under the stage (excluding `.git`) MUST equal the
frozen tree. Any deviation fails `verify.unplanned_mutation`, naming each
divergent path. After `git init`, the non-`.git` comparison is repeated and
`.git` is validated semantically (expected object layout, `HEAD` naming the
configured branch, no hooks with content, empty index).

### 35.4 Race Detector Placement

`go test -race` is not part of the generation gate (CGO and host toolchain
prerequisites would make generation environment-sensitive). Race runs in
Foundry CI against generated fixtures (Section 47) and in generated projects'
strict workflow with an explicit compiler preflight and skip notice
(integrates the FND-008-adjacent CGO reality; see Section 44.3).

## 36. Human Output

### 36.1 Style

Plain line-oriented text on stdout; diagnostics on stderr. No spinners; color
only when stderr is a terminal, `--color` permits it, and `NO_COLOR` is
unset; no interactive prompts ever (DEC-006).

### 36.2 Generate Reporting

`generate` prints: plan summary (files, dependencies, profiles), the network
disclosure, one stable-named progress line per lifecycle stage
(Section 29.1's state names, so logs are diffable), verification results, the
commit outcome, and next steps (`cd`, canonical commands). Every failure
prints: what failed, the exact input location or step, the preserved stage
location when one exists, and the smallest correct remediation.

### 36.3 Quiet and Verbose

`--quiet` suppresses progress and summary lines but never errors, the
preserved-stage location, or the committed destination. `--verbose` adds
diagnostic detail (resolved tool paths, constructed environments, timing) to
stderr. They are mutually exclusive (Section 13.1).

### 36.4 Post-Commit Reporting

After a successful commit, reporting is best-effort: if stdout fails, the
Foundry attempts one concise stderr notice; if both fail, it still exits 0.
Success is never converted to failure by the output channel (FND-012).

### 36.5 SIGPIPE and Broken Streams (normative)

At startup the Foundry subscribes to SIGPIPE with a drained
`signal.Notify` channel. Per Go runtime semantics (SV-04), writes to a broken
stdout/stderr then return `EPIPE` errors instead of killing the process,
which is what makes the Section 31.9 exit contract enforceable rather than
aspirational. Before commit, an `EPIPE` on a required output stream is an
ordinary failure (`report.failed`, exit 1, stage preserved) or cancellation.
After commit, it is absorbed per Section 36.4. Child processes started via
fork/exec receive default signal dispositions (SV-04), so generated tests and
tools keep conventional SIGPIPE behavior; tests prove both properties with
real closed-pipe processes, not only injected writer errors.

## 37. JSON Output

With `--output json`, stdout carries exactly one stable top-level JSON object
(`schema`, `command`, `ok`, `result`, `error`, `warnings`) and **stderr
remains empty** after the flag is recognized: the JSON envelope carries all
structured error detail, so an agent consumes exactly one document from
exactly one stream. Flag-recognition failures before JSON mode is established
report as usage errors (exit 2). `plan --output json` emits the full
Generation Plan. `generate --output json` includes the commit outcome enum
(Section 31.9), per-step results, the preserved stage location when one
exists, and the final conformance-baseline digest. Encoding is deterministic
(stable field order, sorted collections, LF). JSON schemas are versioned with
the plan schema integer; additive evolution only within a major. Exit codes
are identical between output modes. A broken stdout in JSON mode follows
Section 36.5.

## 38. Errors and Exit Codes

### 38.1 Exit Codes

| Code | Meaning                                                        |
| ---- | --------------------------------------------------------------- |
| 0    | Success (including successful commit with failed report stream) |
| 1    | Runtime/system/tool/verification failure                        |
| 2    | Input error: specification, selection, destination, usage       |
| 130  | Cancelled (SIGINT/SIGTERM) before the commit point              |

### 38.2 Error Identity

Every failure carries a stable machine identifier (`domain.reason`), a human
message, the offending location (file:line:column for spec errors; step id
for tool errors; path for filesystem errors), and remediation. The identifier
inventory is normative in Appendix D; notable members:
`spec.unsupported_schema`, `spec.unknown_field`, `spec.duplicate_profile`,
`resolve.unknown_profile`, `resolve.profile_constraint`,
`plan.file_collision`, `fs.unsafe_path`, `fs.namespace_not_private`,
`fs.destination_exists`, `fs.rename_unsupported`, `fs.parent_moved`,
`fs.commit_failed`, `fs.commit_ambiguous`, `render.failed`, `tool.missing`,
`tool.wrong_version`, `tool.timeout`, `tool.failed`, `verify.failed`,
`verify.module_mutation`, `verify.unplanned_mutation`, `git.failed`,
`report.failed`, `catalog.invalid`, `internal.bug`.

### 38.3 Never

Panics as user-visible errors, stack traces by default, partial destination
trees, automatic deletion of preserved stages, or silent fallback behavior.

## 39. Git and GitHub Behavior

The Foundry's entire Git behavior is the optional isolated
`git init -b <branch> --template=<scratch>` in staging (Section 34.2). It
creates no commits, sets no user identity, installs no hooks, adds no
remotes, and never runs any other Git subcommand. GitHub interaction is
entirely owned by generated workflow files; the Foundry itself never calls
any GitHub API. Repository creation, pushing, branch protection, and release
settings are the owner's documented post-generation steps (`docs/releasing.md`
for the distribution profile).

## 40. Provenance

No persistent provenance file is generated (preserved from Stage 4). The
Generation Plan is the inspectable provenance record at generation time;
users who want it persisted redirect `plan --output json` themselves. Generated
files carry no "generated by Foundry" banners except a single comment line in
`AGENTS.md` identifying the Foundry version that produced the initial tree
(informational, never parsed).

## 41. Foundry Versioning

Semantic versioning for the CLI surface, plan JSON schema, and specification
schema together. Schema 1 is the only specification schema; a future schema 2
requires a minor (additive) or major (breaking) release per compatibility
impact. `foundry version` reports version, commit, Go version, and catalog
digest; `--output json` structured. Release binaries embed exact metadata via
build
settings, not linker-flag string injection.

## 42. Foundry Repository Architecture

### 42.1 Canonical Tree

```text
.github/workflows/          # Foundry CI (Section 47)
AGENTS.md                   # Agent instruction authority for the Foundry repo
README.md
catalog/                    # Section 24
cmd/foundry/
  main.go                   # thin process boundary
docs/                       # maintainer docs (architecture, release, testing)
go.mod
go.sum
internal/
  cli/                      # Cobra wiring; flags; output-mode selection
  diagnostic/               # stable error identifiers, source locations, redaction
  spec/                     # TOML parsing, validation, ValidatedSpecification
  catalog/                  # embedded catalog loading + validation + versions.toml
  resolve/                  # flat resolver (Section 27)
  plan/                     # plan construction, validation, serialization
  render/                   # static copy, restricted template, gomod
  fsx/                      # descriptor-relative transaction (Section 31); platform files build-tagged (fsx_linux.go, fsx_darwin.go)
  toolrun/                  # exact-environment subprocess execution; owns the original-cwd descriptor and child-start mutex (Section 34)
  gitinit/                  # isolated git init step
  verify/                   # staging verification and final conformance
  generate/                 # lifecycle orchestration and state machine (Section 29)
  report/                   # human and JSON reporting (consumes typed events/results only)
  version/
integration/                # end-to-end fixtures, incl. extension-path fixtures
  fixtures/
  hostile/                  # hostile filesystem/env/config fixtures
```

Packages exist only when real code exists; no placeholder directories are
created ahead of their phase.

### 42.2 Ownership Rules

`cmd/foundry/main.go` constructs the root command, registers the drained
SIGPIPE subscription, and maps errors to exit codes once. `internal/cli` owns
flags and output mode only. `internal/diagnostic` owns the stable error
identifier registry (uniqueness tested), source locations, and redaction.
`internal/fsx` is the **only** package that performs destination/staging
filesystem mutation — and it contains **no stage deletion code**
(Section 31.6); `internal/toolrun` is the only package that starts
subprocesses and the only owner of the original-working-directory descriptor
and the child-start mutex; a static architecture test enforces all three.
`internal/render` never touches the real filesystem — it writes through the
transaction's rooted writer interface and returns an immutable render
inventory (path, mode, mechanism, rendered-content digest).
`internal/generate` owns the total post-plan state machine and emits typed
events; `internal/report` encodes them and never performs generation work.
Deleted relative to Stage 4: `internal/compose` (profile graph),
`internal/structured` (typed emitters), and every provenance-chain type.

### 42.3 Dependency Direction

Pure flow: `diagnostic → spec/catalog → resolve → render → plan`; side-effect
packages (`fsx`, `toolrun`, `gitinit`, `verify`) consume immutable intent;
`generate` orchestrates them; `cli`/`report` are the outer boundary. Lower
packages never import upward, never import Cobra, and interfaces exist only
where a true external seam exists (process runner, output writers, and a tiny
unexported platform syscall fault-injection seam for tests). No general
filesystem interface, file-sink/placer abstraction, template-provider
interface, or DI container may exist; an architecture test enforces forbidden
imports, and review rejects interfaces with a single production
implementation and no test seam justification.

## 43. Internal Data and Interface Contracts

The load-bearing internal types, in pipeline order:

| Type                      | Producer → Consumer            | Contract                                                                                                            |
| ------------------------- | ------------------------------ | -------------------------------------------------------------------------------------------------------------------- |
| `RawSpecification`        | `spec` parse → `spec` validate | Decoded strict TOML with source positions; no defaults applied                                                        |
| `ValidatedSpecification`  | `spec` → `resolve`             | Immutable; defaults applied; every field within Section 14 constraints                                                |
| `ValidatedCatalog`        | `catalog` → `resolve`          | Immutable; every manifest valid; digest computed; templates parsed                                                    |
| `ResolvedProject`         | `resolve` → `plan`             | Sorted file contributions, dependency set, selected profiles; collision-free                                          |
| `Plan`                    | `plan` → `generate`/`report`   | Section 28; immutable; serializes to stable JSON                                                                      |
| `RenderInventory`         | `render` → `verify`/`plan`     | Immutable per-file inventory: path, mode, mechanism, rendered-content digest                                          |
| `Transaction`             | `fsx` → `generate`             | Holds retained parent/stage handles and recorded stage identity; exposes `RootedWriter`, `DuplicateStageHandle()` (for descriptor-bound child start), `Commit() CommitResult`, and `Close()`; exposes **no** deletion method and never exposes raw paths for mutation |
| `CommitResult`            | `fsx` → `generate`/`report`    | Exactly the Section 31.9 enum with identity-classification detail; carries the preserved-stage name when applicable   |
| `ConformanceBaseline`     | `verify` → `verify`/`report`   | Frozen post-tidy tree (paths, types, modes, byte digests) plus its aggregate digest                                   |
| `StepResult`              | `toolrun` → `verify`/`report`  | Step id, argv, constructed env, duration, exit, bounded output with explicit truncation flags                         |
| `GenerationEvent`         | `generate` → `report`          | Typed lifecycle state transitions (Section 29.1); no output encoding                                                  |
| `FoundryError`            | everywhere → `cli`             | Stable identifier (registry-unique, owned by `diagnostic`), message, location, remediation; wraps cause               |

Invariants: values crossing package boundaries are immutable after
construction and created through constructors that enforce their invariants
and copy mutable collections; contexts flow explicitly as first parameters
only to operations that can block or be canceled and are never stored in
structs; no package-level mutable state; no `panic` across package boundaries
(`internal.bug` wraps recovered orchestration panics at the top level only).

## 44. Dependency Bills of Materials

### 44.1 Foundry Binary BOM

Direct modules exactly as pinned in Section 12.1: `cobra`, `BurntSushi/toml`,
`golang.org/x/mod`, `golang.org/x/sys`; test-only `go-cmp` and
`rogpeppe/go-internal`. Transitive additions are limited to Cobra's
(`spf13/pflag`, `mousetrap`); CI fails if `go mod graph` introduces any new
direct or indirect module not listed in the reviewed BOM file.

### 44.2 Generated Project BOMs

- **Core:** zero third-party runtime modules; tool directives for Staticcheck
  and govulncheck (Section 12.2).
- **CLI:** Cobra (+ transitive pflag/mousetrap); test-only `testscript`.
- **TUI:** Bubble Tea v2, Bubbles v2, Lip Gloss v2 and their transitive Charm
  dependencies, exactly as locked by the golden `go.sum` fixtures.
- **distribution:** no Go modules; GoReleaser/Syft/actions run in CI only.

### 44.3 CGO Policy

Product code, generated projects, analysis, and all release builds use
`CGO_ENABLED=0`. The **only** CGO exception is the Go race detector, which
requires cgo and a host C compiler: race jobs set `CGO_ENABLED=1` after an
explicit compiler preflight and are confined to Foundry CI, generated strict
CI, and explicit maintainer invocation. Race is never part of the generation
gate, never part of release builds, and its absence on a host produces an
explicit skip notice, never a silent pass (from FND-004).

## 45. Testing Strategy

### 45.1 Layers

| Layer                       | Scope and representative tests                                                                                                                                        |
| --------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Unit                        | Spec parsing/validation (table-driven, every error identifier), flat resolver, plan construction, renderer functions, `gomod` generation, commit-result state machine     |
| Golden                      | Plan JSON per fixture; complete generated trees (manifest of path/type/mode + selected full files); guarded update workflow (`REQ-219`)                                    |
| Property/fuzz               | Spec parser fuzzing; destination path normalization fuzzing; template-input fuzzing within the restricted contract                                                        |
| Filesystem/hostile          | Section 31 races: barrier-controlled ancestor and immediate-parent swaps, namespace-custody (owner/mode/sticky) matrices, stage-entry swaps before commit, two-process `EEXIST` commit races with loser-stage preservation asserted, injected ambiguous-rename classification, unexpected `EXDEV`, case-sensitivity matrix, no-replace support probes, cancellation and crash leftovers — all with unrelated sentinel files proven to survive, plus a static audit that no production path deletes a created stage |
| Subprocess                  | Fake-runner unit tests proving exact argv/env/timeouts; real hostile env/config sentinel tests (Section 34.2); descriptor-bound child-cwd tests including pathname swaps during child start and cwd-restore failure; real broken-pipe SIGPIPE tests before and after commit, incl. proof that children retain default SIGPIPE; timeout/cancellation/kill-group and output-cap truncation tests; process-tree audit against the plan |
| Mutation-detection          | A fixture whose generated test mutates staging must fail with `verify.unplanned_mutation`; a cached-then-broken test must still run (`-count=1`)                          |
| Generated-project           | Every golden CLI/TUI fixture compiles, tests, vets, lints cleanly; TUI lifecycle/PTY/debug-log matrices (Sections 18.3, 18.8)                                             |
| End-to-end                  | `testscript` suites driving the real binary across validate/plan/generate/cancel/failure paths on macOS and Linux                                                          |
| Agent acceptance            | Recorded Grok Build/Codex/Cursor task scenarios against generated projects (Section 52)                                                                                    |

### 45.2 Placement Discipline

Every normative "MUST" in Sections 13–41 maps to at least one test layer in
the traceability matrix (Section 54). Tests for the transaction run on both
platforms in CI; local `go test ./...` covers everything not requiring a
second platform.

### 45.3 What Was Deleted

Graph-property suites for profile closure/cycles/capabilities, token-scanner
tests, typed YAML emitter tests, the case-fold scan matrix, and every
stage-cleanup swap test — all removed with their machinery (there is no
cleanup code to test; the replacement is the no-stage-delete static audit and
preservation-reporting tests).

### 45.4 Golden Update Discipline

Goldens are checked-in explicit files with normalized paths/modes and no
timestamps. Updates require the test-only environment variable
`UPDATE_GOLDEN=1`, update one named suite at a time, print every changed
path, and refuse bulk updates above a documented suite threshold without a
second explicit opt-in — so golden convenience can never mass-approve an
architectural change silently.

## 46. Security Model

### 46.1 Trust Boundaries

Trusted: the Foundry binary and embedded catalog, the host `go` and `git`
binaries, the host C compiler (race jobs only), root, any process running as
the same effective user, and any principal already authorized to write the
destination's parent chain (the explicit trusted-host boundary the
namespace-custody check enforces — Section 31.3). Untrusted: the
specification file, the destination path and every parent component, the
filesystem's concurrent unprivileged mutators, host Go/Git configuration and
environment, and generated-test behavior.

### 46.2 Controls

- Descriptor-relative transaction (Section 31): no check-to-use pathname gap;
  namespace-custody check; rooted writes with `fchmod`-exact modes;
  descriptor-bound child working directories; exclusive no-replace commit
  with post-syscall identity classification.
- No automatic deletion (Section 31.6): the highest-impact operation class
  does not exist; every failed stage is preserved 0700 and reported.
- Closed subprocess environments (Section 34.2): host Go/Git config,
  credential helpers, cache programs, private-module routing, and hook
  templates cannot execute or leak.
- SIGPIPE containment (Section 36.5): a broken pipe cannot kill the Foundry
  mid-transaction or falsify the exit contract; children keep default
  behavior.
- Restricted rendering (Section 26): no template escape to filesystem,
  environment, network, or exec; no shell anywhere in the product.
- Final conformance (Section 35.3): generated tests cannot alter committed
  bytes.
- Catalog validation (Section 24.4): embedded content cannot smuggle unsafe
  paths or binaries; the digest binds binary to source.
- Secret hygiene: schema has no secret fields; diagnostics redact
  known-sensitive keys (environment, proxy, credential values) with sentinel
  tests; generated projects never log secrets by construction; debug logs are
  safe-basename exclusive-create 0600 (Section 18.8).
- Generated CI: least permissions, full-SHA pins, no untrusted-PR release
  execution (Sections 16.6, 20).

Each control links to at least one automated test in the Section 45 layers;
the threat-to-control-to-test mapping is part of Phase 2 exit review.

### 46.3 Explicit Non-Goals

No sandbox for generated tests beyond staging conformance, no defense against
a privileged local attacker, no network isolation promise, no supply-chain
attestations for the Foundry itself before Phase 4.

## 47. CI and Automation (Foundry Repository)

### 47.1 Matrix

The Foundry repository — unlike generated private projects — owns
cross-platform filesystem behavior and therefore keeps a larger matrix:

| Workflow            | Trigger                | Jobs                                                                                                                                |
| ------------------- | ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| `ci.yml`            | PR + default branch    | Linux and macOS: format, `go test -count=1 ./...`, vet, Staticcheck; Linux: race (`CGO_ENABLED=1`, preflight); catalog-digest check against the tree; golden generation matrix |
| `strict.yml`        | Weekly + manual        | govulncheck; bounded fuzz corpus run; cold-cache generation timing baseline; full hostile filesystem suite                            |
| `release.yml`       | Tag (Phase 4)          | GoReleaser snapshot/publish, `CGO_ENABLED=0` matrix builds, SBOM, attestations, install smoke tests                                   |

All actions full-SHA pinned; least permissions; Dependabot weekly grouped.

### 47.2 Fixture Regeneration

Any catalog or renderer change regenerates every golden fixture in CI and
fails on undocumented diffs. The guarded update command requires an explicit
flag and produces a reviewable diff; CI never auto-updates goldens.

## 48. Foundry Installation and Distribution

Until Phase 4, installation is `go install` from the repository (or a local
build); the Foundry is its own first distribution dogfood. Phase 4 applies the
distribution profile's own architecture to the Foundry repository:
GoReleaser-built `CGO_ENABLED=0` binaries for darwin/linux × amd64/arm64,
checksums, SBOM, artifact attestations, and immutable-release settings.
`OQ-300` (canonical public module path) is resolved at Phase 4 entry. There is
no self-update mechanism, package-manager submission, or install script
(Sections 57–58).

## 49. Performance and Resource Expectations

Correctness and generated-project quality outrank micro-optimization.
`validate`, `plan`, `catalog`, and `version` are in-process, tool-free, and
SHOULD feel immediate; they never start subprocesses or walk unrelated trees.
Generation latency is dominated by module download and verification, so
per-stage durations are reported separately rather than hidden in one total.

| Area            | Expectation                                                                                             |
| --------------- | -------------------------------------------------------------------------------------------------------- |
| Startup         | Single process; no daemon; embedded catalog parsed lazily once per invocation                             |
| Memory          | Proportional to catalog metadata plus the largest single rendered file                                    |
| Binary size     | Recorded per release; unexplained >2× growth from the accepted baseline blocks release review             |
| Catalog size    | Two archetypes and one post-MVP profile; additions require evidence and matrix-impact review              |
| CI runtime      | Any >2× regression in a stable job triggers diagnosis before caching tricks or check weakening            |
| Baselines       | Warm/cold generation timings captured during Phase 2 dogfood on the owner's macOS machine and Linux CI    |

No absolute millisecond/memory gate is invented before those measurements
exist (they were previously assigned to the deleted Phase 0; they now belong
to Phase 2 dogfood).

## 50. Minimum Viable Foundry

The MVP is the Phase 3 exit state:

1. All public commands complete per Section 13 on macOS and Linux.
2. `generate` produces both archetypes with `profiles = []`, fully verified,
   Git-initialized, committed atomically per Section 31.
3. Generated projects pass their own reduced CI (Section 16.6) unmodified.
4. Dogfood fixtures `foundry-smoke-cli` and `foundry-smoke-tui` exist, and
   real dogfood projects `repo-map` (CLI) and `worktree-status` (TUI) have
   begun (Section 52).
5. Agent acceptance (Section 52.3) passes for Grok Build, Codex, and Cursor.

Explicitly **not** in the MVP: any optional profile, the distribution
workflow, public release of the Foundry itself, and Homebrew or any other
install channel.

## 51. Implementation Phases

### 51.1 Phase Table

Phases are strictly ordered, acyclic, and none is authorized to amend this
specification (defects found during implementation go through ordinary
artifact revision).

| Phase | Name                                            | Scope                                                                                                                        | Entry criteria                                                        | Exit criteria (acceptance evidence)                                                                                                                                    |
| ----- | ----------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 1     | Specification, Catalog, Resolution, Planning    | `diagnostic`, `spec`, `catalog`, `resolve`, `render` (pure), `plan`, `report`, `cli` for the write-free commands                | This artifact is `Accepted — implementation authority` (it is)          | `validate`/`plan`/`catalog`/`version` complete with golden plan JSON, full error-identifier coverage, zero writes/subprocesses/network proven by tests, every unimplemented profile ID (incl. `configuration`, `local-persistence`) rejected; **E5 recorded**              |
| 2     | Transactional CLI Generation and First Dogfood  | `fsx`, `toolrun`, `gitinit`, `verify`, `generate`; CLI archetype end-to-end; hostile suite; performance baselines               | Phase 1 exit; **E1, E2, E3 recorded** before any transaction/tool code   | CLI generation passes default+strict on macOS/Linux incl. hostile/race/preservation suites; `foundry-smoke-cli` and real `repo-map` created and exercised (dogfood begins the moment one CLI fixture passes real-platform transaction + default verification); commit-matrix tests green |
| 3     | TUI and MVP                                     | TUI archetype, lifecycle/PTY/debug-log matrices; generated CI; agent acceptance; MVP closure                                    | Phase 2 exit without a blocking simplification trigger; **E4 recorded**  | Section 50 MVP list complete; `foundry-smoke-tui` + `worktree-status` begun; recorded three-agent acceptance evidence                                                      |
| 4     | Distribution and Release Hardening              | `distribution` profile, release workflows, Foundry self-distribution, `OQ-300` resolution, first public tag dogfood             | Phase 3 exit incl. dogfood triggers resolved; `OQ-300` resolved before publication | Distribution fixture releases end-to-end in a scratch repository incl. private-selection negative cases; Foundry release snapshot green; license-before-publication dogfood recorded |

### 51.2 Phase-Entry Evidence Gates (replaces Phase 0; from FND-011)

These five bounded evidence items MUST be executed and recorded (Appendix I
templates) at the phase boundaries below. They **verify** the architecture
fixed in this document; none may change it silently. A contradicting result
forces an ordinary reviewed revision of this artifact before the gated phase
proceeds — never a pathname fallback, automatic stage deletion, or a new
subsystem. Gates may not be waived, reordered past their phase, or satisfied
by prose.

| ID | Evidence item                                                                                                                                                                                                                 | Bound     | Gates                          | Resolves          |
| -- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------- | ------------------------------ | ----------------- |
| E1 | Descriptor-relative transaction spike: no-follow parent walk with custody check, handle-relative stage creation, descriptor-bound child cwd (incl. pathname swap during child start), `Renameat2`/`RenameatxNp(RENAME_EXCL \| RENAME_NOFOLLOW_ANY)` no-replace commit and identity classification on APFS, ext4, xfs, btrfs, plus one FAT/exFAT negative probe | ≤ 3 days  | Phase 2 entry                  | `OQ-400`, RSK-400 |
| E2 | Exact Go/Git environment isolation spike: sentinel `GOENV`/`GOFLAGS`/`GOCACHEPROG`/`GOAUTH`/`GOVCS`/private-module/git-config/template fixtures prove nothing leaks into tool behavior                                        | ≤ 2 days  | Phase 2 entry                  | FND-005 evidence  |
| E3 | Race prerequisite probe: compiler preflight logic and known-race fixture on the owner's macOS machine and Linux CI                                                                                                            | ≤ 1 day   | Phase 2 entry                  | FND-004 evidence  |
| E4 | Bubble Tea v2 lifecycle spike: `WithContext` + `WithoutSignalHandler` + effect-completion tracking + PTY restoration on quit/Ctrl-C/signal/panic                                                                              | ≤ 2 days  | Phase 3 entry                  | FND-010 evidence  |
| E5 | Version/action lock: re-verify every Section 12 pin and every `catalog/versions.toml` action SHA against primary sources                                                                                                      | ≤ 1 day   | Phase 1 exit                   | RSK-311           |

### 51.3 Reversibility

Until Phase 3 exit, these remain deliberately reversible: exact commit
primitive shape (within Section 31 semantics), verification step ordering,
generated CI job composition, and TUI file layout inside `internal/tui`.
Locked decisions (Section 10) are not reversible through implementation.

## 52. Dogfooding and Agent Acceptance

### 52.1 Named Sequence

Dogfooding proceeds in order: generated fixtures `foundry-smoke-cli` and
`foundry-smoke-tui` (disposable), then real projects `repo-map` (repository
inventory CLI, text/JSON output; adds its first domain command manually) and
`worktree-status` (Git worktree TUI; adds real cooperative asynchronous
refresh manually — the first genuine effect code). Dogfood repositories stay
independent and evolve manually; the Foundry never modifies an existing
project, so comparisons use freshly generated sibling repositories. CLI
dogfood begins inside Phase 2 as soon as one CLI fixture passes the
real-platform transaction plus default verification; TUI dogfood begins
inside Phase 3 as soon as the static TUI passes lifecycle/PTY/debug-log
tests.

### 52.2 Measurements and Simplification Triggers

Each dogfood repository records: orientation files an agent reads before its
first correct edit; attempts/time to first correct edit; package-placement
errors; generated non-test files retained versus deleted (target: zero
mandatory deletions — the FND-014 metric); first-feature diff size and shape;
generation and verification latency (warm/cold); median PR CI latency
(FND-016 metric); default-versus-strict failure split; any demand for a
verification bypass; escaped defects that a strict-cadence check would have
caught earlier (RSK-403); recurring manual configuration/persistence patterns
(future-profile evidence, RSK-401); manual preserved-stage cleanup burden
(RSK-310); and agent observations.

**Mandatory simplification triggers (normative):** deletion of more than one
quarter of generated non-test files in a real project, deletion of the same
generated file in both real projects, or repeated agent boundary confusion on
the same structure MUST trigger a focused simplification review — and any
resulting specification correction — before Phase 4 begins. Metrics turn
dead-scaffolding and framework-creep concerns into evidence instead of taste.

### 52.3 Agent Acceptance

Against fresh generated CLI and TUI projects, Grok Build (primary), Codex
(second independent), and Cursor (third) must each complete recorded
scenarios using only repository contents:

1. **Orient:** locate the instruction authority and name the canonical
   commands and the correct package for a described change.
2. **CLI change:** add a bounded subcommand behavior in the correct package
   with tests.
3. **TUI change:** add a bounded key/view behavior in the correct package
   with tests.
4. **Repair:** diagnose and fix an injected failing test using documented
   commands only.
5. **Boundary decision:** add a side-effect boundary package following the
   documented growth rules.
6. **Report:** produce the change-completion report (files, commands,
   results, risks) exactly per `AGENTS.md`.

The dogfood record includes prompts, inspected paths, diffs, commands,
failures, and a reviewer assessment per agent. Failure caused by generated
docs or structure is a revision trigger, not a waiver. No vendor-specific
repository files are added to make scenarios pass (DEC-015).

## 53. Normative Requirements

This specification defines **125 active** normative requirements, preserving
every Stage 4 identifier for its original subject; no identifier is retired,
renumbered, or reused (`REQ-073`–`REQ-075` remain active as
exclusion-and-recipe requirements for their original subjects). Statements
below are the normative text; the sections cited beside each band carry the
full detail and control on elaboration. Phase codes: **S6** = Stage 6
evidence gates (Section 51.2); **P1–P4** = implementation phases
(Section 51.1). Sources and risk linkage per requirement are in Section 54.

### 53.1 Product Scope and Global Invariants (REQ-001–REQ-012; Sections 8–10)

| ID      | Normative requirement                                                                                                                                                                                                                                 | Phase | Verification                                                                          |
| ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- | --------------------------------------------------------------------------------------- |
| REQ-001 | The Foundry MUST be a personal, repository-first tool creating brand-new Go application repositories, optimized first for the owner's macOS/Linux workflow.                                                                                                | P1    | Specification inspection; end-to-end generation tests; dogfood repositories             |
| REQ-002 | The Foundry MUST be implemented in Go, and every Generated Project MUST be a single-module Go repository pinned `go 1.26.0` / `toolchain go1.26.5` in catalog v1.0.                                                                                       | P1    | Compilation of Foundry and every golden project with the pinned toolchain                |
| REQ-003 | The Foundry MUST NOT modify, merge into, upgrade, synchronize, migrate, or add profiles to any existing project or existing destination.                                                                                                                  | P2    | Destination-refusal tests: existing file/dir/symlink/repo/empty dir all fail before staging |
| REQ-004 | Supported platforms are macOS and Linux only. Product, generated, analysis, and release builds MUST use `CGO_ENABLED=0`; the race detector is the sole exception, running with `CGO_ENABLED=1` only after compiler preflight and only in CI or explicit maintainer runs (Section 44.3). | P2    | Release-matrix build inspection; preflighted race jobs; known-race fixture               |
| REQ-005 | Every Generated Project MUST have exactly one Project Archetype, one primary interaction model, and one primary binary; no helper binary exists in schema 1.                                                                                              | P1    | Resolver tests; catalog inspection; generated tree goldens                               |
| REQ-006 | Generated Projects MUST have no runtime, build, or toolchain dependency on the Foundry and MUST remain fully usable if the Foundry disappears.                                                                                                            | P2    | Generated `go.mod` inspection; independent clone build/test                              |
| REQ-007 | The Foundry MUST operate non-interactively (no prompts, no terminal reads beyond explicit `--spec -`) and MUST support optional isolated `git init` as its only Git action.                                                                               | P1    | Non-interactive process tests; Git behavior tests (Section 39)                           |
| REQ-008 | The Foundry MUST NOT call any GitHub API; GitHub interaction belongs exclusively to generated workflow files and documented owner steps.                                                                                                                  | P1    | Static import/network audit; process tests on network-disabled hosts                     |
| REQ-009 | Every Generated Project MUST carry the portable agent instruction contract: `AGENTS.md` as single agent authority with canonical commands, architecture map, and change-completion report contract, targeting Grok Build first and Codex/Cursor as first-class secondary, with no Claude-specific files. | P3    | Generated docs inspection; three-agent acceptance scenarios (Section 52.3)               |
| REQ-010 | Generation MUST be deterministic within the Section 32.1 envelope and strict: verification failures fail generation; there is no non-strict emit mode.                                                                                                    | P2    | Double-generation byte equality; verification-failure tests                              |
| REQ-011 | Catalog v1.0 MUST contain exactly the `cli` and `tui` archetypes and exactly one optional profile, `distribution`, implemented post-MVP; the MVP implements no optional profile, and `configuration`/`local-persistence` are recipes, not profiles.        | P1    | `catalog list` golden; MVP acceptance with `profiles = []`                               |
| REQ-012 | The Foundry MUST NOT implement plugins, remote catalogs, template downloads, profile scripts, self-update, project upgrade/sync, interactive wizards, or a daemon.                                                                                        | P1    | Command-surface tests; static audit; Section 58 register                                 |

### 53.2 Command Surface and Semantics (REQ-030–REQ-037; Sections 13, 30)

| ID      | Normative requirement                                                                                                                                                                                                              | Phase | Verification                                                                 |
| ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- | ------------------------------------------------------------------------------ |
| REQ-030 | The command surface MUST be `init`, `validate`, `plan`, `generate`, `catalog list`, `catalog show`, `doctor`, `version` with global flags `--output text\|json`, `--quiet`, `--verbose`, `--color` per Section 13.1 (quiet/verbose mutually exclusive; JSON mode rejects human-mode flags). No `--offline`, `--force`, or bypass flag exists anywhere; `--spec` and `init --out` are always explicit with no implicit discovery. | P1    | Help/usage goldens; flag-conflict rejection tests; no `offline` token in surface |
| REQ-031 | `generate` MUST be the only command that writes a Generated Project, starts generate-pipeline subprocesses, or may use the network for generation. Exceptions: `init` may create one Project Spec TOML at an explicit `--out` (no overwrite); `doctor` may probe `go version` under a closed environment. All other commands MUST be proven write-, subprocess-, and network-free (read-only destination observation excepted). | P1    | Syscall/subprocess audit tests; network-disabled host runs; init overwrite refusal |
| REQ-032 | `validate` MUST execute the complete pure pipeline (parse, validate, resolve, plan construction, non-binding destination observation) through the same code path as `plan`, discard the plan, aggregate independent field errors in source order, and exit 0/1/2/130. | P1    | Table-driven validation tests over every error identifier; validate/plan single-code-path test |
| REQ-033 | `plan` MUST be the authoritative dry run: it produces the complete Generation Plan without side effects, accepts `--verify` so the recorded step list matches generation, and is byte-equal to the plan `generate` executes; no separate dry-run command or flag exists. | P1    | Plan golden tests; plan/generate equality tests; absence-of-writes audit         |
| REQ-034 | `generate` MUST execute exactly the plan (Section 29), disclose possible network steps before staging, and place output only through the Section 31 transaction.                                                                          | P2    | End-to-end generation tests; disclosure output goldens; process-tree audit       |
| REQ-035 | `catalog list`/`catalog show` MUST render only embedded catalog metadata (including `core`); `version` MUST report version, commit, Go version, catalog digest, and catalog Go pin / `FOUNDRY_GO_BIN` guidance; `doctor` MUST report pin / path / guidance without failing validate/plan. | P1    | Output goldens against embedded catalog                                          |
| REQ-036 | The Foundry MUST own SIGINT/SIGTERM through one `signal.NotifyContext`; cancellation before any stage exists exits 130 with nothing on disk; cancellation after stage creation stops tools, **preserves the stage**, reports its location, and exits 130; cancellation racing commit is classified solely by the commit result (Section 31.9). | P2    | Cancellation-injection tests at every lifecycle stage with exact no-stage/preserved-stage assertions; commit-race tests |
| REQ-037 | `--output json` MUST emit exactly one stable versioned deterministic JSON document on stdout with empty stderr after flag recognition and identical exit codes to text mode.                                                              | P1    | JSON schema tests; stream-separation and empty-stderr tests                      |

### 53.3 Project Specification (REQ-038–REQ-046; Sections 14–15)

| ID      | Normative requirement                                                                                                                                                                                                                        | Phase | Verification                                                              |
| ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- | ---------------------------------------------------------------------------- |
| REQ-038 | The Project Specification MUST be one strict TOML document ≤ 1 MiB, UTF-8, canonically `foundry.toml`, that is input only and never copied into output.                                                                                          | P1    | Parser tests incl. size/encoding limits                                       |
| REQ-039 | The specification MUST contain exactly one top-level integer `schema = 1` whose physical position has no semantic effect; the field contract is exactly Section 14.3, with unknown schema values failing `spec.unsupported_schema`.             | P1    | Schema-position permutation tests; field-contract table tests                 |
| REQ-040 | Decoding MUST be strict: unknown fields/tables and duplicate keys are fatal with name and line/column; no interpolation, includes, environment references, or expressions exist; field order and formatting never affect resolution.            | P1    | Hostile-input decoding tests                                                  |
| REQ-041 | The schema MUST define no credential/token/secret field, and documentation MUST direct secrets to runtime environment injection only.                                                                                                           | P1    | Schema inspection; docs inspection                                            |
| REQ-042 | `name` MUST be lowercase ASCII kebab-case starting with a letter, 1–63 bytes, and MUST equal the destination basename; `binary` defaults to `name` with the same rules; `description` MUST be a trimmed single-line UTF-8 string of at most 200 bytes.  | P1    | Name/description-validation table tests                                       |
| REQ-043 | `module` MUST pass `golang.org/x/mod/module.CheckPath` with final segment equal to `name` and no semantic import-version suffix; ownership/reachability are not verified.                                                                       | P1    | Module-path validation tests incl. `/vN` rejection                            |
| REQ-044 | The destination MUST satisfy Section 15.3, its parent MUST be acquired only by the no-follow component walk with a retained handle and the namespace-custody check (Sections 31.2–31.3), any parent symlink MUST fail `fs.unsafe_path`, any shared-writable non-sticky component MUST fail `fs.namespace_not_private`, and no synthetic case-fold sibling scan exists. | P2    | Hostile-path suite; custody owner/mode/sticky matrix; parent-swap race tests; case-sensitivity matrix |
| REQ-045 | `visibility` MUST be `private` (default) or `public`; `profiles` MUST list only exact implemented built-in IDs (none in the MVP; `distribution` post-MVP), with duplicates and unknown IDs fatal and order semantically irrelevant.              | P1    | Resolution error tests; MVP empty-profile acceptance                          |
| REQ-046 | `[git] init` (default true) and `[git] initial_branch` (default `main`, lowercase kebab-case) MUST be the only Git fields; everything after generation is the owner's authority.                                                                | P1    | Field tests; generated-repo inspection                                        |

### 53.4 Generated Core and Archetypes (REQ-060–REQ-072; Sections 16–18)

| ID      | Normative requirement                                                                                                                                                                                                                                                       | Phase | Verification                                                                    |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- | ---------------------------------------------------------------------------------- |
| REQ-060 | Core MUST be exactly the Section 16.2 inventory: zero third-party runtime dependencies, no task runner, no `pkg/`/`util/`-style buckets, no license file, no provenance file, no demo behavior, and no dead scaffolding.                                                           | P2    | Generated tree goldens; runtime-dependency audit                                    |
| REQ-061 | Generated documentation MUST follow Section 16.3 ownership: `AGENTS.md` is the single agent authority; `README.md`, `docs/architecture.md`, `docs/commands.md`, `docs/testing.md` (TUI adds `docs/ui-architecture.md`) each own one role without duplicated authority.             | P2    | Docs-ownership inspection tests; cross-file command-consistency checks              |
| REQ-062 | Canonical commands MUST be direct Go tool invocations documented identically in `AGENTS.md`, `docs/commands.md`, and CI, using `go test -count=1 ./...` and declaring Staticcheck/govulncheck as pinned `go.mod` tool directives.                                                  | P2    | Command-consistency tests across docs and workflows                                 |
| REQ-063 | Generated Core CI MUST be exactly one required fast Linux PR/default job (format, mod verify, uncached tests, vet, Staticcheck; superseded-run cancellation) plus one weekly/manual strict workflow (govulncheck; preflighted race; representative macOS; bounded fuzz when targets exist), with documented risk-based promotion. | P3    | Generated workflow goldens; single-required-job assertion; dogfood latency metrics  |
| REQ-064 | The generated CLI MUST match Section 17.2: a minimal working shell (root help, `version`, hidden completion) with thin `main.go`, single `os.Exit` site, and **no** demo domain service.                                                                                            | P2    | CLI tree goldens; compile/run/test of fresh output                                  |
| REQ-065 | CLI commands MUST be constructed by functions without package-level command globals or `init` registration; Cobra confined to `internal/cli`; errors wrapped and translated once at the boundary; exits 0/1/2/130; stdout for output, stderr for diagnostics.                       | P2    | Generated-code static checks; process tests                                         |
| REQ-066 | The generated CLI MUST ship unit tests beside code, constructor-level command tests, and `testscript` process tests for help/version/completion; richer extension examples live in Foundry fixtures and written recipes, not generated code.                                        | P2    | Generated tests pass; fixture-based extension-path tests                            |
| REQ-067 | CLI side-effect boundaries MUST be added as responsibility-named `internal/` packages only when actually needed; none are pre-generated; growth rules are documented in `docs/architecture.md`.                                                                                    | P2    | Generated tree audit; docs inspection; dogfood change scenarios                     |
| REQ-068 | The generated TUI MUST match Section 18.2: one `internal/tui` package with explicit file responsibilities and a minimal static help/quit screen using the Bubbles help component; **no** synthetic asynchronous initialization, fake loading cycle, or empty `effects.go`/`messages.go` scaffolding. | P3    | TUI tree goldens incl. absence checks; compile/run/test of fresh output             |
| REQ-069 | TUI state/update/view MUST follow Section 18.4: one model type, one transition site, pure view, centralized theme and keymap, explicit minimum dimensions with a deterministic too-small notice; typed messages appear only with the first real asynchronous feature.              | P3    | Generated-code static checks; render goldens incl. minimum-size cases               |
| REQ-070 | The TUI MUST have exactly one lifecycle owner per Section 18.3: `signal.NotifyContext` in `main`, `tea.WithContext(appCtx)` + `tea.WithoutSignalHandler()`, framework panic recovery kept enabled, quit paths cancel before `tea.Quit`, exit classification `q`→0 / Ctrl-C-or-signal→130 / failure→1, and startup flags exactly `--version`, `--no-color`, `--debug-log`. | P3    | Lifecycle tests incl. Ctrl-C-as-key; static option checks; PTY restoration tests    |
| REQ-071 | Future effects MUST follow the Section 18.7 cooperative rules (documented and fixture-proven), and `--debug-log` MUST accept only a safe basename resolved against a startup-captured directory descriptor, creating only a new regular file via `O_CREATE\|O_EXCL\|O_WRONLY\|O_NOFOLLOW` 0600 with `fchmod` and descriptor-verified type, refusing every existing or non-regular target, never creating parents, with `slog` JSON and redaction. | P3    | Fixture effect-completion tests; debug-log safety matrix incl. separator/traversal arguments (Section 18.8) |
| REQ-072 | The generated TUI MUST ship model/update unit tests, view goldens (incl. no-color, minimum and too-small sizes), lifecycle tests, PTY restoration smoke tests, and the debug-log matrix; growth rules live in `docs/ui-architecture.md`.                                            | P3    | Generated test suite passes in staging and CI                                       |

### 53.5 Capability Profiles and Classification (REQ-073–REQ-078; Sections 19–23)

| ID      | Normative requirement                                                                                                                                                                                                                                | Phase | Verification                                                                    |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----- | ---------------------------------------------------------------------------------- |
| REQ-073 | `configuration` MUST NOT be a schema-1 profile ID: the catalog MUST NOT contain it, selecting it MUST fail `resolve.unknown_profile`, and the configuration capability is delivered only as the Section 21.1 recipe. The identifier keeps its original subject.                    | P1    | Unknown-profile rejection test; catalog absence inspection; recipe review           |
| REQ-074 | The configuration recipe MUST bound its guidance to Section 21.1 (flags > env > one optional TOML file; `caarlos0/env` + `BurntSushi/toml`; secrets via environment only; 0600 sensitive files) and MUST NOT ship generated configuration files or schema fields.                  | P1    | Recipe content review; generated-output absence checks                              |
| REQ-075 | `local-persistence` MUST NOT be a schema-1 profile ID: the catalog MUST NOT contain it, selecting it MUST fail `resolve.unknown_profile`, and persistence is delivered only as the Section 21.2 recipe (files first; bbolt on demonstrated transactional need).                    | P1    | Unknown-profile rejection test; generated-tree/module absence checks; recipe review |
| REQ-076 | The `distribution` profile MUST implement exactly the Section 20 contract (owned files incl. `CONTRIBUTING.md`/`SECURITY.md`; public + GitHub-module direct predicate); generation MUST NOT inspect, generate, or validate a license, and generated release documentation MUST conspicuously block publication until the owner adds and reviews one. | P4    | Public generation and private/non-GitHub negative tests; release-docs inspection; release dogfood |
| REQ-077 | Profiles MUST be selected as a flat set of exact implemented built-in IDs with only direct archetype/visibility/module-host predicates; schema 1 MUST NOT contain transitive requirements, conflicts, capability registries, helper-binary fields, or provenance chains.           | P1    | Manifest/plan schema tests proving field absence; flat resolver tests               |
| REQ-078 | New capability candidates MUST be classified per Section 19.2 and admitted only through the full Section 19.4 admission test, via explicit revision of this specification.                                                                               | P4    | Dogfood pattern measurements (`REQ-247`); admission-test record; artifact-revision discipline |

### 53.6 Catalog, Ownership, Rendering, and Resolution (REQ-090–REQ-101; Sections 24–27)

| ID      | Normative requirement                                                                                                                                                                                                            | Phase | Verification                                                          |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----- | ------------------------------------------------------------------------ |
| REQ-090 | The catalog MUST be Git-visible source embedded via `go:embed`, versioned with the Foundry, not user-extensible, with a deterministic SHA-256 digest reported by `version` and every plan and compared in CI; `catalog/versions.toml` MUST be the single lock manifest with every entry consumed exactly once (Section 33.4). | P1    | Digest tests; CI tree-vs-embed comparison; lock-manifest consumption tests |
| REQ-091 | Filesystem catalog loading MUST exist only under the `foundrydev` build tag, apply full validation, and fail closed; release binaries MUST NOT contain it.                                                                              | P1    | Build-tag tests; release-binary audit                                     |
| REQ-092 | Catalog manifests MUST declare exactly the Section 24.3 fields; `requires`, `conflicts`, `capabilities`, `provides`, `helper_binary`, and render modes beyond `static`/`template` MUST NOT exist.                                        | P1    | Manifest schema tests; unknown-field rejection                            |
| REQ-093 | Every ordinary output file MUST have exactly one owner; any duplicate output path — including byte-identical content — MUST fail `plan.file_collision` naming every claimant.                                                            | P1    | Collision tests incl. parent/child conflicts                              |
| REQ-094 | Output paths MUST be relative, safe (no `..`, no absolute, no symlink), files 0644, directories 0755 created implicitly, staging 0700; no empty directories, binary assets, or executables in catalog v1.0.                              | P1    | Catalog validation tests; generated tree audits                           |
| REQ-095 | `go.mod` MUST be the only file with multiple semantic contributors, produced solely by the typed `modfile` generator with fatal same-module version conflicts; `go.sum` comes only from `go mod tidy`.                                   | P2    | gomod generator unit tests; conflict tests                                |
| REQ-096 | Exactly three rendering mechanisms MUST exist — static copy, restricted complete-file template, typed `go.mod` — with no token-substitution language and no typed YAML/GoReleaser/`.gitignore`/Dependabot/workflow emitters.              | P2    | Render-strategy enum tests; native-tool parse of generated workflow files |
| REQ-097 | Restricted templates MUST use `[[`/`]]`, `missingkey=error`, frozen typed data, exactly the `join`/`quote` functions, and no partials, includes, reflection, environment, filesystem, time, randomness, network, or exec.                | P2    | Template-contract tests; hostile-template catalog validation              |
| REQ-098 | The resolver MUST be a pure function of validated specification and catalog with no filesystem, environment, subprocess, clock, logging, or output, returning immutable results with stable sorting.                                     | P1    | Purity tests (fake-free unit tests); determinism property tests           |
| REQ-099 | Resolution MUST implement exactly the Section 23.2 rules — unknown/duplicate/incompatible selection failures with exact identifiers — and MUST NOT implement graph traversal, cycle detection, or capability mapping.                    | P1    | Flat-resolution table tests; static absence audit                         |
| REQ-100 | All resolved and planned collections MUST be deterministically ordered (paths, profile IDs, modules); no map-iteration order may reach any output; no provenance-chain structures exist.                                                | P1    | Repeated-resolution equality tests; plan byte-equality                    |
| REQ-101 | The Foundry MUST NOT implement a general constraint solver, fuzzy matching, or "did you mean" search; unknown identifiers fail with the exact sorted available set.                                                                     | P1    | Error-output goldens                                                      |

### 53.7 Plan, Lifecycle, Transaction, and Determinism (REQ-120–REQ-135; Sections 28–33)

| ID      | Normative requirement                                                                                                                                                                                                                                                            | Phase | Verification                                                                     |
| ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- | ------------------------------------------------------------------------------------ |
| REQ-120 | The Generation Plan MUST be a first-class immutable value produced identically by `plan` and `generate`; `generate` MUST execute exactly the plan and nothing else.                                                                                                                       | P1    | Plan-equality tests between commands; process-tree audit against `external_steps`      |
| REQ-121 | The plan MUST contain exactly the Section 28.2 fields — including per-file source and planned-content digests, exact external-step argv/environment/timeout/output caps, tool-output declarations, the final-conformance verification step, the commit-result model, and the plan digest — and MUST NOT contain offline fields, capability lists, or provenance closures. | P1    | Plan JSON schema tests; golden plans; digest-conformance tests                         |
| REQ-122 | The plan MUST serialize to byte-identical JSON for identical validated inputs (stable field order, sorted collections) and MUST be immutable after construction.                                                                                                                          | P1    | Serialization determinism tests; mutation-attempt compile/runtime checks               |
| REQ-123 | `generate` MUST execute the total Section 29 lifecycle in order, with every stage's failure behavior and cleanup defined; no stage may be skipped, reordered, or repeated except as written.                                                                                                | P2    | Lifecycle failure-injection tests at every stage                                       |
| REQ-124 | Destination preflight MUST acquire the parent via the Section 31.2 no-follow component walk with the Section 31.3 custody check, retain the handle for the transaction's life, perform the pre-commit diagnostic parent reobservation, and never follow a path-only check with a path-based mutation.                              | P2    | Parent-swap race tests (barrier-controlled) on macOS/Linux; custody matrix; static transaction audit |
| REQ-125 | Staging MUST be created exclusively relative to the retained parent handle (0700, `.foundry-<name>-<random>`, bounded `EEXIST` retries), its identity recorded, opened as a rooted writer with identity verification, written only through that root, and given exact modes via `fchmod` on open descriptors.                      | P2    | Stage-creation race tests; rooted-writer escape tests; umask-variation mode tests      |
| REQ-126 | Rendering MUST produce complete files with LF endings, `go/format`-clean Go source, and deterministic bytes; after `go mod tidy` the tree MUST be frozen as the approved conformance baseline.                                                                                            | P2    | Renderer goldens; freeze-and-conform tests                                             |
| REQ-127 | All verification MUST complete successfully in staging before commit; no destination placement may occur for an unverified tree; there is no bypass (see `REQ-152`).                                                                                                                       | P2    | Verification-failure tests proving no placement                                        |
| REQ-128 | Optional Git initialization MUST run in staging as exactly the isolated invocation of Section 34.2 (empty owned template, no system/global config), followed by scratch removal, repeated non-`.git` conformance, and semantic `.git` validation.                                          | P2    | Isolated-git tests with hostile host config/templates; `.git` semantic checks           |
| REQ-129 | Commit MUST be a single exclusive no-replace rename relative to the retained parent handle (Linux `RENAME_NOREPLACE`; Darwin `RENAME_EXCL \| RENAME_NOFOLLOW_ANY`), failing closed with `fs.rename_unsupported` where unsupported and on unexpected `EXDEV`; the destination MUST appear atomically and never partially.            | P2    | Commit primitive tests per filesystem matrix (E1); two-process `EEXIST` races           |
| REQ-130 | **No production code path may automatically delete a created stage.** Every uncommitted outcome — failure, cancellation, conflict, ambiguity — MUST preserve the stage and report its exact identity-confirmed location with a manual remediation, per Sections 31.6 and 31.9; every commit outcome MUST resolve exactly per the Section 31.9 matrix. | P2    | Static no-stage-delete audit; preservation-reporting tests; table-driven commit-result state-machine tests with cancellation injection |
| REQ-131 | Commit results MUST be classified by post-syscall identity inspection per Section 31.8 (`fs.commit_ambiguous` on contradictory identities, with all further mutation stopped); concurrent generations to one destination MUST yield exactly one winner with the loser's stage preserved; crash leftovers (`.foundry-<name>-*`) are reported, never auto-collected, and the committed destination is never deleted. | P2    | Identity-classification tests incl. injected ambiguous acknowledgments; concurrent-process tests; leftover-report tests |
| REQ-132 | Determinism inputs MUST be exactly: the Foundry binary (embedded catalog), the validated specification, and the declared tool versions; nothing else may influence output bytes (Section 32.1 envelope).                                                                                  | P2    | Double-generation equality; envelope-variation tests                                    |
| REQ-133 | No output may contain timestamps, usernames, hostnames, locale-dependent text, absolute host paths, or any host-derived data; the destination path appears only as plan identity, never in generated content.                                                                             | P2    | Content scans over golden trees                                                         |
| REQ-134 | All generated text files MUST use LF endings, files 0644/dirs 0755/staging 0700; name equivalence is decided by the host filesystem via native exact-child lookup plus the exclusive commit, with no synthetic case-fold or normalization scan.                                            | P2    | Cross-platform tree manifest comparison; case-sensitivity matrix                        |
| REQ-135 | Every dependency, tool, action, and toolchain version MUST be exact per Section 12 and `catalog/versions.toml`; tool behavior MUST be pinned by the Section 34.2 constructed environments (`GOTOOLCHAIN=local`, `GOENV=off`, empty `GOFLAGS`, empty `GOPRIVATE`/`GONOPROXY`/`GONOSUMDB`/`GOINSECURE`), never by inherited host configuration; the exact Go version is preflighted before staging. | P2    | BOM/plan inspection; hostile-environment sentinel tests; wrong-version preflight tests  |

### 53.8 Verification, Output, Errors, Git, and Operations (REQ-150–REQ-165; Sections 34–41, 48–49)

| ID      | Normative requirement                                                                                                                                                                                                                            | Phase | Verification                                                              |
| ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- | ---------------------------------------------------------------------------- |
| REQ-150 | Default verification MUST be gofmt conformance, tidy mutation-set validation (`go.mod`/`go.sum` only; exact pins reparse; no replace/exclude directives), `go mod verify`, `go test -count=1 -buildvcs=false ./...`, and `go vet -buildvcs=false ./...` in staging, all passing before commit, with bounded failure output naming the step. | P2    | Generated-project end-to-end tests; cached-test bypass regression test; module-mutation tests |
| REQ-151 | Strict mode (`--verify strict`) MUST add `go tool staticcheck ./...` and `go tool govulncheck ./...`; race is never part of generation verification and runs only in CI/explicit contexts per Section 44.3.                                            | P2    | Strict-mode matrix tests; absence-of-race-in-generation audit                  |
| REQ-152 | No flag, environment variable, or configuration may bypass, weaken, or skip verification; `--verify` accepts exactly `default` and `strict`.                                                                                                          | P2    | Flag-rejection tests; help goldens                                             |
| REQ-153 | The Foundry MUST execute exactly the plan-declared steps using only the `go` and `git` binaries located once at startup, with no shell anywhere; a missing tool or wrong exact Go version fails before staging with `tool.missing`/`tool.wrong_version` and install guidance.        | P2    | Runner allowlist tests; missing/wrong-version tool tests                       |
| REQ-154 | Every subprocess MUST run with the exact constructed environment of Section 34.2 (empty base plus allowlist), its working directory bound to the retained stage descriptor via the serialized `fchdir` protocol of Section 34.4 (never a pathname `Dir`), plan-declared timeout, per-stream 4 MiB output caps with explicit truncation, process-group kill on cancellation, and captured output replayed only on failure. | P2    | Environment-construction unit tests; sentinel leak tests; child-cwd swap and restore-failure tests; timeout/cap tests |
| REQ-155 | Human output MUST follow Section 36: plain line-oriented stdout, diagnostics on stderr, stable per-stage progress names, `NO_COLOR` honored, no prompts, every failure naming what/where/remediation and any preserved stage; post-commit report failure never changes a success exit. | P1    | Output goldens; real broken-pipe stream-failure tests                          |
| REQ-156 | JSON output MUST follow Section 37: one versioned deterministic document on stdout with empty stderr, commit-outcome enum and preserved-stage location included in `generate` results, no error state coexisting with a committed repository, additive-only evolution within a schema major. | P1    | JSON schema tests; committed-success stream-failure tests; empty-stderr tests  |
| REQ-157 | Every failure MUST carry a stable `domain.reason` identifier from the Appendix D inventory, owned by the `internal/diagnostic` registry with tested uniqueness; identifiers are append-only across releases.                                                                          | P1    | Error-taxonomy completeness and uniqueness tests                               |
| REQ-158 | Exit statuses MUST be exactly 0 (success, including post-commit report failure), 1 (runtime/tool/verification/commit failure/ambiguity), 2 (input/selection/destination/custody/usage), 130 (cancelled before commit), with the commit result always dominant; the Foundry MUST catch SIGPIPE per Section 36.5 so this contract holds under real broken pipes, while children retain default SIGPIPE. | P2    | Exit-code matrix tests; commit-dominance tests; real SIGPIPE process tests      |
| REQ-159 | Errors MUST be actionable (location + smallest correct remediation), `--verbose` adds diagnostic detail to stderr only, and all diagnostics MUST redact known-sensitive environment values.                                                            | P1    | Diagnostic and redaction tests                                                 |
| REQ-160 | The Foundry's complete Git behavior MUST be the single isolated `git init` of Section 39; no commits, identity, hooks, remotes, or other subcommands ever.                                                                                             | P2    | Git behavior tests; process-tree audit                                         |
| REQ-161 | No persistent Foundry provenance file may be generated; the plan is the provenance record; the single `AGENTS.md` comment line is the only generated Foundry reference and is never parsed.                                                            | P2    | Generated tree audit                                                           |
| REQ-162 | Foundry versioning MUST be semantic across CLI surface, plan schema, and specification schema together, with embedded build metadata from build settings, not linker string injection.                                                                | P1    | Version-output tests; release inspection                                       |
| REQ-163 | Dependabot MUST keep the Foundry repository updated weekly; catalog pin bumps MUST regenerate all fixtures in CI; a published vulnerability in a pinned generated dependency MUST trigger a catalog patch release.                                     | P4    | Dependabot config inspection; update-PR drill; vuln-response drill              |
| REQ-164 | Foundry installation MUST be `go install`/local build until Phase 4, then the distribution profile's own architecture applied to the Foundry (Section 48); no self-update or package-manager submission.                                              | P4    | Install smoke tests; release snapshot                                          |
| REQ-165 | Performance MUST meet Section 49: write-free commands immediate and tool-free; per-stage generation timing reported; baselines measured in Phase 2 dogfood before any absolute gate is set.                                                            | P2    | Timing-report tests; recorded baselines                                        |

### 53.9 Foundry Repository Architecture (REQ-180–REQ-188; Sections 42–43)

| ID      | Normative requirement                                                                                                                                                                                                | Phase | Verification                                             |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----- | ------------------------------------------------------------ |
| REQ-180 | The Foundry repository MUST match the Section 42.1 canonical tree; `internal/compose`, `internal/structured`, and provenance-chain types MUST NOT exist.                                                                    | P1    | Tree inspection; import tests                                  |
| REQ-181 | `cmd/foundry/main.go` MUST be the only `os.Exit` site and the single error-to-exit translation point; `internal/cli` owns flags and output-mode selection only.                                                             | P1    | Static checks; process tests                                   |
| REQ-182 | `internal/spec` and `internal/catalog` MUST own parsing/validation exclusively and produce immutable validated values with source positions.                                                                                | P1    | Ownership/import tests                                         |
| REQ-183 | `internal/resolve` and `internal/plan` MUST own resolution and planning exclusively, implementing only the flat Section 23/27/28 semantics.                                                                                 | P1    | Import tests; flat-semantics audit                             |
| REQ-184 | `internal/fsx` MUST be the only package mutating the destination/staging filesystem, exposing only descriptor-relative operations and **no stage-deletion method or code path**, with platform commit adapters build-tagged; `internal/render` MUST write only through the rooted writer interface and return an immutable digest inventory.  | P2    | Static architecture tests incl. no-delete audit; import-graph enforcement |
| REQ-185 | `internal/toolrun` MUST be the only subprocess-starting package and the sole owner of the original-cwd descriptor and child-start mutex; `internal/generate` MUST own the total post-plan state machine, emitting typed events, using `fsx`/`toolrun`/`gitinit`/`verify` without duplicating their authority and without output encoding.     | P2    | Fake-runner tests; static subprocess audit; event/encoding separation tests |
| REQ-186 | `internal/report` MUST own all human/JSON formatting, consuming typed events/results only; `internal/diagnostic` MUST own the error-identifier registry; `internal/version` and embedded assets MUST be the only build-metadata sources.                                                                                                     | P1    | Output-ownership tests; registry tests                         |
| REQ-187 | Package dependencies MUST flow strictly downward per Section 42.3, with interfaces only for true external seams (process runner, output writers, unexported platform fault-injection); no general FS/sink/template interface or DI container may exist; no import cycles, no upward imports, no Cobra below `internal/cli`.                   | P1    | Import-cycle/static architecture tests; interface inventory review |
| REQ-188 | Contexts MUST flow explicitly as first parameters; boundary-crossing values MUST be immutable; no package-level mutable state; panics MUST NOT cross package boundaries except into the single top-level `internal.bug` wrap. | P1    | Static checks; invariant unit tests                            |

### 53.10 Testing and Security (REQ-210–REQ-224; Sections 45–47)

| ID      | Normative requirement                                                                                                                                                                                                          | Phase | Verification                                                       |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----- | ---------------------------------------------------------------------- |
| REQ-210 | Specification parsing/validation MUST have table-driven unit tests covering every error identifier and every field rule, plus parser fuzzing.                                                                                           | P1    | Coverage of the Appendix D spec-domain identifiers                       |
| REQ-211 | Resolver and planner MUST have unit, property (determinism/ordering), and golden-plan tests for every fixture.                                                                                                                          | P1    | Golden suite in CI                                                       |
| REQ-212 | Rendering MUST have unit and golden tests per mechanism (`static`, `template`, `gomod`), including native-tool parsing of generated workflow/GoReleaser files and `go/format` idempotence.                                              | P2    | Renderer test suite                                                      |
| REQ-213 | Filesystem behavior MUST have the hostile suite of Section 45.1: parent/stage swap races, custody matrices, rooted-writer escapes, commit races with loser preservation, ambiguity classification, `EXDEV`, case matrix, no-replace probes, cancellation/crash leftovers, sentinel survival, and the no-stage-delete static audit, run on macOS and Linux in CI.  | P2    | Hostile suite green on both platforms; audit in CI                       |
| REQ-214 | External tool execution MUST have fake-runner unit tests plus real sentinel tests proving environment/config/template isolation, descriptor-bound child cwd (incl. swaps during start and restore failure), timeout, cancellation, output caps, failure replay, and real broken-pipe behavior before and after commit with default child SIGPIPE proven.          | P2    | Sentinel suite (E2 promoted into CI); SIGPIPE process tests              |
| REQ-215 | Every golden generated project MUST compile, test (`-count=1`), vet, and lint cleanly in CI, including TUI lifecycle/PTY/debug-log matrices and the deliberate-mutation fixture failing with `verify.unplanned_mutation`.               | P3    | Generated-project matrix in CI                                           |
| REQ-216 | Profile testing MUST cover exactly the real selection space: no profiles (MVP) and `distribution` on both archetypes with both visibilities (post-MVP); no synthetic combinatorial matrix is required.                                  | P4    | Bounded combination tests                                                |
| REQ-217 | Fuzzing MUST cover the spec parser and path normalization; race jobs (preflighted, `CGO_ENABLED=1`) MUST run in Foundry CI with a known-race fixture proving instrumentation; benchmarks are added only when a measured regression exists. | P2    | Fuzz corpus in strict CI; race-fixture evidence                          |
| REQ-218 | End-to-end `testscript` suites MUST drive the real binary across validate/plan/generate/cancel/failure paths on Linux and macOS CI.                                                                                                     | P3    | Cross-platform e2e workflows                                             |
| REQ-219 | Golden updates MUST require the test-only `UPDATE_GOLDEN=1`, update one named suite at a time, print changed paths, and refuse bulk updates above a documented threshold without a second opt-in; CI MUST never auto-update goldens (Section 45.4).                                | P1    | Guarded-update command tests; CI policy check                            |
| REQ-220 | Security controls of Section 46.2 MUST each have at least one attacking test: transaction races, template escapes, catalog smuggling, hostile tool configuration; findings feed the risk register.                                      | P2    | Threat-matrix test inventory; security review                            |
| REQ-221 | No secret may appear in specifications, plans, generated content, logs, or diagnostics; redaction of known-sensitive environment/proxy/credential values MUST be sentinel-tested; module downloads MUST honor the captured host `GOPROXY`/`GOSUMDB` with private-module routing closed (`GOPRIVATE`/`GONOPROXY`/`GONOSUMDB`/`GOINSECURE` empty). | P3    | Redaction sentinel tests; dependency scan; plan content scan             |
| REQ-222 | Foundry PR/default CI MUST run the Section 47.1 `ci.yml` matrix and block merge on any failure, including the catalog-digest and golden-matrix checks.                                                                                  | P3    | Actual GitHub Actions runs                                               |
| REQ-223 | Foundry scheduled CI MUST run `strict.yml` (govulncheck, fuzz, cold-cache baseline, full hostile suite); generated projects MUST carry exactly the reduced Section 16.6 workflows.                                                      | P3    | Scheduled workflow runs; generated workflow goldens                      |
| REQ-224 | All GitHub Actions everywhere MUST be pinned by full commit SHA with version comments; release workflows MUST be tag-triggered with least permissions and never run on untrusted pull requests.                                          | P4    | Workflow inspection tests; release snapshot                              |

### 53.11 Phases, Dogfood, and Authority (REQ-240–REQ-248; Sections 50–52)

| ID      | Normative requirement                                                                                                                                                                                                    | Phase | Verification                                                    |
| ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- | ------------------------------------------------------------------- |
| REQ-240 | The five phase-entry evidence gates (E1–E5, Section 51.2) MUST be executed and recorded at their exact phase boundaries — E5 before Phase 1 exit, E1–E3 before any Phase 2 transaction/tool code, E4 before Phase 3 TUI work; no gate may be waived, reordered past its phase, or satisfied by prose. | S6    | Recorded evidence per Appendix I; phase-boundary review               |
| REQ-241 | Phase 1 MUST deliver the complete write-free surface (`validate`, `plan`, `catalog`, `version`) with golden plans, full error-identifier coverage, rejection of every unimplemented profile ID, and proven absence of writes/subprocesses/network.                                                   | P1    | Phase 1 exit checklist (Section 51.1)                                 |
| REQ-242 | Phase 2 MUST deliver transactional CLI generation passing default and strict verification plus the hostile/race/preservation suites on macOS/Linux, and MUST create and exercise `foundry-smoke-cli` and real `repo-map`, beginning dogfood the moment one CLI fixture passes real-platform transaction plus default verification. | P2    | Phase 2 exit checklist; dogfood records                               |
| REQ-243 | Phase 3 MUST deliver the Section 50 MVP: both archetypes with `profiles = []`, reduced generated CI, TUI matrices green, `foundry-smoke-tui` and `worktree-status` begun, and three-agent acceptance recorded.                | P3    | MVP acceptance matrix; recorded agent evidence                        |
| REQ-244 | Phase 4 MUST deliver the `distribution` profile end-to-end (scratch-repository release dogfood incl. the license-before-publication step and private/non-GitHub negative cases), Foundry self-distribution, and `OQ-300` resolution before publication.                                             | P4    | Release snapshot; distribution dogfood record                         |
| REQ-245 | Dogfooding MUST proceed in order through `foundry-smoke-cli`, `foundry-smoke-tui`, then real `repo-map` and `worktree-status`; dogfood repositories stay independent, the Foundry never modifies an existing project, and profile comparisons generate fresh siblings.                               | P2    | Repository inspection; recorded dogfood reports                       |
| REQ-246 | Grok Build, Codex, and Cursor MUST each complete the six recorded Section 52.3 scenarios (orient, CLI change, TUI change, repair, boundary decision, report) on fresh generated projects using only repository contents; generated-cause failures are revision triggers.                            | P3    | Recorded agent task reports and diffs                                 |
| REQ-247 | Dogfood MUST record the Section 52.2 measurements — orientation/edit metrics, retained/deleted files, first-feature diff, latency, PR CI latency, escaped defects, recurring config/persistence patterns, preserved-stage burden — and the mandatory simplification triggers MUST force review before Phase 4.  | P2    | Dogfood measurement records; trigger-review records                   |
| REQ-248 | This artifact is the implementation authority; Phase 1 is authorized immediately, gated phases MUST NOT proceed without their Section 51.2 evidence records, a contradicting record forces a reviewed revision of this artifact before the gated phase proceeds, and no phase may amend this specification.     | S6    | Program artifact workflow; phase-table inspection                     |

## 54. Requirement Traceability

Every active requirement traces to its sources (locked decisions, inherited
recommendations, Stage 5 findings via the Section 4 ledger) and risks. To
avoid duplicating the Stage 4 recommendation ledger, `REC-###` linkage is
inherited unchanged from the Stage 4 traceability matrix except where a
finding revised the requirement; the finding column below is the Stage 6
delta.

| Band                | Requirements        | Primary sources                                        | Stage 5 findings integrated                    | Risk linkage                          |
| ------------------- | ------------------- | ------------------------------------------------------- | ------------------------------------------------ | --------------------------------------- |
| Scope/invariants    | REQ-001–REQ-012     | DEC-001–DEC-016; REC-200 series; Charter                 | FND-004 (REQ-004), FND-008 (REQ-011)             | RSK-300, RSK-302, RSK-311                |
| Commands            | REQ-030–REQ-037     | DEC-006, DEC-012; REC-203, REC-213                       | FND-007 (REQ-030, REQ-034), FND-010/012 (REQ-036) | RSK-301, RSK-310                         |
| Specification       | REQ-038–REQ-046     | REC-201–REC-206                                          | FND-013 (REQ-038–040), FND-001/018 (REQ-044), FND-008 (REQ-045) | RSK-300, RSK-312                         |
| Core/archetypes     | REQ-060–REQ-072     | DEC-004, DEC-005, DEC-010; REC-010–REC-030; Report 2      | FND-014/016 (REQ-060–066), FND-010/014/017 (REQ-068–072) | RSK-305, RSK-306, RSK-403                |
| Profiles            | REQ-073–REQ-078     | DEC-011; REC-020, REC-034, REC-205                        | FND-008 (REQ-073–075 recast as exclusion/recipe, REQ-078), FND-019 (REQ-076), FND-009 (REQ-077) | RSK-401, RSK-402                         |
| Catalog/rendering   | REQ-090–REQ-101     | REC-207–REC-209, REC-214                                  | FND-009 (REQ-092, 098–100), FND-015 (REQ-095–097)  | RSK-303, RSK-304                         |
| Plan/transaction    | REQ-120–REQ-135     | REC-210–REC-212                                           | FND-001/002/003/018 (REQ-124–131, 134), FND-006 (REQ-121, 126, 127), FND-012 (REQ-123, 129), FND-004/005 (REQ-135) | RSK-300, RSK-310, RSK-311, RSK-400        |
| Verification/output | REQ-150–REQ-165     | DEC-012; REC-016, REC-028–REC-031, REC-111, REC-213       | FND-006 (REQ-150), FND-004/016 (REQ-151), FND-005 (REQ-153, 154, 160), FND-012 (REQ-155, 156, 158), FND-017 (REQ-159), FND-019 (REQ-164) | RSK-301, RSK-308, RSK-309, RSK-312        |
| Repo architecture   | REQ-180–REQ-188     | REC-104, REC-214; Charter                                 | FND-009/015 (REQ-183, 184, 187), FND-001/002 (REQ-184), FND-005 (REQ-185), FND-010 (REQ-188) | RSK-303                                  |
| Testing/security    | REQ-210–REQ-224     | REC-015–REC-033, REC-215                                  | FND-001/002/018 (REQ-213, 220), FND-005/007 (REQ-214), FND-006/010/014/017 (REQ-215), FND-008/009 (REQ-216), FND-004/016 (REQ-217, 222, 223), FND-019 (REQ-224), FND-017 (REQ-221) | RSK-300–RSK-312                          |
| Phases/dogfood      | REQ-240–REQ-248     | REC-102, REC-103, REC-205, REC-217; DEC-016               | FND-011 (REQ-240, 248), FND-014 (REQ-242, 243, 247), FND-008/016 (REQ-243, 247), FND-019 (REQ-244) | RSK-302, RSK-305, RSK-307, RSK-400–RSK-403 |

Full per-requirement phase and verification appear inline in Section 53.
Appendix F records the Stage 4 → Stage 6 requirement delta, including the
recast of `REQ-073`–`REQ-075` into exclusion-and-recipe requirements.

## 55. Risk Register

All Stage 4 risk identifiers survive with revised mitigations; the four
Stage 5 review risks are adopted. Likelihood/impact: L/M/H.

| ID      | Risk                                                                                              | L/I  | Revised mitigation                                                                                                                                  | Contingency                                                                     |
| ------- | --------------------------------------------------------------------------------------------------- | ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------- |
| RSK-300 | Filesystem race and platform semantics differ across macOS/Linux filesystems                          | M/H  | Descriptor-relative transaction (Section 31); exclusive no-replace commit; fail-closed `fs.rename_unsupported`; hostile suite on both platforms          | Narrow the supported path contract further; never fall back to destructive rename   |
| RSK-301 | Cold-cache or network-sensitive verification fails or is slow on unprovisioned machines               | H/M  | Truthful network disclosure (no offline claim); exact tool environments; per-step timing; cold baselines in Phase 2                                      | Fail truthfully before placement; document cache warm-up                            |
| RSK-302 | Profile/scaffold growth becomes a framework again                                                     | M/M  | Recipe-first rule with two-real-projects evidence bar (Section 21); flat schema with prohibited composition fields (`REQ-077`, `REQ-092`)                | Reject candidates; require explicit specification revision                          |
| RSK-303 | Generators/templates drift into a hidden mini-framework                                               | M/M  | Three rendering mechanisms only; one shared file (`go.mod`); native-tool parse tests; `REQ-096`                                                          | Delete mechanisms; convert to complete owned files                                  |
| RSK-304 | Embedded and visible catalog drift apart                                                              | L/M  | Deterministic digest in `version`/plan; CI tree-vs-embed comparison (`REQ-090`)                                                                          | Block release on digest mismatch                                                    |
| RSK-305 | TUI lifecycle or architecture proves too heavy or incomplete in real terminals                        | M/M  | Single-owner lifecycle (Section 18.3) validated by the E4 gate and PTY matrices; real `worktree-status` dogfood in Phase 3                                | Revise Section 18 through ordinary artifact revision                                |
| RSK-306 | Documentation and CI command drift in generated projects                                              | M/M  | One canonical command list asserted identical across `AGENTS.md`/docs/CI by tests (`REQ-062`)                                                            | Consistency test failure blocks catalog release                                     |
| RSK-307 | Agent portability or instruction quality changes across Grok Build/Codex/Cursor                       | M/M  | Portable single-authority `AGENTS.md`; recorded three-agent acceptance with revision triggers (`REQ-246`)                                               | Revise instruction contract; never add vendor-specific files                        |
| RSK-308 | Distribution depends on GitHub settings and changing release APIs                                     | M/M  | Post-MVP timing; scratch-repository release dogfood; documented owner settings incl. license and immutable releases (`REQ-076`, `REQ-244`)               | Pin/adjust workflows in a catalog patch release                                     |
| RSK-309 | Dependency or action vulnerability and maintenance drift                                              | M/M  | Exact pins; weekly Dependabot; govulncheck cadence; full-SHA action pins; vuln-response drill (`REQ-163`)                                                | Catalog patch release                                                               |
| RSK-310 | Preserved-stage debris accumulates and burdens owners (no automatic deletion exists)                  | M/L  | Every preserved stage is hidden `0700` with its exact location reported in text and JSON; dogfood measures manual cleanup burden (`REQ-247`)             | **Revisit trigger:** if dogfood shows recurring multi-stage debris in real use, commission a reviewed, separately specified cleanup design; never add deletion ad hoc |
| RSK-311 | Version evidence staleness between research and implementation                                        | M/M  | E5 version/action lock at Phase 1 exit (`REQ-240`); `catalog/versions.toml` single lock manifest; Stage 4 TV-01–TV-18 base                               | Re-lock and revise pins before Phase 2                                              |
| RSK-312 | Configuration or diagnostics leak secrets                                                             | L/H  | Secret-free schema; redaction tests; exclusive-create 0600 debug logs; plan content scans (`REQ-041`, `REQ-159`, `REQ-221`)                              | Treat any leak as a release blocker                                                 |
| RSK-400 | Descriptor-relative transaction primitives behave differently across kernels/filesystems               | M/H  | E1 spike on APFS/ext4/xfs/btrfs plus negative probe as the Phase 2 entry gate (`OQ-400`); build-tagged platform adapters in `internal/fsx`; fail-closed `fs.rename_unsupported`                       | Constrain supported filesystems explicitly; fail closed; revise Section 31 by reviewed revision if contradicted |
| RSK-401 | Repeated project-local configuration/persistence work without generated profiles                      | H/L  | Deliberate: dogfood measures recurring patterns as future-profile evidence (`REQ-247`)                                                                   | Promote a recipe to a profile via explicit revision when the evidence bar is met     |
| RSK-402 | Complete workflow templates duplicate small CI structure between archetypes                            | M/L  | Accepted cost of deleting typed emitters; templates stay small, visible, independently testable                                                          | Extract a shared static fragment only if divergence causes a real defect             |
| RSK-403 | Lighter generated CI detects some defects after merge instead of before                                | M/L  | Documented risk-based promotion of scheduled checks; dogfood measures escaped defects (`REQ-063`, `REQ-247`)                                             | Promote specific checks to required PR gates per project                            |

## 56. Open Questions and Remaining Blockers

### 56.1 OQ-300 — Canonical Foundry public module path and release repository

Non-blocking for Phases 1–3. Required evidence: owner decision on the public
module path/repository before the first public Foundry release. Owning phase:
Phase 4 entry. Deadline: Phase 4 entry. Conservative default if unresolved:
remain private under the current module path and defer Phase 4 release
hardening; nothing in Phases 1–3 depends on the answer.

### 56.2 OQ-400 — Exact descriptor-relative transaction primitive on macOS and Linux

**Architecturally resolved; executable confirmation gates Phase 2 entry.**
The architecture is normatively fixed (Section 31) and supported by
documentary primary-source verification (SV-01–SV-03); the remaining question
is executable confirmation that the selected primitives (`os.Root` method
set; `Renameat2(RENAME_NOREPLACE)`; `RenameatxNp(RENAME_EXCL |
RENAME_NOFOLLOW_ANY)`; descriptor-bound child cwd) behave as specified on
APFS, ext4, xfs, and btrfs, plus the fail-closed behavior on a filesystem
without no-replace support. Required evidence: the E1 spike record
(Appendix I). Owner: Robert Guss. Deadline: before any Phase 2 transaction or
tool code is written; bounded at ≤ 3 days. Conservative default if the spike
contradicts a detail: revise Section 31 through ordinary reviewed artifact
revision before Phase 2 proceeds — never improvise a pathname fallback or
proceed with known-false transaction claims.

### 56.3 Remaining Gated Work

This artifact is `Accepted — implementation authority` and Phase 1 is
authorized now. No finding disposition, requirement, or phase is blocked by
anything other than the phase-entry evidence gates:

1. **E5** (version/action lock, ≤ 1 day) — before Phase 1 exit.
2. **E1, E2, E3** (transaction spike, environment isolation spike, race
   prerequisite probe; ≤ 6 days combined) — before any Phase 2 transaction
   or tool code.
3. **E4** (Bubble Tea lifecycle spike, ≤ 2 days) — before Phase 3 TUI work.

Each record is filed per Appendix I through the program's ordinary artifact
workflow. A contradicting record forces a minimal reviewed revision of this
artifact before the gated phase proceeds.

## 57. Deferred Work

Deferred with explicit triggers (unchanged from Stage 4 except as noted):

| Item                                   | Trigger to revisit                                                                              |
| -------------------------------------- | ------------------------------------------------------------------------------------------------- |
| `configuration` profile (now recipe)   | Two real projects retain one complete configuration architecture (revised by FND-008)              |
| `local-persistence` profile (now recipe) | Two real projects retain one complete persistence architecture (revised by FND-008)               |
| HTTP client capability                 | Two dogfood projects independently repeat the same timeout/transport/testing architecture           |
| Rate limiting/retry capability         | Demonstrated recurring idempotent-retry need                                                        |
| Property testing (`rapid`)             | Fuzzing/table tests prove insufficient for a real generated-project defect class                    |
| Homebrew or other install channels     | Demonstrated public install demand after Phase 4                                                    |
| Helper-binary support                  | A real product needs a separately supervised process; explicit schema revision (revised by FND-009) |
| Interactive prompt layer               | Never at the expense of non-interactive determinism; recipe only                                    |
| Advanced observability                 | A generated project demonstrably needs more than clear errors and optional debug logs               |
| Signing/notarization beyond attestations | macOS Gatekeeper friction observed in real distribution dogfood                                    |
| Richer TUI report surfaces for the Foundry itself | Never for schema 1; the Foundry stays a CLI                                              |

## 58. Rejected Work

Rejected outright; reversal requires an explicitly revised accepted
specification:

- Verification bypass or `--verify none` (undermines the defining quality
  promise).
- `--offline` or any offline mode (unenforceable; FND-007).
- Existing-project modification, upgrade, synchronization, or profile
  addition (DEC-002/DEC-003).
- Plugins, remote catalogs, template downloads, profile scripts, arbitrary
  user templates (DEC-014).
- Generic profile-composition framework: transitive requires, conflicts,
  capability registry, providers, helper-binary schema, provenance closure
  (FND-009).
- Typed YAML/GoReleaser/`.gitignore`/Dependabot/workflow emitters and the
  limited-substitution token language (FND-015).
- Generated demo features (`greet`, synthetic TUI initialization) (FND-014).
- Persistent generated provenance files.
- Windows support (DEC-013); Claude-specific conventions (DEC-015).
- Viper, secret storage, SQLite-by-default, mutation testing, self-update,
  vendor agent files, generation telemetry.

## 59. Definition of Done for the Initial Foundry

The initial Foundry is done when:

1. Phases 1–4 exit criteria (Section 51.1) are all met with recorded
   evidence.
2. Every Section 53 requirement is implemented and verified per its listed
   verification, with the traceability matrix green.
3. All four dogfood repositories exist with recorded Section 52.2
   measurements and no unaddressed revision trigger.
4. Three-agent acceptance evidence is recorded.
5. The Foundry distributes itself through its own distribution architecture,
   and one scratch-repository release dogfood (including the license
   publication step) is recorded.
6. The risk register has no High-impact risk without an implemented
   mitigation and test.

## 60. Implementation Handoff

| Question                                | Answer                                                                                                                                                                             |
| --------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| May implementation begin?               | **Yes — Phase 1 now.** Status is `Accepted — implementation authority`; gated phases require their Section 51.2 evidence records (Section 56.3).                                      |
| First implementation task (now)         | `internal/diagnostic` (error-identifier registry) then `internal/spec`: strict TOML parsing/validation with the full error-identifier table and fuzzing (`REQ-038`–`REQ-046`, `REQ-157`, `REQ-210`). |
| Evidence work runnable in parallel      | E5 (version/action lock, before Phase 1 exit); E1–E3 (before any Phase 2 transaction/tool code); E4 (before Phase 3 TUI work). Recorded per Appendix I. These replace the deleted Phase 0. |
| Package order                           | `diagnostic` → `spec` → `catalog` → `resolve` → `render` (pure) → `plan` → `report`/`cli` (Phase 1) → `fsx` → `toolrun`/`gitinit` → `verify` → `generate` (Phase 2) → TUI archetype content (Phase 3) → `distribution` (Phase 4). |
| Earliest CLI dogfood                    | Inside Phase 2, the moment one CLI fixture passes real-platform transaction + default verification: `foundry-smoke-cli`, then real `repo-map`.                                        |
| Earliest TUI dogfood                    | Inside Phase 3, once the static TUI passes lifecycle/PTY/debug-log tests: `foundry-smoke-tui`, then real `worktree-status`.                                                           |
| Profiles by phase                       | Phases 1–3: none (`profiles = []`; every unimplemented ID rejected). Phase 4: `distribution`.                                                                                         |
| Reversible until Phase 3 exit           | Commit-primitive shape within Section 31 semantics; verification step ordering; generated CI job composition; TUI internal file layout (Section 51.3).                                |
| Phase-boundary evidence                 | Exactly the exit-criteria column of Section 51.1 plus the entry-gate records, executed via the Section 16.4/45 command sets, recorded in the repository.                              |
| Remaining gated work                    | Only the unexecuted E1–E5 evidence records at their phase boundaries (chiefly `OQ-400` via E1).                                                                                       |

## 61. References

Primary sources verified for this revision (2026-07-30) and inherited
verifications:

1. Go standard library, `os.Root` documentation — descriptor-relative method
   set and symlink-escape guarantees. https://pkg.go.dev/os#Root
2. Go security release notes — Go 1.26.5 fix for the `os.Root` trailing-slash
   symlink escape (CVE-2026-39822). https://go.dev/doc/devel/release
3. `golang.org/x/sys/unix` — `Renameat2` (Linux, `RENAME_NOREPLACE`) and
   `RenameatxNp` (Darwin, `RENAME_EXCL`, `RENAME_NOFOLLOW_ANY`).
   https://pkg.go.dev/golang.org/x/sys/unix
4. POSIX `openat`/`fchdir` specifications — directory-descriptor object
   continuity and descriptor-based working-directory change.
   https://pubs.opengroup.org/onlinepubs/9699919799/functions/openat.html and
   https://pubs.opengroup.org/onlinepubs/9699919799/functions/fchdir.html
5. Go `os/signal` documentation — SIGPIPE default behavior on fd 1/2,
   `signal.Notify(SIGPIPE)` converting kills into `EPIPE` write errors, and
   fork/exec resetting child signal dispositions.
   https://pkg.go.dev/os/signal
6. POSIX sticky-bit (`S_ISVTX`) directory semantics — restricted deletion in
   shared-writable directories.
   https://pubs.opengroup.org/onlinepubs/9699919799/basedefs/V1_chap04.html
7. `cmd/go` environment documentation — `GOENV`, `GOFLAGS`, `GOCACHEPROG`,
   `GOAUTH`, `GOVCS`, `GOTOOLCHAIN`, `GOPRIVATE`/`GONOPROXY`/`GONOSUMDB`,
   test caching, `-count=1`, and `-buildvcs`.
   https://pkg.go.dev/cmd/go
8. Go race detector documentation — cgo/C-compiler requirement and platform
   support. https://go.dev/doc/articles/race_detector
9. `git-init` and `git-config` documentation — template directories,
   `GIT_CONFIG_GLOBAL`/`GIT_CONFIG_SYSTEM`/`GIT_CONFIG_NOSYSTEM`,
   `--template`. https://git-scm.com/docs/git-init
10. Bubble Tea v2 documentation — `WithContext`, `WithoutSignalHandler`,
    panic recovery, command goroutine semantics.
    https://pkg.go.dev/charm.land/bubbletea/v2
11. `golang.org/x/mod` — `module.CheckPath`, `modfile`.
    https://pkg.go.dev/golang.org/x/mod
12. Stage 4 version verifications TV-01–TV-18 (2026-07-29), inherited as the
    pin evidence base pending the E5 lock record.

Program artifacts: `docs/00-program-blueprint.md`,
`docs/01-research-charter.md`, `docs/reports/01…03-*.md`,
`docs/specifications/01-definitive-foundry-specification.md` (superseded),
`docs/reviews/01-definitive-foundry-specification-adversarial-review.md`,
`docs/prompts/05-adversarial-review-prompt.md`.

## Appendix A — Canonical Generated Trees

The normative CLI and TUI trees are Sections 17.2 and 18.2. The
`distribution` profile (post-MVP) adds exactly this delta to either
archetype, with no other file touched:

```text
.github/workflows/release.yml             # tag-triggered release
.github/workflows/dependency-review.yml   # PR dependency review
.goreleaser.yaml
docs/releasing.md                         # incl. license-before-publication block
```

No generated tree ever contains `pkg/`, `util/`, `common/`, `scripts/`, a
Makefile/Taskfile, `.golangci.yml`, a license file, a provenance file,
`internal/greet`, or any demo feature.

## Appendix B — Canonical Project Specification Examples

Private CLI (MVP shape):

```toml
schema = 1
name = "repo-map"
module = "github.com/robertguss/repo-map"
description = "Repository inventory CLI with text and JSON output"
archetype = "cli"
destination = "/Users/robertguss/Projects/tools/repo-map"
```

Private TUI:

```toml
schema = 1
name = "worktree-status"
module = "github.com/robertguss/worktree-status"
description = "Git worktree status TUI with asynchronous refresh"
archetype = "tui"
destination = "/Users/robertguss/Projects/tools/worktree-status"

[git]
initial_branch = "main"
```

Public CLI with distribution (valid only after the profile ships, post-MVP;
no license file exists at generation time — publication is blocked until the
owner adds one):

```toml
schema = 1
name = "repo-map"
module = "github.com/robertguss/repo-map"
description = "Repository inventory CLI with text and JSON output"
archetype = "cli"
destination = "./repo-map"
visibility = "public"
profiles = ["distribution"]
```

No example uses `--offline`, a removed profile, or depends on field order;
`schema` first is style, not validity.

## Appendix C — Illustrative Generation Plan

Abbreviated for length; the normative field contract is Section 28.2.

```json
{
  "schema": 1,
  "foundry": {"version": "0.1.0", "go": "go1.26.5", "catalog_digest": "sha256:…"},
  "project": {"name": "repo-map", "binary": "repo-map", "module": "github.com/robertguss/repo-map",
              "description": "Repository inventory CLI…", "archetype": "cli", "visibility": "private"},
  "destination": "/Users/robertguss/Projects/tools/repo-map",
  "profiles": [],
  "files": [
    {"path": ".github/workflows/ci.yml", "owner": "core", "mode": "0644", "render": "template", "source": "catalog/core/files/ci.yml.tmpl", "content_sha256": "…"},
    {"path": "cmd/repo-map/main.go", "owner": "archetype:cli", "mode": "0644", "render": "template", "source": "catalog/archetypes/cli/files/main.go.tmpl", "content_sha256": "…"},
    {"path": "go.mod", "owner": "shared:gomod", "mode": "0644", "render": "gomod", "source": "typed", "content_sha256": "…"}
  ],
  "dependencies": [{"module": "github.com/spf13/cobra", "version": "v1.10.2", "scope": "runtime", "owner": "archetype:cli"}],
  "tools": [{"module": "honnef.co/go/tools", "version": "…", "owner": "core"}],
  "external_steps": [
    {"id": "go-mod-tidy", "binary": "/usr/local/go/bin/go", "argv": ["go", "mod", "tidy", "-mod=mod"], "cwd": "stage-descriptor",
     "mutates": ["go.mod", "go.sum"], "network": "may", "timeout_s": 600, "output_cap_bytes": 4194304,
     "env": {"GOENV": "off", "GOFLAGS": "", "GOTOOLCHAIN": "local", "GOPRIVATE": "", "…": "…"}},
    {"id": "go-test", "binary": "/usr/local/go/bin/go", "argv": ["go", "test", "-count=1", "-buildvcs=false", "-mod=readonly", "./..."], "cwd": "stage-descriptor",
     "mutates": [], "network": "no", "timeout_s": 300, "output_cap_bytes": 4194304, "env": {"…": "…"}}
  ],
  "verification": {"mode": "default", "checks": ["gofmt", "module-mutation", "go-mod-verify", "go-test", "go-vet", "final-conformance"]},
  "network": {"may_be_required": true, "reasons": ["module resolution during go mod tidy on cold caches"]},
  "git": {"init": true, "initial_branch": "main"},
  "commit_result_model": "section-31.9/v1",
  "plan_sha256": "…",
  "warnings": []
}
```

## Appendix D — Error Identifier and Exit-Code Matrix

| Identifier                  | Meaning                                                              | Exit |
| --------------------------- | ---------------------------------------------------------------------- | ---- |
| `spec.parse_error`          | TOML syntax error (line/column)                                        | 2    |
| `spec.unsupported_schema`   | `schema` not in supported set (`1`)                                    | 2    |
| `spec.unknown_field`        | Unknown field/table (strict decoding)                                  | 2    |
| `spec.duplicate_key`        | Duplicate TOML key or table (strict decoding)                          | 2    |
| `spec.too_large`            | Project Specification exceeds the 1 MiB size cap                       | 2    |
| `spec.invalid_encoding`     | Project Specification is not UTF-8 (or has a leading BOM)              | 2    |
| `spec.invalid_field`        | Field violates its Section 14.3 rule                                   | 2    |
| `spec.duplicate_profile`    | Same profile ID listed twice                                           | 2    |
| `resolve.unknown_profile`   | Profile ID not an implemented built-in; sorted available set named     | 2    |
| `resolve.unknown_archetype` | Archetype not `cli`/`tui`                                              | 2    |
| `resolve.profile_constraint`| Direct archetype/visibility predicate failed                           | 2    |
| `plan.file_collision`       | Duplicate output ownership (all claimants named)                       | 1    |
| `fs.unsafe_path`            | Symlink among destination parent components / unsafe destination       | 2    |
| `fs.parent_missing`         | Destination parent absent or not a directory                           | 2    |
| `fs.namespace_not_private`  | Shared-writable non-sticky destination-parent component (custody)      | 2    |
| `fs.destination_exists`     | Destination exists in any form (incl. commit-time `EEXIST`); losing stage preserved | 2 |
| `fs.rename_unsupported`     | Filesystem lacks exclusive no-replace rename, or unexpected `EXDEV`; stage preserved | 1 |
| `fs.parent_moved`           | Pre-commit diagnostic reobservation shows the retained parent no longer at its pathname; stage preserved | 1 |
| `fs.commit_failed`          | Commit rename failed (other); stage preserved                          | 1    |
| `fs.commit_ambiguous`       | Contradictory post-syscall identity observations; all mutation stopped; stage state reported | 1 |
| `render.failed`             | Template/format failure (file and cause named)                         | 1    |
| `tool.missing`              | Required `go`/`git` not found at startup                               | 1    |
| `tool.wrong_version`        | Preflight `go version` differs from the exact pinned toolchain         | 1    |
| `tool.timeout`              | External step exceeded its plan-declared timeout; stage preserved      | 1    |
| `tool.failed`               | External step non-zero exit (step id, argv, bounded output); stage preserved | 1 |
| `verify.failed`             | Verification step failed in staging; stage preserved                   | 1    |
| `verify.module_mutation`    | Tidy changed files beyond `go.mod`/`go.sum` or altered pinned requirements | 1 |
| `verify.unplanned_mutation` | Staged tree deviates from frozen baseline (paths named); stage preserved | 1  |
| `git.failed`                | Isolated `git init` or `.git` semantic validation failed; stage preserved | 1 |
| `report.failed`             | Required pre-commit output stream failed (`EPIPE`); stage preserved    | 1    |
| `catalog.invalid`           | Embedded/dev catalog failed validation                                 | 1    |
| `usage.invalid`             | Unknown command/flag/argument or conflicting flags                     | 2    |
| `internal.bug`              | Recovered orchestration panic; report requested                        | 1    |
| Cancellation (pre-commit)   | SIGINT/SIGTERM before commit; any created stage preserved and reported | 130  |

Identifiers are append-only across releases (`REQ-157`).

## Appendix E — External Step Inventory

Normative environment construction is Section 34.2; every transactional
step's working directory is bound to the retained stage descriptor per
Section 34.4 (preflight steps run from the Foundry's startup cwd). No step
uses a shell; per-stream output cap is 4 MiB with explicit truncation.

| Step id            | argv                                                              | When               | Network | Timeout |
| ------------------ | ----------------------------------------------------------------- | ------------------ | ------- | ------- |
| `go-preflight`     | `go version` (exact pinned-version comparison)                     | Startup            | no      | 60 s    |
| `git-preflight`    | `git --version`                                                    | Startup (if init)  | no      | 60 s    |
| `go-mod-tidy`      | `go mod tidy -mod=mod`                                             | Always             | may     | 600 s   |
| `go-mod-verify`    | `go mod verify`                                                    | Always             | no      | 120 s   |
| `go-test`          | `go test -count=1 -buildvcs=false -mod=readonly ./...`             | Always             | no      | 300 s   |
| `go-vet`           | `go vet -buildvcs=false -mod=readonly ./...`                       | Always             | no      | 300 s   |
| `go-staticcheck`   | `go tool staticcheck ./...`                                        | Strict             | no      | 300 s   |
| `go-govulncheck`   | `go tool govulncheck ./...`                                        | Strict             | may     | 600 s   |
| `git-init`         | `git init --initial-branch=<branch> --template=<scratch> .`        | When `[git] init`  | no      | 60 s    |

gofmt conformance, tidy mutation-set validation, and final conformance are
in-process checks, not subprocesses. Strict `govulncheck` may access the
vulnerability database; this is part of the pre-staging network disclosure.

## Appendix F — Stage 4 → Stage 6 Requirement Delta

- **Recast (same subjects, never reused):** `REQ-073`, `REQ-074`, `REQ-075`
  — the configuration and local-persistence profile contracts become active
  exclusion-and-recipe requirements (catalog absence, `resolve.unknown_profile`
  rejection, Section 21 recipe boundaries). No identifier is retired.
- **Materially revised:** REQ-004, REQ-011, REQ-030–REQ-037,
  REQ-038–REQ-040, REQ-042–REQ-046, REQ-060, REQ-062–REQ-078, REQ-090,
  REQ-092, REQ-095–REQ-100, REQ-121, REQ-123–REQ-131, REQ-134,
  REQ-135, REQ-150, REQ-151, REQ-153–REQ-160, REQ-164,
  REQ-183–REQ-188, REQ-212–REQ-217, REQ-219–REQ-224,
  REQ-240–REQ-248 (finding mapping in Sections 4–5, 54).
- **Substantively unchanged:** all remaining identifiers.
- **New identifiers allocated:** none — every correction fit within existing
  subjects, so no unused identifier from REQ-001–REQ-299 was consumed.
- **Phase reallocation:** every requirement previously assigned to the
  deleted "Phase 0" now carries S6 (phase-entry evidence gates) or its
  natural implementation phase, per Section 53.

## Appendix G — Catalog Manifest Contract (Illustrative)

```toml
# catalog/archetypes/cli/manifest.toml
schema = 1
id = "cli"
kind = "archetype"
description = "Conventional CLI project archetype"

[[files]]
path = "cmd/{{binary}}/main.go"     # rendered path parameterization is limited
render = "template"                  # "static" | "template"
source = "files/main.go.tmpl"
mode = "0644"

[[dependencies]]
module = "github.com/spf13/cobra"
version = "v1.10.2"
scope = "runtime"                    # "runtime" | "test" | "tool"
```

Profile manifests add `compatible_archetypes = [...]` and optionally
`requires_visibility = "public"`. No other relational field exists
(`REQ-092`). The full field contract lives in
`catalog/schemas/catalog-manifest.md` in the Foundry repository.

## Appendix H — Glossary

| Term                    | Meaning                                                                                      |
| ----------------------- | ------------------------------------------------------------------------------------------------ |
| Foundry                 | The generator CLI specified by this artifact                                                       |
| Generated Project       | An independent Go repository the Foundry creates                                                   |
| Project Archetype       | The single primary application architecture (`cli` or `tui`)                                       |
| Capability Profile      | Optional catalog unit composing one complete capability (`distribution` only in v1.0)              |
| Recipe                  | Documented post-generation guidance with no generated files (Section 21)                           |
| Core                    | Files/standards present in every Generated Project                                                 |
| Project Specification   | The strict TOML input (`foundry.toml`)                                                             |
| Generation Plan         | Immutable inspectable contract between interpretation and side effects                             |
| Stage / staging         | The transaction-owned hidden `0700` sibling directory (`.foundry-<name>-<random>`) where output is rendered and verified |
| Parent/stage handle     | Retained directory descriptors that own all transactional filesystem operations                    |
| Namespace custody       | The Section 31.3 check rejecting shared-writable non-sticky destination-parent components          |
| Preserved stage         | A created stage that did not commit; never automatically deleted, always reported with its exact location |
| Exclusive no-replace commit | `RENAME_NOREPLACE`/`RENAME_EXCL \| RENAME_NOFOLLOW_ANY` rename making the destination appear atomically |
| Identity classification | Post-syscall inspection of source/destination objects to classify a commit outcome (Section 31.8)  |
| Final conformance       | Byte/mode/type/path comparison of the stage against the frozen post-tidy tree                      |
| Strict mode             | Verification adding Staticcheck and govulncheck                                                    |
| E1…E5                   | The five phase-entry evidence gates (Section 51.2)                                                 |

## Appendix I — Phase-Entry Evidence Record Template

One record per evidence item, committed under `docs/evidence/` (new files are
justified as the designated evidence location):

```markdown
# E<n> — <title>

- Date, machine(s), OS/kernel/filesystem versions
- Gated phase (per Section 51.2) and gate condition
- Exact commands / spike code reference (committed alongside)
- Expected behavior (cite the specification section)
- Observed behavior (verbatim output excerpts)
- Result: CONFIRMS | CONTRADICTS <section>
- If CONTRADICTS: required specification revision, filed and reviewed before
  the gated phase proceeds
```

Each gated phase requires its records with result CONFIRMS (or integrated
reviewed revisions), reviewed by the owner, referenced from the phase-entry
decision. No gate may be waived or satisfied by prose.

---

End of specification. This artifact supersedes
`docs/specifications/01-definitive-foundry-specification.md` in full.
