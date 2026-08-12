# E5 — Version/action lock record

- **Date:** 2026-07-30
- **Gated phase:** Phase 1 **exit** (Section 51.2, REQ-240)
- **Resolves:** RSK-311 (version evidence staleness between research and implementation)
- **Inherits:** Stage 4 TV-01–TV-18 base (2026-07-29); re-confirmed this session
- **Lock manifest:** [`catalog/versions.toml`](../../catalog/versions.toml) (Section 33.4 / REQ-090)
- **Spike / promotable fixture:** [`integration/hostile/e5/`](../../integration/hostile/e5/)
- **Raw logs:** [`docs/evidence/e5-logs/`](e5-logs/)

## Machine / tool versions

| Field | Value |
| ----- | ----- |
| Hostname | `dev-box` |
| OS | Ubuntu 24.04.4 LTS (`PRETTY_NAME`) |
| Kernel | `Linux 6.8.0-134-generic #134-Ubuntu SMP PREEMPT_DYNAMIC x86_64` |
| Arch | `linux/amd64` |
| Host Go (mise) | `go1.26.4` |
| Effective toolchain | `go1.26.5` via `GOTOOLCHAIN=go1.26.5` |
| GOROOT (toolchain) | `…/golang.org/toolchain@v0.0.1-go1.26.5.linux-amd64` |
| Module proxy | `https://proxy.golang.org,direct` |
| Checksum DB | `sum.golang.org` |
| `gh` | `2.96.0` |
| `curl` | `8.5.0` |
| Python | `3.12.3` |

Source: [`e5-logs/machine.txt`](e5-logs/machine.txt).

## Exact commands

```bash
# From repository root

# Module pin probes (proxy.golang.org + sum.golang.org):
export GOTOOLCHAIN=go1.26.5
export GOPROXY=https://proxy.golang.org,direct
export GOSUMDB=sum.golang.org
go list -m -json github.com/spf13/cobra@v1.10.2
go list -m -json github.com/BurntSushi/toml@v1.6.0
go list -m -json golang.org/x/mod@v0.38.0
go list -m -json golang.org/x/sys@v0.47.0
go list -m -json github.com/google/go-cmp@v0.7.0
go list -m -json github.com/rogpeppe/go-internal@v1.15.0
go list -m -json charm.land/bubbletea/v2@v2.0.8
go list -m -json github.com/charmbracelet/bubbles/v2@v2.1.1
go list -m -json charm.land/lipgloss/v2@v2.0.5
go list -m -json honnef.co/go/tools@v0.7.0
go list -m -json golang.org/x/vuln@v1.6.0

# Tool release tags → commit SHAs (GitHub API):
gh api repos/golang/go/git/ref/tags/go1.26.5
gh api repos/goreleaser/goreleaser/git/ref/tags/v2.17.1
gh api repos/anchore/syft/git/ref/tags/v1.44.0
gh api repos/dominikh/go-tools/git/ref/tags/2026.1
# (plus actions/* and goreleaser/goreleaser-action, anchore/sbom-action tags)

# Go 1.26.5 binary distribution present:
curl -fsSIL https://go.dev/dl/go1.26.5.linux-amd64.tar.gz

# Structural lock suite (offline, promotable):
GOTOOLCHAIN=go1.26.5 go test ./integration/hostile/e5/ -count=1 -v -timeout 60s

# Full module regression:
GOTOOLCHAIN=go1.26.5 go test ./... -count=1 -timeout 180s
```

Captured:

| Log | Path |
| --- | ---- |
| Module probes | [`e5-logs/module-probes.txt`](e5-logs/module-probes.txt) |
| Action/tool tag→SHA | [`e5-logs/action-tool-probes.txt`](e5-logs/action-tool-probes.txt) |
| Machine-readable pin map | [`e5-logs/pins-resolved.json`](e5-logs/pins-resolved.json) |
| Structural suite | [`e5-logs/go-test.txt`](e5-logs/go-test.txt) (`EXIT:0`) |
| Machine envelope | [`e5-logs/machine.txt`](e5-logs/machine.txt) |

## Expected behavior (specification)

