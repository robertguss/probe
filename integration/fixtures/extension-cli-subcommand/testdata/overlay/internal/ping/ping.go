// Package ping is a responsibility-named domain package for the Foundry
// extension-path fixture. It does not import Cobra (Section 17.4 / REQ-065).
//
// This package is Foundry-owned test overlay content only — it is never
// generated into user projects by the CLI archetype catalog.
package ping

import "fmt"

// Format returns a deterministic one-line status for the given target.
func Format(target string) string {
	if target == "" {
		target = "localhost"
	}
	return fmt.Sprintf("pong %s", target)
}
