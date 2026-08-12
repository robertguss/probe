# probe

Agent-first Go CLI that turns throwaway API spikes into a reusable personal **catalog** of real (redacted) requests/responses and notes.

```bash
go install github.com/robertguss/probe/cmd/probe@latest
# or: go build -o probe ./cmd/probe
```

## Golden path

```bash
probe init --json
probe auth set canvas --type bearer --token-env CANVAS_TOKEN --json
fnox exec -- probe hit GET /api/v1/courses --auth canvas --base "$BASE" --save courses --json
probe note "pending via Link header" --json
probe promote canvas --endpoint get-courses --json
```

Later:

```bash
probe catalog show canvas --json
fnox exec -- probe hit GET /api/v1/courses --auth canvas --api canvas --json
```

## Agent contract

- Inputs are flags/args only (no interactive prompts).
- `--json` or non-TTY → JSON envelope on stdout; diagnostics on stderr.
- Global `--json` = envelope. Request body = `--body` (or `--file` / `--file -`).
- Unknown flags → exit 2 with an example invocation.
- Discover with layered help: `probe`, `probe hit --help` (each has Examples).

### Envelope

```json
{
  "ok": true,
  "command": "probe hit",
  "data": {},
  "error": null,
  "meta": { "exitCode": 0, "version": "0.1.0", "requestId": "001" }
}
```

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Transport / internal |
| 2 | Usage / unknown flag |
| 3 | HTTP 4xx (non-429) |
| 4 | HTTP 5xx |
| 5 | Rate limited after retries |

## Secrets

`probe` does **not** load `.env`. Secrets enter via process env (`fnox exec -- probe …`). Config stores env var names only. Artifacts redact `Authorization`, `Cookie`, `Set-Cookie`, and token query params.

## Layout

Spike (cwd / `--dir`):

```text
.probe/
  config.yaml
  session.json
  requests/
  responses/
  notes.md
  log.jsonl
```

Catalog (`$PROBE_CATALOG` or `~/.local/share/probe/catalog`):

```text
$PROBE_CATALOG/<api-name>/
  api.yaml
  fixtures/
  notes.md
```

## Develop

```bash
go test ./...
go build -o /tmp/probe ./cmd/probe
/tmp/probe quickstart --json
```

See [AGENTS.md](AGENTS.md) and [.agents/skills/probe/SKILL.md](.agents/skills/probe/SKILL.md).

## License

MIT
