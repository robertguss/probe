# E1 — Descriptor-relative transaction spike

- **Date:** 2026-07-30
- **Gated phase:** Phase 2 entry (Section 51.2, REQ-240)
- **Resolves:** OQ-400, RSK-400 (executable confirmation of Section 31 / 34.4)
- **Spike code:** [`integration/hostile/e1/`](../../integration/hostile/e1/)
- **Raw logs:** [`docs/evidence/e1-logs/`](e1-logs/)

## Machine / tool versions

| Field | Value |
| ----- | ----- |
| Hostname | `dev-box` |
| OS | Ubuntu 24.04.4 LTS (`PRETTY_NAME`) |
| Kernel | `Linux 6.8.0-134-generic #134-Ubuntu SMP PREEMPT_DYNAMIC x86_64` |
| Arch | `linux/amd64` |
| Go | `go1.26.5` (`GOTOOLCHAIN=go1.26.5`; pin matches Section 12 / SV-01 CVE-2026-39822 fix) |
| `golang.org/x/sys` | `v0.34.0` |
| Loop tools | `mkfs.xfs 6.6.0`, `mkfs.btrfs 6.6.3`, `mkfs.vfat 4.2`, `losetup` |

### Filesystems exercised (live)

| FS | How mounted | Path (ephemeral harness) |
| -- | ----------- | ------------------------ |
| **ext4** | Host root (`/dev/sda1`) private dir | `/tmp/e1-matrix-*/mnt/host` |
| **xfs** | 320 MiB loop image | `/tmp/e1-matrix-*/mnt/xfs` |
| **btrfs** | 128 MiB loop image | `/tmp/e1-matrix-*/mnt/btrfs` |
| **vfat** | 32 MiB loop image (`uid/gid` mount opts) | `/tmp/e1-matrix-*/mnt/vfat` |
| **APFS** | GitHub Actions macos-latest (arm64, APFS) | **PASS** live RenameatxNp matrix (bead 962; run 30626724956) |

## Exact commands

```bash
# From repository root — mounts ext4 workdir + xfs/btrfs/vfat loopbacks, runs all probes:
./integration/hostile/e1/scripts/e1-mount-matrix.sh

# Host-FS-only (no sudo), still exercises contracts 1–4, 6 on current FS:
GOTOOLCHAIN=go1.26.5 go test ./integration/hostile/e1/ -count=1 -v -timeout 120s

# Darwin commit adapter compile (SV-02 RenameatxNp symbol presence):
GOTOOLCHAIN=go1.26.5 GOOS=darwin GOARCH=amd64 \
  go test -c -o /tmp/e1-darwin.test ./integration/hostile/e1/
```

`E1_FS_ROOTS` used by the matrix run (from `e1-logs/machine.txt`):

```
E1_FS_ROOTS=ext4:/tmp/e1-matrix-3247683/mnt/host,xfs:/tmp/e1-matrix-3247683/mnt/xfs,btrfs:/tmp/e1-matrix-3247683/mnt/btrfs,vfat:/tmp/e1-matrix-3247683/mnt/vfat
```

## Expected behavior (specification)

| Contract | Spec | Expectation |
| -------- | ---- | ----------- |
| 1 | §31.2–31.3 | No-follow `openat(O_NOFOLLOW\|O_DIRECTORY)` parent walk; custody rejects shared-writable non-sticky; sticky `/tmp` allowed; symlink component → `fs.unsafe_path` |
| 2 | §31.5 | Stage via `mkdirat` mode `0700`, name `.foundry-<name>-<random>`, identity re-check; **no** `MkdirTemp` / pathname staging |
| 3 | §34.4 | Serialized `fchdir(stage_fd)` → child `Dir=""` → `fchdir(orig_fd)`; pathname swap of stage entry during start must not redirect child |
| 4 | §31.7–31.8 | Linux `Renameat2(RENAME_NOREPLACE)`; Darwin `RenameatxNp(RENAME_EXCL\|RENAME_NOFOLLOW_ANY)`; classify by identity, not errno alone |
| 5 | §51.2 E1 / OQ-400 | Positive: APFS, ext4, xfs, btrfs. Negative: FS without no-replace → `fs.rename_unsupported`, stage preserved |
| 6 | §11.3 SV-01–03 | `os.Root` method set; x/sys exclusive renames; `openat`/`fchdir` object continuity after pathname rename |