| Contract | Spec | Expectation |
| -------- | ---- | ----------- |
| 1 | §12 | Every component in the Section 12 pin table is exact and still exists at its primary source |
| 2 | §12.1 / CVE-2026-39822 | Go **1.26.5** is the toolchain pin and includes the `os.Root` trailing-slash fix |
| 3 | §33.1 / REQ-224 | GitHub Actions are pinned by **full commit SHA** with human-readable tag recorded |
| 4 | §33.4 / REQ-090 | `catalog/versions.toml` is the **single** machine-readable lock; no second versions lock |
| 5 | §33.3 / RSK-311 | Stage 4 TV base is re-verified (not merely inherited as prose) |
| 6 | §33.2 | Pin drift → filed revision / follow-up, **not** silent golden edits |

**Forbidden:** floating tags (`latest`); ranges; prose-only satisfaction of E5; unused dual lock files.

## Stage 4 TV inheritance map (re-confirmed 2026-07-30)

| TV | Pin | Primary source probe | Result |
| -- | --- | -------------------- | ------ |
| TV-01 | Go **1.26.5** (+ CVE-2026-39822) | `go version` under `GOTOOLCHAIN=go1.26.5`; tag `go1.26.5` → `c19862e5…`; dl.google.com 200; golang-announce | **PASS** |
| TV-02 | Cobra **v1.10.2** | `go list -m -json` + proxy info | **PASS** |
| TV-03 | BurntSushi/toml **v1.6.0** | `go list -m -json` + proxy info | **PASS** |
| TV-04 | x/mod **v0.38.0** | `go list -m -json` + proxy info | **PASS** |
| TV-05 | x/sys **v0.47.0** | `go list -m -json` + proxy info | **PASS** |
| TV-06 | go-cmp **v0.7.0** | `go list -m -json` + proxy info | **PASS** |
| TV-07 | testscript (go-internal **v1.15.0**) | `go list -m -json` + proxy info | **PASS** |
| TV-08 | Bubble Tea **v2.0.8** (`charm.land/bubbletea/v2`) | `go list -m -json` + proxy info | **PASS** |
| TV-09 | Bubbles **v2.1.1** (`charm.land/bubbles/v2`; GitHub mirror path same origin hash) | `go list -m -json` | **PASS** |
| TV-10 | Lip Gloss **v2.0.5** (`charm.land/lipgloss/v2`) | `go list -m -json` + proxy info | **PASS** |
| TV-11 | Staticcheck **v0.7.0 / 2026.1** | Origin ref `refs/tags/2026.1` hash `ff63afaf…` | **PASS** |
| TV-12 | govulncheck **v1.6.0** (`golang.org/x/vuln`) | `go list -m -json` + proxy info | **PASS** |
| TV-13 | GoReleaser OSS **v2.17.1** | GitHub release tag → `83f4c19a…` | **PASS** |
| TV-14 | Syft **v1.44.0** | GitHub release tag → `8cb78ce4…` | **PASS** |
| TV-15 | `actions/checkout` full SHA | tag `v5.1.0` → `fbc6f399…` | **PASS** |
| TV-16 | `actions/setup-go` full SHA | tag `v6.5.0` → `924ae3a1…` | **PASS** |
| TV-17 | Release/distribution action set (upload/download/attest/dependency-review/goreleaser-action/sbom-action) full SHAs | GitHub tag resolution for each | **PASS** |
| TV-18 | Lock single-file + Section 12 coverage suite | `go test ./integration/hostile/e5/` | **PASS** |

## Observed behavior

### Pass/fail table (run 2026-07-30T20:46:52Z structural; online probes ~20:43–20:45Z)

| Probe | Path | Result |
| ----- | ---- | ------ |
| Go 1.26.5 toolchain active | `GOTOOLCHAIN=go1.26.5 go version` → `go1.26.5` | **PASS** |
| Go 1.26.5 download | `curl -I https://go.dev/dl/go1.26.5.linux-amd64.tar.gz` → 302→200 | **PASS** |
| CVE-2026-39822 in 1.26.5 | golang-announce + go#79005 + release notes | **PASS** (documented in lock) |
| All Section 12 modules resolve | proxy.golang.org via `go list -m -json` | **PASS** (12/12 module probes OK) |
| Staticcheck 2026.1 ≡ v0.7.0 | Origin.Ref=`refs/tags/2026.1` | **PASS** |
| GoReleaser v2.17.1 tag | GitHub releases | **PASS** |
| Syft v1.44.0 tag | GitHub releases | **PASS** |
| 8 action full SHAs | `gh api …/git/ref/tags/…` | **PASS** |
| `catalog/versions.toml` exists as sole lock | catalog/ directory scan | **PASS** |
| Structural suite Section 12 coverage | `TestE5VersionActionLock` | **PASS** |
| Action SHA 40-hex + tags | `ValidateActions` | **PASS** |
| CVE doc in lock | `TestE5CVEGoPinDocumented` | **PASS** |
| Full module `go test ./...` | e1+e3+e4+e5 | **PASS** |

