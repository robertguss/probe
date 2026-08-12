package e3

import "strings"

// GenerationStep is one verification step from Section 35 / Appendix E.
// Race is intentionally absent from this inventory.
type GenerationStep struct {
	ID     string // Appendix E step id, or in-process check name
	Argv   string // documented argv / description
	When   string // Always | Strict | Startup | …
	IsRace bool   // must be false for all generation-gate steps
	InProc bool   // true for gofmt / mutation-set / final conformance
}

// DefaultGenerationGate is Section 35.1 default verification (plus in-process checks).
// Source of truth for promotion into internal/verify (P2).
func DefaultGenerationGate() []GenerationStep {
	return []GenerationStep{
		{ID: "gofmt-conformance", Argv: "go/format idempotence (in-process)", When: "Always", InProc: true},
		{ID: "tidy-mutation-set", Argv: "validate go.mod/go.sum only + exact pins (in-process)", When: "Always", InProc: true},
		{ID: "go-mod-verify", Argv: "go mod verify", When: "Always"},
		{ID: "go-test", Argv: "go test -count=1 -buildvcs=false -mod=readonly ./...", When: "Always"},
		{ID: "go-vet", Argv: "go vet -buildvcs=false -mod=readonly ./...", When: "Always"},
		{ID: "final-conformance", Argv: "non-.git tree byte compare (in-process)", When: "Always", InProc: true},
	}
}

// StrictGenerationGate is Section 35.2 additions on top of default.
// Still excludes race (race belongs to scheduled strict CI, not generation).
func StrictGenerationGate() []GenerationStep {
	return append(DefaultGenerationGate(),
		GenerationStep{ID: "go-staticcheck", Argv: "go tool staticcheck ./...", When: "Strict"},
		GenerationStep{ID: "go-govulncheck", Argv: "go tool govulncheck ./...", When: "Strict"},
	)
}

// GenerationGateContainsRace reports whether any step is a race job or
// mentions -race in argv. Spec requires this to be false (Section 35.4).
func GenerationGateContainsRace(steps []GenerationStep) bool {
	for _, s := range steps {
		if s.IsRace {
			return true
		}
		low := strings.ToLower(s.Argv)
		if strings.Contains(low, "-race") || strings.Contains(low, "race detector") {
			return true
		}
		if s.ID == "go-test-race" || s.ID == "go-race" {
			return true
		}
	}
	return false
}
