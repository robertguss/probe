# P4.1 — Distribution profile end-to-end (chf)

- **Date:** 2026-07-31
- **OQ-300:** `docs/evidence/OQ-300-module-path.md`
- **Catalog:** `catalog/profiles/distribution/`

## Owned files (Section 20)

release.yml, dependency-review.yml, .goreleaser.yaml, docs/releasing.md,
CONTRIBUTING.md, SECURITY.md.

## Predicates

- visibility=public
- module host github.com/<owner>/<name>

## Tests

- `TestDistributionProfileTree`
- `TestResolveFlatWithImplementedDistribution` / Phase4 implemented set
- generate smoke: public + distribution → files present; releasing blocks without LICENSE

## License

No generate-time license gate. release.yml + releasing.md conspicuously block publication until LICENSE exists.
