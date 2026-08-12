# Catalog Manifest Field Contract

Human-readable contract for unit manifests under `catalog/`
(SPEC-FOUNDRY-002 Section 24.3, Appendix G, REQ-092).

## Required top-level fields

| Field         | Type   | Notes                                              |
| ------------- | ------ | -------------------------------------------------- |
| `schema`      | int    | Must be `1`                                        |
| `id`          | string | Stable unit ID (`core`, `cli`, `tui`, `distribution`, …) |
| `kind`        | string | `core` \| `archetype` \| `profile`                 |
| `description` | string | Non-empty human description                        |

## Optional top-level fields (profiles)

| Field                    | Type     | Notes                                      |
| ------------------------ | -------- | ------------------------------------------ |
| `compatible_archetypes`  | []string | Flat list of archetype IDs                 |
| `requires_visibility`    | string   | e.g. `"public"`                            |

## `[[files]]` entries

| Field    | Type   | Notes                                         |
| -------- | ------ | --------------------------------------------- |
| `path`   | string | Relative output path (safe; no `..`/absolute) |
| `render` | string | `static` \| `template` only                   |
| `source` | string | Path relative to the unit directory           |
| `mode`   | string | `0644` in v1.0                                |

## `[[dependencies]]` entries

| Field    | Type   | Notes                              |
| -------- | ------ | ---------------------------------- |
| `module` | string | Exact module path                  |
| `version`| string | Exact semver (lock-pinned)         |
| `scope`  | string | `runtime` \| `test` \| `tool`      |

## Forbidden fields (hard reject)

`requires`, `conflicts`, `capabilities`, `provides`, `helper_binary`, and any
render mode other than `static` / `template`. Profile DAGs and capability
graphs do not exist (FND-009, REQ-092).

`go.mod` is not listed as an owned file; it is produced by the typed generator
from dependency contributions (Section 26.4).
