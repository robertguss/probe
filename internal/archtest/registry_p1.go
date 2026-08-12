package archtest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// P1Domains are Appendix D domain prefixes that apply to Phase 1 write-free
// product paths (validate/plan/catalog/version + pure pipeline packages).
// Domains owned by later phases (fs, tool, verify, git) remain registered for
// completeness (REQ-157 append-only) but are tracked separately.
var P1Domains = []string{
	"spec",
	"resolve",
	"plan",
	"render",
	"report",
	"catalog",
	"usage",
	"internal",
}

// LaterPhaseDomains are registered now but emitted by packages that land after
// Phase 1 (transaction / tools). Listed for super-suite logging only.
var LaterPhaseDomains = []string{
	"fs",
	"tool",
	"verify",
	"git",
}

// RegistryGap is one missing or incomplete registry row.
type RegistryGap struct {
	Kind   string // "missing_id" | "empty_remediation" | "unknown_domain"
	ID     string
	Detail string
}

func (g RegistryGap) String() string {
	return fmt.Sprintf("registry: kind=%s id=%s detail=%s", g.Kind, g.ID, g.Detail)
}

// CheckP1RegistryCompleteness asserts every Appendix D identifier is registered
// with remediation, and every P1 domain has at least one identifier (REQ-157 /
// REQ-188 / P1.8). Missing IDs are listed sorted for agent-legible failure.
func CheckP1RegistryCompleteness() []RegistryGap {
	var gaps []RegistryGap

	// Full inventory completeness vs diagnostic.AllIdentifiers constants.
	want := diagnostic.AllIdentifiers()
	missing := diagnostic.MissingIDs(want)
	if len(missing) > 0 {
		ids := make([]string, len(missing))
		for i, id := range missing {
			ids[i] = string(id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			gaps = append(gaps, RegistryGap{
				Kind:   "missing_id",
				ID:     id,
				Detail: "Appendix D identifier not registered",
			})
		}
	}

	// Every registered entry must have remediation + meaning.
	for _, e := range diagnostic.Entries() {
		if strings.TrimSpace(e.Remediation) == "" {
			gaps = append(gaps, RegistryGap{
				Kind:   "empty_remediation",
				ID:     string(e.ID),
				Detail: "registry entry missing remediation (REQ-159)",
			})
		}
		if strings.TrimSpace(e.Meaning) == "" {
			gaps = append(gaps, RegistryGap{
				Kind:   "empty_meaning",
				ID:     string(e.ID),
				Detail: "registry entry missing meaning",
			})
		}
	}

	// Each P1 domain must own ≥1 identifier.
	byDomain := map[string]int{}
	for _, id := range want {
		dom := domainOf(string(id))
		byDomain[dom]++
	}
	for _, dom := range P1Domains {
		if byDomain[dom] == 0 {
			gaps = append(gaps, RegistryGap{
				Kind:   "unknown_domain",
				ID:     dom + ".*",
				Detail: "P1 domain has zero Appendix D identifiers",
			})
		}
	}

	// Sanity: every known constant is Known().
	for _, id := range want {
		if !diagnostic.Known(id) {
			gaps = append(gaps, RegistryGap{
				Kind:   "missing_id",
				ID:     string(id),
				Detail: "Known() returned false for constant",
			})
		}
	}

	return gaps
}

// P1RegisteredIDs returns sorted Appendix D identifiers whose domain is a P1 domain.
func P1RegisteredIDs() []string {
	var out []string
	p1 := map[string]bool{}
	for _, d := range P1Domains {
		p1[d] = true
	}
	for _, id := range diagnostic.AllIdentifiers() {
		if p1[domainOf(string(id))] {
			out = append(out, string(id))
		}
	}
	sort.Strings(out)
	return out
}

func domainOf(id string) string {
	i := strings.IndexByte(id, '.')
	if i <= 0 {
		return id
	}
	return id[:i]
}