**Forbidden:** pathname fallback, `os.MkdirTemp` staging, automatic stage deletion (§31.6).

## Observed behavior

### Pass/fail table (matrix run 2026-07-30T20:21:44Z)

| Probe | ext4 | xfs | btrfs | vfat | Notes |
| ----- | ---- | --- | ----- | ---- | ----- |
| Happy path stage+commit | PASS | PASS | PASS | n/a (dedicated FAT probe) | `Renameat2(RENAME_NOREPLACE)` + identity match |
| Symlink parent → `fs.unsafe_path` | PASS | PASS | PASS | — | `openat` errno `ENOTDIR` / not a directory |
| Shared-writable non-sticky → `fs.namespace_not_private` | PASS | PASS | PASS | — | mode `0777` rejected |
| Sticky `/tmp` permitted | PASS | — | — | — | SV-05 |
| Child cwd + pathname swap | PASS | PASS | PASS | — | child `cat` sentinel after stage rename |
| Parent pathname swap; retained handle | PASS | PASS | PASS | — | stage create OK; reobserve → `fs.parent_moved`; stage preserved |
| Two-process exclusive commit EEXIST | PASS | PASS | PASS | — | 1 winner / 1 loser; loser stage preserved |
| FAT exclusive rename | — | — | — | PASS | **Supports** `RENAME_NOREPLACE` on Linux 6.8 fat (see note) |
| `fs.rename_unsupported` fail-closed | PASS (injected `ENOTSUP`) | — | — | — | stage preserved; dest absent |
| §31.8 identity classification matrix | PASS (unit) | | | | all 6 classes |
| SV-01 `os.Root` no escape | PASS | | | | rejects `../` and absolute |
| SV-03 openat/fchdir continuity | PASS | | | | same dev/ino after rename |
| Darwin adapter cross-compile | PASS | | | | links `_renameatx_np` |

Full verbose log: [`e1-logs/go-test.txt`](e1-logs/go-test.txt).

### FAT / fail-closed note (contract 5 refinement)

Gate text expected a FAT/exFAT **negative** (`fs.rename_unsupported`). On **Linux 6.8.0-134**, the in-kernel `fat` driver implements `RENAME_NOREPLACE` correctly:

- first exclusive rename → committed  
- second exclusive rename to same dest → `EEXIST` / conflict class, **loser stage preserved**

This does **not** contradict Section 31.7 (which maps `ENOTSUP`/`EINVAL` → `fs.rename_unsupported` *when* the kernel reports unsupported). The fail-closed path is proven by:

1. `TestE1_RenameUnsupported_FailClosedInjected` (`ENOTSUP` → `fs.rename_unsupported`, stage kept)
2. `TestMapUncommittedError_ENOTSUP_EINVAL` (`ENOTSUP`/`EOPNOTSUPP`/`EINVAL` mapping)

No pathname fallback was invented for FAT.

### Darwin / APFS

| Item | Status |
| ---- | ------ |
| `commit_darwin.go` uses `unix.RenameatxNp(..., RENAME_EXCL\|RENAME_NOFOLLOW_ANY)` | Present |
| `GOOS=darwin go test -c` (amd64 + arm64) | Exit 0; binary links `renameatx_np` ([`e1-logs/darwin-crosscompile.txt`](e1-logs/darwin-crosscompile.txt)) |
| FS label on Darwin | `detectFSType` via `Statfs.Fstypename` (`fsdetect_darwin.go`) → `apfs` |
| Live APFS run on macOS | **PASS** — GitHub Actions `macos-latest` run `30626724956` (`darwin-evidence` / `macos-e1-apfs`); logs [`e1-logs/darwin-apfs-*.txt`](e1-logs/) |

