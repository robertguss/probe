# Foundry examples

Copy-paste **Project Specifications** for humans and agents. These fixtures are
product surface (not optional docs): use them instead of inventing fields.

**Authority:** [SPEC-FOUNDRY-002](../docs/02-definitive-foundry-specification-revised-fable-5.md)
(Sections 14–15 field contract, Appendix B canonical examples, Appendix D error
ids). Do not invent product behavior outside that document.

See the [User Guide](../docs/user-guide.md) for the full end-user walkthrough.
This page is the quick agent/human workflow for these fixture files.

## Agent workflow (validate → plan dry-run → generate)

1. **Scaffold or copy** a valid example (do not invent fields from memory):
   ```bash
   foundry init --out ./my-cli.toml --name my-cli --module github.com/you/my-cli
   # or: cp examples/minimal-cli.toml ./my-cli.toml
   ```
2. **Edit** only what you need: `name`, `module` (final path segment **must**
   equal `name`), `description`, `destination` (basename **must** equal
   `name`), and optionally `archetype` (`cli` or `tui`).
3. **Never invent profiles.** Leave the default:
   ```toml
   profiles = []
   ```
   The only profile ID currently selectable is `distribution` (requires
   `visibility = "public"` and a `github.com/<owner>/<name>` module — see
   [`appendix-b-public-cli.toml`](appendix-b-public-cli.toml)).
   `configuration` and `local-persistence` are **not** profile
   IDs — they fail `resolve.unknown_profile`. After generate, owners add config
   or persistence by hand using the Section 21 recipes:
   [`docs/recipes/configuration.md`](../docs/recipes/configuration.md) and
   [`docs/recipes/local-persistence.md`](../docs/recipes/local-persistence.md).
4. **Validate** (no project writes):
   ```bash
   foundry validate --spec examples/minimal-cli.toml
   # or: foundry validate --spec path/to/your.toml
   ```
5. **Plan** — authoritative dry-run (text summary; `--output json` for the
   full plan). There is no separate `--dry-run` flag:
   ```bash
   foundry plan --spec examples/minimal-cli.toml
   foundry plan --spec examples/minimal-cli.toml --verbose
   foundry plan --spec examples/minimal-cli.toml --verify strict --output json
   ```
6. **Generate** only after plan looks right (`git init` only — commit yourself):
   ```bash
   foundry generate --spec path/to/your.toml
   # optional: --dest <path> overrides destination (recorded in plan; generate owns project writes)
   ```

Build the binary from the repo root first:
`CGO_ENABLED=0 go build -o foundry ./cmd/foundry`.

## Files

| File                                                         | Role                                     | Expected result                                                                                           |
| ------------------------------------------------------------ | ---------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| [`minimal-cli.toml`](minimal-cli.toml)                       | Fully valid private CLI                  | `validate` + `plan` + `generate` succeed                                                                  |
| [`minimal-tui.toml`](minimal-tui.toml)                       | Fully valid private TUI                  | `validate` + `plan` + `generate` succeed                                                                  |
| [`appendix-b-private-cli.toml`](appendix-b-private-cli.toml) | Appendix B private CLI (canonical)       | `validate` + `plan` + `generate` succeed                                                                  |
| [`appendix-b-private-tui.toml`](appendix-b-private-tui.toml) | Appendix B private TUI (canonical)       | `validate` + `plan` + `generate` succeed                                                                  |
| [`appendix-b-public-cli.toml`](appendix-b-public-cli.toml)   | Appendix B public CLI + `distribution`   | `validate` + `plan` + `generate` succeed (`visibility = "public"`, module is `github.com/<owner>/<name>`) |
| [`invalid-unknown-field.toml`](invalid-unknown-field.toml)   | Negative: unknown key                    | Exit 2, id **`spec.unknown_field`**                                                                       |
| [`invalid-bad-profile.toml`](invalid-bad-profile.toml)       | Negative: non-profile / unimplemented ID | Exit 2, id **`resolve.unknown_profile`**                                                                  |
| [`invalid-bad-name.toml`](invalid-bad-name.toml)             | Negative: name rule                      | Exit 2, id **`spec.invalid_field`**                                                                       |

Full per-id negative matrix for e2e lives under
[`cmd/foundry/testdata/specs/`](../cmd/foundry/testdata/specs/) (exported by
P1.2.c / **go-foundry-cli-d0i**).

### Invalid fixtures — exact error ids

| Fixture                      | Injected fault                   | Expected `domain.reason`  |
| ---------------------------- | -------------------------------- | ------------------------- |
| `invalid-unknown-field.toml` | top-level `author`               | `spec.unknown_field`      |
| `invalid-bad-profile.toml`   | `profiles = ["configuration"]`   | `resolve.unknown_profile` |
| `invalid-bad-name.toml`      | `name = "bad_name"` (underscore) | `spec.invalid_field`      |

Comments at the top of each invalid file restate the expected id for e2e
assertions (bead **go-foundry-cli-5an.1** write-free testscripts consume these
paths).

## Field cheatsheet (do not invent keys)

Required: `schema` (exactly `1`), `name`, `module`, `description`, `archetype`,
`destination`.

Optional: `binary` (default `name`), `visibility` (`private`\|`public`,
default `private`), `profiles` (default `[]`), `[git] init` (default `true`),
`[git] initial_branch` (default `"main"`).

Unknown fields and tables are fatal (`spec.unknown_field`). No secrets, no
env interpolation, no includes.

## Related docs

| Doc                                                                                | Role                                                  |
| ---------------------------------------------------------------------------------- | ----------------------------------------------------- |
| [SPEC-FOUNDRY-002](../docs/02-definitive-foundry-specification-revised-fable-5.md) | Implementation authority                              |
| [docs/user-guide.md](../docs/user-guide.md)                                        | End-user guide: install, commands, fields, errors     |
| [docs/dev/testing.md](../docs/dev/testing.md)                                      | Unit vs e2e, step logger, goldens, build tags         |
| [README.md](../README.md)                                                          | Repo quickstart (validate → plan → generate)          |
| [Configuration recipe](../docs/recipes/configuration.md)                           | §21.1 post-generation guidance — **not** a profile ID |
| [Local persistence recipe](../docs/recipes/local-persistence.md)                   | §21.2 post-generation guidance — **not** a profile ID |

## Consumers

- **go-foundry-cli-5an.1** — write-free e2e testscripts: valid CLI green;
  invalids assert exact error ids; TUI documents plan-or-exact-error.
- **go-foundry-cli-6wj** — validate/plan unit path may also load these fixtures.
