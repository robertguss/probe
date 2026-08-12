# Agent instructions for probe

## What this is

`probe` is an agent-first HTTP spike CLI. Hit APIs non-interactively, save redacted request/response artifacts under `.probe/`, and promote survivors into a personal catalog so you stop rediscovering the same endpoints in ad-hoc scripts.

## Secrets (non-negotiable)

- `probe` does **not** load `.env` files. Secrets enter only via process environment.
- Typical invocation: `fnox exec -- probe hit …`
- Config stores **env var names** / auth profile metadata only — never secret values.
- Do not `cat` secret stores or ask the user to paste tokens into chat.
- Never print secret values in stdout/stderr/`doctor`/`--json` (only set/missing).
- Saved artifacts redact `Authorization`, `Cookie`, `Set-Cookie`, and token-like query params.

## Rate limits

- Retries on HTTP 429 and 503; honors `Retry-After`; respects `--retries` / `--max-wait` / `--no-retry`.
- Client throttle: `--rps` and/or catalog `rate_limit.rps`.
- Still 429 after retries → **exit 5** and `error.code=rate_limited`.

## Agent contract

- Prefer `--json` (or non-TTY) for machine envelopes on stdout; diagnostics on stderr.
- No interactive prompts. Unknown flags → exit 2 with an example invocation.
- Start with `probe quickstart --json` and `probe schema --json`.
- Prefer `probe catalog show <api> --json` before inventing a one-off HTTP script.
- Global `--json` is the envelope flag. Request body is `--body` (or `--file` / `--file -` for stdin).

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success (`hit`: HTTP 2xx after retries) |
| 1 | Transport / internal |
| 2 | Usage / unknown flag / bad args |
| 3 | HTTP 4xx (non-429) |
| 4 | HTTP 5xx (after retries) |
| 5 | Rate limited (429 after retries exhausted) |

## Golden path

```bash
probe init --json
probe auth set canvas --type bearer --token-env CANVAS_TOKEN --json
fnox exec -- probe hit GET /api/v1/courses --auth canvas --base "$BASE" --save courses --json
probe note "pending via Link header" --json
probe promote canvas --endpoint get-courses --json
probe catalog show canvas --json
```

Later sessions:

```bash
probe catalog show canvas --json
fnox exec -- probe hit GET /api/v1/courses --auth canvas --api canvas --json
```
