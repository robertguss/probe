# Releasing

Public binary-release scaffolding is provided by the **`distribution`**
profile. This document is the publication authority for owners.

## Publication blocker (FND-019 / REQ-076)

**Do not publish a release until:**

1. You have **added and reviewed** an appropriate `LICENSE` file for this
   repository (Foundry never inspects, selects, generates, or validates a
   license at generate time — the destination did not exist yet).
2. Repository settings match the release workflow: tag-only release job,
   least privileges, immutable releases enabled where available.
3. You have verified checksums / SBOM / attestations on a dry-run or
   pre-release tag.

Publication without those steps is an **owner process failure**, not a
Foundry generate failure.

## How to release

1. Ensure `main` is green (Core CI + strict checks as appropriate).
2. Tag: `git tag vX.Y.Z && git push origin vX.Y.Z`.
3. GitHub Actions `release.yml` runs GoReleaser (`CGO_ENABLED=0`) for
   darwin/linux × amd64/arm64, produces checksums, SBOM (Syft), and build
   provenance attestations.
4. Verify the GitHub Release assets before announcing.

## Install (consumers)

After a public tag:

```bash
go install github.com/<owner>/<repo>/cmd/<binary>@vX.Y.Z
```

Exact module path is the project `module` field (must be
`github.com/<owner>/<name>` for this profile).

## Security

- Release workflow is **tag-triggered only** (never untrusted PRs).
- Actions are full-SHA pinned.
- Dependency review runs on pull requests (see `dependency-review.yml`).