Primary-source SV-02 already documents Darwin rename APIs; E1 adds an executable, build-tagged adapter ready for promotion.

#### Live APFS procedure (go-foundry-cli-962)

On a macOS host with APFS (Tailscale peer `roberts-macbook-pro` when online):

```bash
# From repository root on the Mac:
./integration/hostile/e1/scripts/e1-darwin-apfs.sh

# Equivalent one-liner:
GOTOOLCHAIN=go1.26.5 go test ./integration/hostile/e1/ -count=1 -v -timeout 180s
```

Expected evidence artifacts (written by the harness):

| File | Contents |
| ---- | -------- |
| `e1-logs/darwin-apfs-machine.txt` | hostname, `sw_vers`, APFS/`diskutil`, Go, `E1_FS_ROOTS` |
| `e1-logs/darwin-apfs-go-test.txt` | full `-v` probe log; must show `RenameatxNp(RENAME_EXCL\|RENAME_NOFOLLOW_ANY)` |
| `e1-logs/darwin-apfs-summary.txt` | exit code + pass/fail extraction |

Required pass table (append under this section after a green live run): happy path, custody (symlink + shared-writable + sticky), child-cwd pathname swap, parent-swap retained handle, two-writer EEXIST, §31.8 identity unit, SV-01/SV-03, platform rename syscall label. **Do not revise Section 31** unless a probe CONTRADICTS.

### Raw log excerpts

Happy-path commit (ext4):

```
E1PROBE ... fs=ext4 probe=commit step=classified syscall=Renameat2(RENAME_NOREPLACE)
  args=".foundry-proj-8861d4fb5fd1a012 -> proj" errno= outcome=pass
  detail="syscall ok; destination identity matches recorded stage"
```

Child cwd after pathname swap (xfs/btrfs/ext4 same shape):

```
E1PROBE ... probe=child_cwd step=pathname_swap ... outcome=pass
  detail="pathname renamed under retained stage fd"
E1PROBE ... probe=child_cwd step=start syscall=exec args="[/bin/cat e1-cwd-sentinel.txt] Dir=<empty>" outcome=pass
E1PROBE ... probe=child_cwd step=swap_proof outcome=pass
  detail="child read sentinel after pathname rename of stage entry"
```

Two-process EEXIST (btrfs):

```
... Renameat2 ... -> proj errno= outcome=pass detail="syscall ok; destination identity matches recorded stage"
... Renameat2 ... -> proj errno=file exists outcome=fail
  detail="fs.destination_exists: destination present with different identity; ... (stage preserved: .foundry-proj-...)"
... probe=eexist_race step=done outcome=pass detail="winners=1 losers=1"
```

Fail-closed unsupported:

```
E1PROBE ... probe=rename_unsupported step=injected_enotsup
  errno=operation not supported outcome=pass
  detail="fs.rename_unsupported stage=.foundry-proj-... preserved; stage present with recorded identity; destination absent"
```

## Architecture audit (spike)

| Check | Result |
| ----- | ------ |
| Pathname fallback / `os.MkdirTemp` staging | **Absent** — stages via `mkdirat` on parent fd only |
| Automatic stage deletion API | **Absent** — `Stage.Close` releases fds only; §31.6 honored |
| Production `internal/fsx` | **Not created** (Phase 2 still gated; spike is under `integration/hostile/e1`) |

## P2.1.e promotion path (fixtures without rewrite)

**Promoted (go-foundry-cli-qd8):** permanent suite at
[`integration/hostile/fsx/`](../../integration/hostile/fsx/) (`//go:build hostile`)
against production `internal/fsx`; injection matrix in
`internal/fsx/hostile_test.go`; CI job `linux-hostile-fsx`; multi-FS script
`integration/hostile/fsx/scripts/fsx-mount-matrix.sh`. E1 remains the
promotion-source spike (still green under CI).

