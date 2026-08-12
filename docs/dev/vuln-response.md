# Vulnerability response (cf1 / REQ-163)

## Generated projects

1. **PR/default:** no govulncheck (FND-016 latency).
2. **Weekly/strict:** `go tool govulncheck ./...` in generated `strict.yml`.
3. **Promotion:** owners may promote govulncheck to the required PR job when
   risk justifies it (RSK-403 / `docs/testing.md`).

## Foundry repository

1. **strict.yml** weekly job runs `go tool govulncheck ./...`.
2. **Dependabot** (`.github/dependabot.yml`) groups Go modules and GitHub
   Actions weekly.
3. Action pins are full SHAs (`TestWorkflowActionSHAPins`); bump via catalog
   lock + goldens for generated workflows.

## Response playbook

1. Identify CVE via govulncheck or Dependabot.
2. Prefer exact pin bump in `catalog/versions.toml` (or go.mod for Foundry BOM).
3. Regenerate goldens; run `go test ./internal/catalog/ ./internal/archtest/`.
4. For generated consumers: cut a Foundry patch release; advise
   `foundry generate` re-run or manual pin bump.
5. Do not introduce floating version ranges.
