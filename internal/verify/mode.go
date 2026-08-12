package verify

import (
	"fmt"
	"os"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/plan"
)

// Mode selects the verification step list (Section 35 / REQ-033 / REQ-152).
// Exactly two values are admitted: default and strict. There is no "none",
// no skip, and no environment variable may weaken or bypass verification.
type Mode string

const (
	// ModeDefault is gofmt + module-mutation + verify + test + vet + final-conformance.
	ModeDefault Mode = "default"
	// ModeStrict adds staticcheck and govulncheck (Section 35.2).
	ModeStrict Mode = "strict"
)

// AdmittedModes is the exhaustive allowed set (REQ-152 / Section 58).
var AdmittedModes = []Mode{ModeDefault, ModeStrict}

// ForbiddenModeTokens are rejected spellings that must never be accepted
// (REQ-152 / RL-58-VERIFY-BYPASS). Compared case-insensitively after trim.
var ForbiddenModeTokens = []string{
	"none",
	"off",
	"skip",
	"false",
	"0",
	"disable",
	"disabled",
}

// EnvWeakenKeys are environment variable names that MUST NOT influence
// verification strength (REQ-152). ParseMode and AssertNoEnvWeaken refuse them
// when set to a truthy weaken value; product code never reads them to skip.
var EnvWeakenKeys = []string{
	"FOUNDRY_SKIP_VERIFY",
	"FOUNDRY_VERIFY_NONE",
	"FOUNDRY_NO_VERIFY",
	"FOUNDRY_VERIFY_BYPASS",
	"SKIP_VERIFY",
	"VERIFY_NONE",
}

// Valid reports whether m is default or strict.
func (m Mode) Valid() bool {
	switch m {
	case ModeDefault, ModeStrict:
		return true
	default:
		return false
	}
}

// Normalize maps empty to ModeDefault; other values unchanged.
func (m Mode) Normalize() Mode {
	if m == "" {
		return ModeDefault
	}
	return m
}

// String returns the mode text.
func (m Mode) String() string { return string(m) }

// PlanMode converts to plan.VerifyMode for plan construction interop.
func (m Mode) PlanMode() plan.VerifyMode {
	switch m.Normalize() {
	case ModeStrict:
		return plan.VerifyStrict
	default:
		return plan.VerifyDefault
	}
}

// FromPlan converts plan.VerifyMode into verify.Mode.
func FromPlan(m plan.VerifyMode) Mode {
	switch m.Normalize() {
	case plan.VerifyStrict:
		return ModeStrict
	default:
		return ModeDefault
	}
}

// ParseMode accepts exactly "default" or "strict" (case-insensitive, trimmed).
// Empty becomes ModeDefault. Forbidden tokens and any other value fail closed
// with usage.invalid-class messaging via verify.failed for package-local use
// (CLI surface uses usage.invalid via flags).
func ParseMode(raw string) (Mode, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return ModeDefault, nil
	}
	for _, bad := range ForbiddenModeTokens {
		if v == bad {
			return "", diagnostic.Newf(
				diagnostic.IDVerifyFailed,
				diagnostic.Location{},
				"verify mode %q is forbidden; only default and strict are admitted (REQ-152)",
				raw,
			).WithRemediation(
				"Pass --verify default or --verify strict. There is no bypass, skip, or none mode.",
			)
		}
	}
	switch Mode(v) {
	case ModeDefault, ModeStrict:
		return Mode(v), nil
	default:
		return "", diagnostic.Newf(
			diagnostic.IDVerifyFailed,
			diagnostic.Location{},
			"invalid verify mode %q; want default or strict",
			raw,
		).WithRemediation(
			"Pass --verify default or --verify strict. No other values are accepted (REQ-152).",
		)
	}
}

// AssertNoEnvWeaken fails closed when a known weaken env key is set to a
// truthy value. Verification strength is flag-only (REQ-152).
//
// Empty / unset / explicit "0"/"false"/"no" are ignored (not weaken).
// Any other non-empty value is treated as an attempted bypass.
func AssertNoEnvWeaken() error {
	for _, key := range EnvWeakenKeys {
		val, ok := os.LookupEnv(key)
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		if val == "" {
			continue
		}
		low := strings.ToLower(val)
		if low == "0" || low == "false" || low == "no" || low == "off" {
			continue
		}
		return diagnostic.Newf(
			diagnostic.IDVerifyFailed,
			diagnostic.Location{},
			"environment variable %s=%q must not weaken verification (REQ-152)",
			key, val,
		).WithRemediation(
			fmt.Sprintf(
				"Unset %s. Verification mode is controlled only by --verify default|strict; "+
					"no environment variable may skip, weaken, or bypass the generation gate.",
				key,
			),
		)
	}
	return nil
}