| Spike path | Promotion target |
| ---------- | ---------------- |
| `integration/hostile/e1/` | **Kept** as evidence spike; permanent probes in `integration/hostile/fsx` + `internal/fsx` (`//go:build hostile`) |
| `AcquireParent` / `checkCustody` | `internal/fsx` parent walk (P2.1.a) — exercised by hostile custody matrix |
| `CreateStage` + `os.Root` writer | `internal/fsx` stage + rooted writer (P2.1.b) |
| `Commit` + `ClassifyCommit` + platform `exclusiveRename` | `internal/fsx` commit (P2.1.c); keep `commit_linux.go` / `commit_darwin.go` build tags |
| `RunInStage` / `ProvePathnameSwap` | `internal/toolrun` child-cwd protocol (P2.2) |
| `e1_test.go` probe table + `E1_FS_ROOTS` + `scripts/e1-mount-matrix.sh` | `FOUNDRY_FSX_ROOTS` / `E1_FS_ROOTS` + `scripts/fsx-mount-matrix.sh`; CI `linux-hostile-fsx` |
| `ProbeLog` JSON/line format | `integration/hostile/fsx` ProbeLog (`HOSTILEFSX` lines + JSON artifacts) |

**Do not rewrite:** identity classification matrix, EEXIST two-writer race, parent-swap reobserve, child-cwd pathname swap, custody mode/sticky cases.

## Result

**CONFIRMS** Section 31 (descriptor-relative transaction) and Section 34.4 (descriptor-bound child cwd) on **Linux** for **ext4, xfs, btrfs**, with exclusive no-replace commit, identity classification, custody/no-follow walk, and fail-closed `fs.rename_unsupported` handling. SV-01 / SV-02 / SV-03 executable. No pathname fallback; no automatic stage deletion.

**Operational refinements recorded (not contradictions of §31):**

1. **vfat on Linux 6.8** supports `RENAME_NOREPLACE` (exclusive EEXIST works); fail-closed unsupported path proven via `ENOTSUP` injection rather than live FAT ENOTSUP.
2. **Live APFS / Darwin rename execution** awaits a macOS host (adapter present and cross-compiles). Track under P2.1.e macOS leg / follow-up bead — does not waive Linux E1 gate evidence above.

**OQ-400 / RSK-400:** architecture executable on supported Linux filesystems; proceed to Phase 2 `internal/fsx` implementation using this spike as the primitive template. Live APFS confirmation is a promotion/CI obligation, not a Section 31 redesign trigger.


## Live APFS pass table (2026-07-31, CI macos-latest)

| Field | Value |
| ----- | ----- |
| Runner | GitHub Actions `macos-latest` (darwin/arm64) |
| Workflow | `.github/workflows/darwin-evidence.yml` job `macos-e1-apfs` |
| Run | https://github.com/robertguss/go-foundry-cli/actions/runs/30626724956 |
| OS | macOS 26.4 (Darwin 25.4.0) |
| FS | APFS (`/System/Volumes/Data`) |
| Go | go1.26.5 |
| Exclusive rename | `RenameatxNp(RENAME_EXCL\|RENAME_NOFOLLOW_ANY)` |
| Harness | `./integration/hostile/e1/scripts/e1-darwin-apfs.sh` |
| Package result | `go test ./integration/hostile/e1/ -count=1` **PASS** (exit 0) |
| Raw logs | [`e1-logs/darwin-apfs-machine.txt`](e1-logs/darwin-apfs-machine.txt), [`darwin-apfs-go-test.txt`](e1-logs/darwin-apfs-go-test.txt), [`darwin-apfs-summary.txt`](e1-logs/darwin-apfs-summary.txt) |

**Outcome:** CONFIRMS Section 31 Darwin adapter on live APFS. No Section 31 revision required.
