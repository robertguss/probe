package resolve

import (
	"fmt"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// Predicate names used in resolve.profile_constraint messages (agents must
// not need source to identify the failed check).
const (
	PredicateCompatibleArchetype = "compatible_archetypes"
	PredicateRequiresVisibility  = "requires_visibility"
	PredicateModuleHost          = "module_host"
)

// CheckProfilePredicates evaluates direct archetype / visibility / module-host
// predicates for one profile unit against the selected project fields
// (Section 23.2, REQ-077). First failure wins with resolve.profile_constraint
// naming the failed predicate.
//
// Flat only: no graph, no capability mapping. Pure: no I/O.
//
// Module-host is profile-specific. Schema-1 only defines it for distribution
// (Section 20): module must be exactly github.com/<owner>/<name>.
func CheckProfilePredicates(m *catalog.Manifest, archetype, visibility, modulePath string) error {
	if m == nil {
		return diagnostic.New(
			diagnostic.IDResolveProfileConstraint,
			"profile manifest is nil",
			diagnostic.Location{},
		)
	}
	if m.Kind != catalog.KindProfile {
		return profileConstraintError(m.ID, PredicateCompatibleArchetype,
			fmt.Sprintf("unit %q is kind %q, not profile", m.ID, m.Kind))
	}

	// 1. Compatible archetype (required for every profile unit).
	if !containsString(m.CompatibleArchetypes, archetype) {
		return profileConstraintError(m.ID, PredicateCompatibleArchetype,
			fmt.Sprintf(
				"profile %q is incompatible with archetype %q; compatible archetypes: %s",
				m.ID, archetype, formatIDSet(sortedCopy(m.CompatibleArchetypes)),
			))
	}

	// 2. Optional direct visibility predicate.
	if m.RequiresVisibility != "" && m.RequiresVisibility != visibility {
		return profileConstraintError(m.ID, PredicateRequiresVisibility,
			fmt.Sprintf(
				"profile %q requires visibility %q (project visibility is %q)",
				m.ID, m.RequiresVisibility, visibility,
			))
	}

	// 3. Module-host predicate for distribution (Section 20).
	// Other profiles have no module-host predicate in schema 1.
	if m.ID == "distribution" {
		if !IsGitHubHostedModule(modulePath) {
			return profileConstraintError(m.ID, PredicateModuleHost,
				fmt.Sprintf(
					"profile %q requires a GitHub-hosted module path of the form github.com/<owner>/<name> (got %q)",
					m.ID, modulePath,
				))
		}
	}

	return nil
}

// IsGitHubHostedModule reports whether modulePath is exactly
// github.com/<owner>/<name> — two non-empty path segments after the host
// (Section 20 direct predicate). No deeper nesting, no other hosts.
func IsGitHubHostedModule(modulePath string) bool {
	if modulePath == "" {
		return false
	}
	// Exact shape: github.com/OWNER/NAME
	const prefix = "github.com/"
	if !strings.HasPrefix(modulePath, prefix) {
		return false
	}
	rest := modulePath[len(prefix):]
	if rest == "" || strings.HasPrefix(rest, "/") {
		return false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return false
	}
	owner, name := parts[0], parts[1]
	if owner == "" || name == "" {
		return false
	}
	// Reject empty-looking or path-traversal-ish segments.
	if owner == "." || owner == ".." || name == "." || name == ".." {
		return false
	}
	if strings.Contains(owner, "\\") || strings.Contains(name, "\\") {
		return false
	}
	return true
}

// profileConstraintError builds resolve.profile_constraint naming the failed
// predicate (Section 23.2).
func profileConstraintError(profileID, predicate, message string) *diagnostic.FoundryError {
	msg := message
	if !strings.Contains(msg, predicate) {
		msg = fmt.Sprintf("%s [predicate=%s]", message, predicate)
	}
	fe := diagnostic.New(diagnostic.IDResolveProfileConstraint, msg, diagnostic.Location{})
	return fe.WithRemediation(fmt.Sprintf(
		"Profile %q failed direct predicate %q. "+
			"Remove the profile from the specification profiles array, or adjust archetype/visibility/module "+
			"so the predicate passes. Schema-1 profiles have only direct predicates (no graph/capability solver). "+
			"MVP: use profiles = [].",
		profileID, predicate,
	))
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