All required `E5PROBE` outcomes: **pass** (0 fail). Full log: [`e5-logs/go-test.txt`](e5-logs/go-test.txt).

### Go 1.26.5 / CVE-2026-39822

| Item | Evidence |
| ---- | -------- |
| Announce | https://groups.google.com/g/golang-announce/c/OrmQE_Yp5Sc — Go 1.26.5 + 1.25.12 security releases |
| Issue | https://go.dev/issue/79005 — `os.Root` escape via symlink + trailing slash |
| CVE | CVE-2026-39822 |
| Fix commit (1.26 branch) | `f9ef7f55988f03afeb3b8354367d0fa8d053683d` (release-branch.go1.26) |
| Tag commit | `c19862e5f8415b4f24b189d065ed739517c548ba` (`go1.26.5`) |
| Downloads | https://go.dev/dl/ |

**CONFIRMS** Section 12.1 Go **1.26.5 exact** pin remains correct and security-current for the `os.Root` path class used by descriptor-relative work.

### Module pins (primary: proxy.golang.org / sum.golang.org)

| ID | Module | Version | Origin hash (abbrev) | Result |
| -- | ------ | ------- | -------------------- | ------ |
| cobra | `github.com/spf13/cobra` | v1.10.2 | `88b30ab8…` | **OK** |
| toml | `github.com/BurntSushi/toml` | v1.6.0 | `52534926…` | **OK** |
| x_mod | `golang.org/x/mod` | v0.38.0 | `792ac169…` | **OK** |
| x_sys | `golang.org/x/sys` | v0.47.0 | `9e7e939d…` | **OK** |
| go_cmp | `github.com/google/go-cmp` | v0.7.0 | `9b12f366…` | **OK** |
| testscript | `github.com/rogpeppe/go-internal` | v1.15.0 | `8b1dcd46…` | **OK** |
| bubbletea | `charm.land/bubbletea/v2` | v2.0.8 | `fc707bb7…` | **OK** |
| bubbles | `charm.land/bubbles/v2` | v2.1.1 | `d2b2217d…` | **OK** |
| lipgloss | `charm.land/lipgloss/v2` | v2.0.5 | `5bd778d0…` | **OK** |
| staticcheck | `honnef.co/go/tools` | v0.7.0 / 2026.1 | `ff63afaf…` | **OK** |
| govulncheck | `golang.org/x/vuln` | v1.6.0 | `19b0bb6a…` | **OK** |

### Tool releases (primary: GitHub release tags)

| Tool | Version | Tag commit | Result |
| ---- | ------- | ---------- | ------ |
| goreleaser | v2.17.1 | `83f4c19a5c5c0b9efef6bf2aedc6805bbcb9dfe2` | **OK** |
| syft | v1.44.0 | `8cb78ce40ced6a731fb83f2a491a67444f541bf1` | **OK** |

**Note (not a contradiction):** Syft has newer tags as of lock date (e.g. v1.50.0). Section 12 pin **v1.44.0** is retained. Bumps go through catalog release + golden regeneration (Section 33.2), not silent edit.

### GitHub Action full SHAs (Foundry + generated workflows)

| Action | Tag | Full SHA | Scope |
| ------ | --- | -------- | ----- |
| `actions/checkout` | v5.1.0 | `fbc6f3992d24b796d5a048ff273f7fcc4a7b6c09` | foundry-ci, core:ci, distribution |
| `actions/setup-go` | v6.5.0 | `924ae3a1cded613372ab5595356fb5720e22ba16` | foundry-ci, core:ci, distribution |
| `actions/upload-artifact` | v5.0.0 | `330a01c490aca151604b8cf639adc76d48f6c5d4` | foundry-ci, distribution |
| `actions/download-artifact` | v5.0.0 | `634f93cb2916e3fdff6788551b99b062d0335ce0` | foundry-ci, distribution |
| `actions/dependency-review-action` | v4.9.0 | `2031cfc080254a8a887f58cffee85186f0e49e48` | distribution |
| `actions/attest-build-provenance` | v3.2.0 | `96278af6caaf10aea03fd8d33a09a777ca52d62f` | distribution, foundry-release |
| `goreleaser/goreleaser-action` | v6.4.0 | `e435ccd777264be153ace6237001ef4d979d3a7a` | distribution, foundry-release |
| `anchore/sbom-action` | v0.20.11 | `43a17d6e7add2b5535efe4dcae9952337c479a93` | distribution, foundry-release |

