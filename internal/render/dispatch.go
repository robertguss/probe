package render

import (
	"fmt"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// Job is one render request dispatched by Mechanism (Section 26.1 / REQ-096).
//
// Exactly one payload field must be non-nil and match Mechanism:
//   - MechanismStatic   → Static
//   - MechanismTemplate → Template
//   - MechanismGomod    → Gomod
//
// CatalogReader is required for static and template; gomod is typed-only.
type Job struct {
	// Mechanism selects the render strategy (static | template | gomod).
	Mechanism Mechanism
	// Static is required when Mechanism == MechanismStatic.
	Static *StaticJob
	// Template is required when Mechanism == MechanismTemplate.
	Template *TemplateJob
	// Gomod is required when Mechanism == MechanismGomod.
	Gomod *GomodInput
}

// Render dispatches one job by mechanism into pure buffers (or via Writer).
//
// Phase 1 purity: never opens the real destination filesystem. When w is nil,
// content is retained only in the returned Inventory for plan digests.
// Unknown mechanisms and payload mismatches fail closed (render.failed).
func Render(cat CatalogReader, job Job, w Writer) (*Inventory, error) {
	if !job.Mechanism.Valid() {
		return nil, diagnostic.Newf(
			diagnostic.IDRenderFailed,
			diagnostic.PathLocation(string(job.Mechanism)),
			"unknown render mechanism %q (allowed: static, template, gomod)",
			job.Mechanism,
		)
	}
	switch job.Mechanism {
	case MechanismStatic:
		if job.Static == nil {
			return nil, dispatchPayloadError(MechanismStatic, "Static job is required")
		}
		if job.Template != nil || job.Gomod != nil {
			return nil, dispatchPayloadError(MechanismStatic, "only Static payload may be set")
		}
		return RenderStatic(cat, *job.Static, w)
	case MechanismTemplate:
		if job.Template == nil {
			return nil, dispatchPayloadError(MechanismTemplate, "Template job is required")
		}
		if job.Static != nil || job.Gomod != nil {
			return nil, dispatchPayloadError(MechanismTemplate, "only Template payload may be set")
		}
		return RenderTemplate(cat, *job.Template, w)
	case MechanismGomod:
		if job.Gomod == nil {
			return nil, dispatchPayloadError(MechanismGomod, "Gomod input is required")
		}
		if job.Static != nil || job.Template != nil {
			return nil, dispatchPayloadError(MechanismGomod, "only Gomod payload may be set")
		}
		return RenderGomod(*job.Gomod, w)
	default:
		// Unreachable when Valid() is exhaustive; keep fail-closed.
		return nil, diagnostic.Newf(
			diagnostic.IDRenderFailed,
			diagnostic.PathLocation(string(job.Mechanism)),
			"unhandled render mechanism %q",
			job.Mechanism,
		)
	}
}

// RenderAll dispatches jobs in order into one sorted Inventory (REQ-096).
// Fail-closed: the first error aborts with no partial inventory returned.
// Duplicate output paths across jobs fail closed (render.failed).
// When w is non-nil, each successful file is written before the next job.
//
// Inventory contract invariants are enforced by TestRenderInventoryContract
// and TestGeneratedProjectRequiredFilesContract in contract_test.go.
func RenderAll(cat CatalogReader, jobs []Job, w Writer) (*Inventory, error) {
	entries := make([]Entry, 0, len(jobs))
	content := make(map[string][]byte, len(jobs))
	seen := make(map[string]struct{}, len(jobs))

	for i, job := range jobs {
		inv, err := Render(cat, job, nil)
		if err != nil {
			return nil, err
		}
		for _, e := range inv.Entries() {
			if _, dup := seen[e.Path]; dup {
				return nil, diagnostic.Newf(
					diagnostic.IDRenderFailed,
					diagnostic.PathLocation(e.Path),
					"render job %d (%s) duplicates output path %q",
					i, job.Mechanism, e.Path,
				)
			}
			data, ok := inv.Content(e.Path)
			if !ok {
				return nil, diagnostic.Newf(
					diagnostic.IDRenderFailed,
					diagnostic.PathLocation(e.Path),
					"render job %d missing content for path %q",
					i, e.Path,
				)
			}
			seen[e.Path] = struct{}{}
			if w != nil {
				if err := w.WriteFile(e.Path, e.Mode, data); err != nil {
					return nil, wrapWriteError(e.Path, err)
				}
			}
			entries = append(entries, e)
			content[e.Path] = data
		}
	}
	return newInventory(entries, content), nil
}

// Merge combines inventories into one sorted Inventory.
// Fail-closed on path collisions. Nil inventories are skipped.
// Content bytes are defensive-copied; inputs are not mutated.
func Merge(invs ...*Inventory) (*Inventory, error) {
	var entries []Entry
	content := make(map[string][]byte)
	seen := make(map[string]struct{})

	for i, inv := range invs {
		if inv == nil {
			continue
		}
		for _, e := range inv.Entries() {
			if _, dup := seen[e.Path]; dup {
				return nil, diagnostic.Newf(
					diagnostic.IDRenderFailed,
					diagnostic.PathLocation(e.Path),
					"merge inventory %d duplicates output path %q",
					i, e.Path,
				)
			}
			data, ok := inv.Content(e.Path)
			if !ok {
				return nil, diagnostic.Newf(
					diagnostic.IDRenderFailed,
					diagnostic.PathLocation(e.Path),
					"merge inventory %d missing content for path %q",
					i, e.Path,
				)
			}
			seen[e.Path] = struct{}{}
			entries = append(entries, e)
			content[e.Path] = data
		}
	}
	return newInventory(entries, content), nil
}

func dispatchPayloadError(m Mechanism, msg string) *diagnostic.FoundryError {
	return diagnostic.New(
		diagnostic.IDRenderFailed,
		fmt.Sprintf("mechanism %s: %s", m, msg),
		diagnostic.PathLocation(string(m)),
	).WithRemediation(
		"Dispatch Job payload must match Mechanism: static→Static, template→Template, gomod→Gomod. " +
			"Exactly three mechanisms exist (REQ-096 / Section 26.1).",
	)
}
