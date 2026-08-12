# Write-free e2e specification fixtures

Exported by **go-foundry-cli-d0i** (P1.2.c) for consumption by **go-foundry-cli-5an.1**
(P1.8.a write-free testscripts) and later e2e beads.

Source of truth for package-level goldens remains `internal/spec/testdata/`.
These copies are process-boundary fixtures for the real `foundry` binary.

## Layout

| Path | Role |
| ---- | ---- |
| `valid/` | Specs that must pass `foundry validate` (and plan when catalog allows) |
| `invalid/` | Specs that must fail with an exact Appendix D `spec.*` (or resolve) id |

## Valid

| File | Notes |
| ---- | ----- |
| `appendix-b-private-cli.toml` | Appendix B private CLI |
| `appendix-b-private-tui.toml` | Appendix B private TUI |
| `appendix-b-public-cli.toml` | Appendix B public + `profiles = ["distribution"]` (spec-valid; resolve may reject until profile ships) |
| `minimal-cli.toml` | Product example (also under `examples/`) |
| `minimal-tui.toml` | Product example (also under `examples/`) |

## Invalid (`spec.*` domain)

| File | Expected id |
| ---- | ----------- |
| `parse_error.toml` | `spec.parse_error` |
| `unsupported_schema.toml` | `spec.unsupported_schema` |
| `unknown_field.toml` | `spec.unknown_field` |
| `duplicate_key.toml` | `spec.duplicate_key` |
| `invalid_field.toml` | `spec.invalid_field` |
| `duplicate_profile.toml` | `spec.duplicate_profile` |
| `nested_unknown_table.toml` | `spec.unknown_field` |
| `hostile_env_destination.toml` | `spec.invalid_field` |
| `examples-invalid-*.toml` | Mirrors of `examples/invalid-*.toml` |

Programmatic-only ids (`spec.too_large`, `spec.invalid_encoding`) are covered in
`internal/spec` unit tests; e2e may synthesize them with a generator if needed.

## Also see

- `examples/` — human/agent product surface (same Appendix B + minimal set)
- `docs/dev/testing.md` — unit vs e2e conventions
- SPEC-FOUNDRY-002 Appendix B / Appendix D