Rendered workflow form (normative style):

```yaml
uses: actions/checkout@fbc6f3992d24b796d5a048ff273f7fcc4a7b6c09 # v5.1.0
```

### Single lock manifest

| Check | Result |
| ----- | ------ |
| Path | `catalog/versions.toml` only |
| No `versions.lock` / alternate lock beside it | **PASS** |
| schema | `1` |
| toolchain.go | `1.26.5` |
| modules / tools / actions counts | 9 / 4 / 8 |
| Unique entry ids | **PASS** |
| Every Section 12 pin present with exact version | **PASS** |

### Raw log excerpts

Toolchain:

```
go version go1.26.5 linux/amd64
GOTOOLCHAIN=go1.26.5
```

Module probe (representative):

```
=== TV-01: github.com/spf13/cobra@v1.10.2 ===
RESULT=OK version=v1.10.2 time=2025-12-03T23:51:15Z
  ref=refs/tags/v1.10.2 hash=88b30ab89da2d0d0abb153818746c5a2d30eccec
```

Staticcheck alias:

```
"Version": "v0.7.0",
"Origin": { "Ref": "refs/tags/2026.1", "Hash": "ff63afafc529279f454e02f1d060210bd4263951" }
```

Action probe (representative):

```
OK   actions/checkout@v5.1.0 -> fbc6f3992d24b796d5a048ff273f7fcc4a7b6c09
OK   actions/setup-go@v6.5.0 -> 924ae3a1cded613372ab5595356fb5720e22ba16
```

Structural suite:

```
E5PROBE step=locate outcome=pass path=…/catalog/versions.toml
E5PROBE step=parse outcome=pass schema=1 go=1.26.5 modules=9 tools=4 actions=8
E5PROBE step=section12 outcome=pass pins=14
E5PROBE step=actions outcome=pass actions=8 required=8
E5PROBE step=single_lock outcome=pass file=catalog/versions.toml
E5SUMMARY ok=true go=1.26.5 modules=9 tools=4 actions=8
PASS
```

### Working-tree observations (not Section 12 contradictions)

| Observation | Disposition |
| ----------- | ----------- |
| Spike `go.mod` had pulled `golang.org/x/sys v0.46.0` (Bubble Tea E4) while Section 12 / lock pin is **v0.47.0** | **Aligned during E5:** `go get golang.org/x/sys@v0.47.0` + `go mod tidy`; full `./...` suite green. No Section 12 pin revision. |
| Syft newer than v1.44.0 on GitHub | **No pin revision.** Retain Section 12 pin; Dependabot/catalog release owns bumps. |
| Action tags chosen at lock time (v5.1.0 checkout, v6.5.0 setup-go, …) | First establishment of action SHAs in-repo (Stage 4 TV base covered module/tool pins; full action SHA lock completed by E5). |

**Pin revisions filed:** none (all Section 12 pins **CONFIRMED** against primary sources).

## P1.3 / catalog promotion path (fixtures without rewrite)

| Spike path | Promotion target |
| ---------- | ---------------- |
| `catalog/versions.toml` | Embedded via `go:embed` in `internal/catalog` (bead `wkz`) |
| `integration/hostile/e5` Section 12 table | `internal/catalog` lock validation unit tests |
| `ValidateActions` 40-hex + tag rules | Catalog validation + workflow golden checks |
| `e5-logs/*` probe commands | CI “re-lock” / Dependabot review checklist |
| Single-lock directory scan | Catalog package reject dual lock files |

**Do not rewrite:** Section 12 expected pin table, full-SHA action map, CVE-2026-39822 documentation on the Go pin.

## Result

**CONFIRMS** Section 12 (Final Technology Stack), Section 33.1–33.4 (exact pins + single lock manifest), REQ-090, REQ-135, and REQ-224 for every Section 12 pin and every action SHA recorded in `catalog/versions.toml`, re-verified against primary sources on **2026-07-30**.

- Stage 4 TV-01–TV-18 base: **re-confirmed**
- `catalog/versions.toml`: **single lock**, schema 1, complete
- Pin revisions: **none required**
- RSK-311: mitigated for Phase 1 exit by this record

Phase 1 may exit on the version/action lock gate (E5) once remaining Phase 1 product exit criteria are met; this record satisfies the E5 portion of REQ-240.
