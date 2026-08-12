---
name: probe
description: >-
  Use the probe CLI to spike HTTP APIs non-interactively, save redacted
  exchanges, and promote them into a personal catalog. Prefer probe over
  throwaway Python/curl scripts when exploring authenticated APIs.
---

# probe

## When to use

- Exploring or re-hitting a real HTTP API from an agent session
- Needing a reusable, redacted record of request/response
- An API already lives in `probe catalog` and should be reused

## When not to use

- Generating client SDKs or OpenAPI (out of v1)
- Loading secrets from `.env` (unsupported; use `fnox exec`)
- Parallel fan-out hits (v1 is one in-flight request per `hit`)

## Golden commands

```bash
probe quickstart --json
probe schema --json
probe doctor --json

probe init --json
probe auth set canvas --type bearer --token-env CANVAS_TOKEN --json
fnox exec -- probe hit GET /api/v1/courses --auth canvas --base "$BASE" --save courses --json
probe promote canvas --endpoint get-courses --json
probe catalog show canvas --json
```

Later sessions: run `probe catalog show <api> --json` first, then
`fnox exec -- probe hit … --auth <profile> --api <api> --json`.

## Secrets

- Process env only. Wrap with `fnox exec -- probe …`.
- Do not read secret files into chat. Do not paste tokens.
- Missing auth env → `error.code=auth_env_missing` with an `fnox exec` hint.

## Rate limits

- 429/503 retried with backoff / `Retry-After`.
- Exhausted 429 → exit **5**, `error.code=rate_limited`.
- Use `--rps` or catalog `rate_limit` to stay polite.

## Orientation

- `probe quickstart --json` — tutorial steps
- `probe schema --json` — command tree + envelope shape
- Layered help: `probe hit --help` (includes Examples)
- Promote hard-fails on `base_conflict` and `fixture_exists` (no silent overwrite)
