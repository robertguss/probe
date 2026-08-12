package archtest

// Profile admission process document (Section 19.4 / REQ-078).
// Authored by go-foundry-cli-tpa; keeps vague profile proposals process-blocked.
const DocProfileAdmissionPath = "docs/dev/profile-admission.md"

// RequiredProfileAdmissionHeadings are markdown ATX headings that MUST appear
// in docs/dev/profile-admission.md. Stable identifiers for the process; do not
// rename lightly (heading parser test depends on exact strings).
var RequiredProfileAdmissionHeadings = []string{
	"# Profile admission process",
	"## Purpose",
	"## Classification gate (Section 19.2)",
	"## Section 19.4 admission checklist (copy-paste for PR review)",
	"## Required evidence",
	"## Recipe holdouts: configuration and local-persistence",
	"## Combination matrix bound (Section 45)",
	"## How to propose a profile",
	"## Reviewer rejection criteria",
	"## Related requirements and beads",
}

// RequiredProfileAdmissionNeedles are body substrings that must appear so the
// doc cannot be gutted while leaving headings. Anchors for §19.4 / REQ-078
// and the recipe holdouts named in the bead acceptance criteria.
var RequiredProfileAdmissionNeedles = []string{
	"Section 19.4",
	"REQ-078",
	"configuration",
	"local-persistence",
	"two real",
	"domain-independent",
	"combination matrix",
	"SPEC-FOUNDRY-002",
	"resolve.unknown_profile",
	// Checklist item anchors from the normative seven.
	"**1. Complete**",
	"**2. Domain-independent**",
	"**3. Recurring**",
	"**4. Single owner**",
	"**5. Flat composition**",
	"**6. Proportionate burden**",
	"**7. Bounded matrix**",
}
