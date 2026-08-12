# Profile admission process

Maintainer process for admitting a **new Capability Profile** to the Foundry
catalog. This is the operational form of **SPEC-FOUNDRY-002 Section 19.4**
and **REQ-078**.

**Authority:**
[`docs/02-definitive-foundry-specification-revised-fable-5.md`](../02-definitive-foundry-specification-revised-fable-5.md)
(§19.2 classification, §19.4 Profile Admission Test, §20 `distribution`,
§21 recipes, §45 combination matrix, REQ-078).

**Mechanical check:** `internal/archtest` asserts this document exists and
contains the required section headings (see `TestProfileAdmissionDocHeadings`).

**Hard rule:** no profile lands by PR alone. Admission requires an **accepted
specification revision** that records every checklist item below. Vague
proposals that skip evidence, invent application domain code, or expand the
combination matrix without a bound **must be rejected**.

---

## Purpose

Future profile proposals cannot skip the bar:

1. Classification proves the candidate is a **Profile**, not Core, Archetype,
   Recipe, Deferred, or Rejected work (§19.2–19.3).
2. The full **Section 19.4 admission test** is completed with recorded
   evidence.
3. Catalog and schema change only via explicit revision of SPEC-FOUNDRY-002.

Until those hold, the candidate stays out of the generated catalog.

---

## Classification gate (Section 19.2)

Before writing a profile PR, classify the candidate. Only **Profile**
continues to the admission checklist.

| Classification | When it applies | Outcome |
| -------------- | --------------- | ------- |
| **Core** | Near-universal benefit without slowing feedback | Not a profile; Core change proposal |
| **Archetype** | Inseparable from the interaction model (`cli` / `tui`) | Not a profile; archetype change proposal |
| **Profile** | Complete, recurring, **domain-independent** architecture: files, dependencies, docs, and tests that **at least two real projects** would retain unchanged | Proceed to §19.4 checklist |
| **Recipe** | Sound decision pattern that depends on application intent the Project Specification does not carry | Document under §21; **no** profile ID, no generated files |
| Deferred / Rejected | Insufficient demand, domain-specific, or on the Section 58 red-line list | Do not admit; see §22 / §58 |

**Scaffolding is not a profile.** A candidate that can only emit placeholders
the owner must replace is prohibited scaffolding (§19.2 Profile rule).

---

## Section 19.4 admission checklist (copy-paste for PR review)

Copy the block below into the specification-revision PR (or linked design
record). **Every item must be checked with evidence.** Unchecked items = no
admission.

```markdown
### Profile admission record (Section 19.4 / REQ-078)

**Candidate profile ID:** `<id>`
**Spec revision PR / commit:** `<link>`
**Reviewer:** `<name>`
**Date:** `<YYYY-MM-DD>`

#### Classification
- [ ] Classified as **Profile** under Section 19.2 (not Core / Archetype / Recipe / Deferred / Rejected)
- [ ] Not scaffolding: no placeholders the owner must replace at generation time

#### Section 19.4 normative checklist (all required)
- [ ] **1. Complete** — works at generation time with no placeholder the owner must replace
- [ ] **2. Domain-independent** — requires no application intent the Project Specification does not carry
- [ ] **3. Recurring** — at least **two real Generated Projects** independently retained substantially the same files, dependencies, docs, and tests (cite paths / dogfood records; REQ-247)
- [ ] **4. Single owner** — exactly one owner for every file it emits; no shared structured-file semantics beyond typed `go.mod` contributions
- [ ] **5. Flat composition** — constraints are expressible as direct archetype/visibility (and related) predicates with **no** requires/conflicts graph
- [ ] **6. Proportionate burden** — generated test and CI burden is proportionate to projects that select it
- [ ] **7. Bounded matrix** — catalog combination test matrix remains bounded (Section 45); growth uses only archetype-alone, profile-alone, direct-predicate, pairwise, and curated maximal cases — **never** a hypothetical closure graph

#### Evidence (required; no hand-waving)
- [ ] Project A retention evidence (repo path, date, retained file/dep/doc/test list)
- [ ] Project B retention evidence (repo path, date, retained file/dep/doc/test list)
- [ ] Owned-file list and ownership map (no collisions with Core/Archetype/other profiles)
- [ ] Direct predicate(s) written in catalog terms (archetype / visibility / module host as applicable)
- [ ] Combination-matrix impact note (which §45 case classes grow; count bound stated)
- [ ] Explicit confirmation: **no application-domain invention** (no invented config structs, persistence models, business types, or domain workflows)

#### Gate
- [ ] Accepted revision of SPEC-FOUNDRY-002 records this admission (catalog + REQ surface as needed)
- [ ] Implementation PR depends on that accepted revision (not the reverse)

**Decision:** ADMIT / REJECT  
**Rationale:** …
```

---

## Required evidence

Admission is evidence-driven. The following are **mandatory**:

| Evidence | Minimum bar |
| -------- | ----------- |
| **Two real projects** | Two independently generated (or Foundry-dogfood) projects retained substantially the same complete architecture after real use — not synthetic fixtures alone |
| **Complete architecture** | Files, dependencies, docs, **and** tests retained; generation-time complete (no TODO/placeholder owned outputs) |
| **Bounded combination matrix impact** | Written note mapping the candidate onto Section 45 case classes; no exponential/closure-graph growth |
| **No application-domain invention** | Project Specification fields alone drive every emitted byte; domain types/workflows stay out |

Dogfood pattern measurements under **REQ-247** are the preferred source for
the two-project recurrence claim. Point at concrete records under
`docs/evidence/` when they exist.

---

## Recipe holdouts: configuration and local-persistence

These identifiers are **not** schema-1 profiles. They remain **recipes**
until (and unless) a full Section 19.4 admission passes **and** an accepted
specification revision reclassifies them.

| Former / aspirational ID | Current classification | Spec locus | Written recipe | Catalog rule |
| ------------------------ | ---------------------- | ---------- | -------------- | ------------ |
| `configuration` | **Recipe only** | §21.1; REQ-073, REQ-074 | [`docs/recipes/configuration.md`](../recipes/configuration.md) | MUST NOT appear as a profile ID; selection fails `resolve.unknown_profile` |
| `local-persistence` | **Recipe only** | §21.2; REQ-075 | [`docs/recipes/local-persistence.md`](../recipes/local-persistence.md) | MUST NOT appear as a profile ID; selection fails `resolve.unknown_profile` |

### What recipes may do

- Document libraries, precedence, and criteria as **post-generation guidance**
- Appear in written architecture notes labeled as guidance, not generation contracts

### What recipes must not do

- Ship a catalog profile ID or generated owned files
- Add schema fields or plan/profile selection surface
- Bypass Section 19.4 by renaming scaffolding as a “lightweight profile”

Proposals that reintroduce `configuration` or `local-persistence` as profiles
without two-project retention evidence and a full §19.4 record **fail review
immediately**.

---

## Combination matrix bound (Section 45)

Profile admission must not explode CI. Growth strategies allowed when
extending the matrix:

1. **Archetype-alone** cases  
2. **Profile-alone** cases  
3. **Direct-predicate** cases  
4. **Pairwise** cases  
5. **Curated maximal** cases  

Forbidden: hypothetical **requires/conflicts closure graphs**, transitive
capability walks, or “test every subset of N profiles.”

The admission record must state which of (1)–(5) change and by how many cases.

---

## How to propose a profile

1. **Classify** under §19.2 (use the table above). If Recipe → write/update
   §21-style guidance only; stop.
2. **Gather evidence** from two real projects (dogfood / owner repos).
3. **Draft** a SPEC-FOUNDRY-002 revision: profile contract (see §20 shape),
   classification table row, REQ updates if needed, matrix impact.
4. **Fill** the copy-paste **Section 19.4 admission checklist** in that PR.
5. **Review** against rejection criteria below; merge the **spec** only when
   all boxes pass.
6. **Implement** catalog/manifest/tests only after the revision is accepted.

Order is normative for process discipline: **spec admission before code**.

---

## Reviewer rejection criteria

Reject (or convert to recipe/deferred) when **any** of the following hold:

| Signal | Why |
| ------ | --- |
| Vague “would be nice” / single-project anecdote | Fails recurrence (§19.4 item 3) |
| Placeholders, stubs, or owner-must-fill owned files | Fails completeness; scaffolding prohibited |
| Invented application domain (config structs, DB models, business APIs) | Fails domain-independence; Project Specification does not carry that intent |
| Requires/conflicts/capability DAG / helper-binary schema | Flat model only (REQ-077, §19.4 item 5, Section 58) |
| Unbounded matrix growth | §19.4 item 7 / Section 45 |
| Reintroduces `configuration` or `local-persistence` without full admission + revision | REQ-073–075 recipe boundary |
| Implementation PR without accepted spec revision | Violates REQ-078 artifact-revision discipline |
| Section 58 red-line subject | Hard reject; see `docs/evidence/section-58-redlines.md` |

Reviewers paste the checklist into the PR thread and leave items unchecked
until evidence is linked. **Silence is not approval.**

---

## Initial catalog profile (reference only)

The only profile in the initial catalog is **`distribution`** (§20),
implemented post-MVP (Phase 4). It is not a template for skipping admission:
new profiles still run the full §19.4 process. MVP implements **no** optional
profile (`profiles = []`).

---

## Related requirements and beads

| ID / bead | Role |
| --------- | ---- |
| **REQ-078** | Normative: classify per §19.2; admit only via full §19.4 + explicit spec revision |
| **REQ-073**–**REQ-075** | `configuration` / `local-persistence` exclusion-and-recipe boundary |
| **REQ-076** | `distribution` contract (sole initial profile) |
| **REQ-077** | Flat exact-ID selection; no requires/conflicts graph |
| **REQ-011** | Catalog v1.0 contents; recipes vs profiles |
| **REQ-247** | Dogfood pattern measurements feeding recurrence evidence |
| `go-foundry-cli-tpa` | This process document + heading checks |
| `go-foundry-cli-dlv` | Cross-cutting governance parent |
| `go-foundry-cli-6hp` | Section 21 recipe content: [`docs/recipes/configuration.md`](../recipes/configuration.md), [`docs/recipes/local-persistence.md`](../recipes/local-persistence.md) |
| `go-foundry-cli-vu8` / `go-foundry-cli-bjt` | Dogfood / residual-risk paths that feed admission evidence |

---

## Document maintenance

When Section 19.4 checklist items change in the specification, update:

1. The copy-paste block in **Section 19.4 admission checklist** above  
2. `RequiredHeadings` / required body needles in
   `internal/archtest` profile-admission checks  
3. This document’s authority links and recipe holdout table if IDs move  

Do not weaken the bar under delivery pressure. Deletion and recipes remain
preferred over premature profiles (Charter / §19.2 burden of proof).
