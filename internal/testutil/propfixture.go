package testutil

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/spec"
)

// SpecFixture is a property-test input bag that can be turned into a Foundry
// Project Specification TOML. Shared by resolve and plan property suites so
// generators stay aligned (bead go-foundry-cli-n0y.2).
type SpecFixture struct {
	Name        string
	Description string
	Archetype   string
	Visibility  string
	Profiles    []string
}

// String makes testing/quick failure messages readable.
func (f SpecFixture) String() string {
	return fmt.Sprintf("SpecFixture{name=%q archetype=%q visibility=%q profiles=%v}",
		f.Name, f.Archetype, f.Visibility, f.Profiles)
}

// FieldOrderA is a stable top-level TOML field order.
var FieldOrderA = []string{
	"schema", "name", "module", "description",
	"archetype", "destination", "visibility", "profiles",
}

// FieldOrderB is a permuted order used to prove Validate normalizes input shape.
var FieldOrderB = []string{
	"schema", "description", "archetype", "name",
	"module", "profiles", "destination", "visibility",
}

// RandomName returns a lowercase alphabetic project name of length 3–12.
func RandomName(r *rand.Rand) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	n := 3 + r.Intn(10)
	b := make([]byte, n)
	b[0] = letters[r.Intn(len(letters))]
	for i := 1; i < n; i++ {
		b[i] = letters[r.Intn(len(letters))]
	}
	return string(b)
}

// RandomSentence returns a short multi-word description.
func RandomSentence(r *rand.Rand) string {
	const words = "foundry test project demo sample property quick check"
	parts := strings.Fields(words)
	n := 2 + r.Intn(4)
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(parts[r.Intn(len(parts))])
	}
	return b.String()
}

// GenBareSpecFixture builds a valid bare (no profiles) fixture with random
// name/description and random archetype/visibility.
func GenBareSpecFixture(r *rand.Rand) SpecFixture {
	archetypes := []string{"cli", "tui"}
	visibilities := []string{"public", "private"}
	return SpecFixture{
		Name:        RandomName(r),
		Description: RandomSentence(r),
		Archetype:   archetypes[r.Intn(len(archetypes))],
		Visibility:  visibilities[r.Intn(len(visibilities))],
		Profiles:    nil,
	}
}

// GenDistributionFixture builds a public CLI with the distribution profile
// (the only selectable catalog profile).
func GenDistributionFixture(r *rand.Rand) SpecFixture {
	f := GenBareSpecFixture(r)
	f.Archetype = "cli"
	f.Visibility = "public"
	f.Profiles = []string{"distribution"}
	return f
}

// GenProfileSetFixture randomly chooses bare, distribution, or (for negative
// paths) a single unknown profile id. Callers that need only valid Resolve
// inputs should filter Unknown profiles or use GenValidProfileFixture.
func GenProfileSetFixture(r *rand.Rand) SpecFixture {
	f := GenBareSpecFixture(r)
	switch r.Intn(4) {
	case 0:
		f.Profiles = nil
	case 1:
		// distribution requires public visibility.
		f.Visibility = "public"
		if f.Archetype != "cli" && f.Archetype != "tui" {
			f.Archetype = "cli"
		}
		f.Profiles = []string{"distribution"}
	case 2:
		// Invalid: unknown profile id — Validate may accept; Resolve rejects.
		f.Profiles = []string{"not-a-real-profile"}
	default:
		// Invalid combo: private + distribution (profile constraint).
		f.Visibility = "private"
		f.Archetype = "cli"
		f.Profiles = []string{"distribution"}
	}
	return f
}

// GenValidProfileFixture only emits inputs that Resolve accepts: bare or
// public + distribution.
func GenValidProfileFixture(r *rand.Rand) SpecFixture {
	f := GenBareSpecFixture(r)
	if r.Intn(2) == 0 {
		f.Profiles = nil
		return f
	}
	f.Visibility = "public"
	f.Profiles = []string{"distribution"}
	return f
}

// ToTOML renders the fixture as a Project Specification using the given
// top-level field order (for field-order independence properties).
func (f SpecFixture) ToTOML(order []string) (string, error) {
	mod := "github.com/example/" + f.Name
	dest := "./" + f.Name
	profiles := "[]"
	if len(f.Profiles) > 0 {
		parts := make([]string, len(f.Profiles))
		for i, p := range f.Profiles {
			parts[i] = fmt.Sprintf("%q", p)
		}
		profiles = "[" + strings.Join(parts, ", ") + "]"
	}
	fields := map[string]string{
		"schema":      "1",
		"name":        fmt.Sprintf("%q", f.Name),
		"module":      fmt.Sprintf("%q", mod),
		"description": fmt.Sprintf("%q", f.Description),
		"archetype":   fmt.Sprintf("%q", f.Archetype),
		"destination": fmt.Sprintf("%q", dest),
		"visibility":  fmt.Sprintf("%q", f.Visibility),
		"profiles":    profiles,
	}
	if order == nil {
		order = FieldOrderA
	}
	var b strings.Builder
	for _, k := range order {
		v, ok := fields[k]
		if !ok {
			return "", fmt.Errorf("unknown field %q", k)
		}
		b.WriteString(k)
		b.WriteString(" = ")
		b.WriteString(v)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

// Validate decodes and validates the fixture TOML. Returns the validated
// specification or an error (invalid fixtures are expected in some properties).
func (f SpecFixture) Validate(order []string) (*spec.ValidatedSpecification, error) {
	toml, err := f.ToTOML(order)
	if err != nil {
		return nil, err
	}
	raw, err := spec.Decode("foundry.toml", []byte(toml))
	if err != nil {
		return nil, err
	}
	return spec.Validate(raw)
}

// MustValidate is like Validate but fatals the test on error.
func (f SpecFixture) MustValidate(t testingTB, order []string) *spec.ValidatedSpecification {
	t.Helper()
	vs, err := f.Validate(order)
	if err != nil {
		t.Fatalf("SpecFixture.Validate for %s: %v", f, err)
	}
	return vs
}

// testingTB is the subset of testing.T used by helpers (avoids importing
// testing in pure helpers that only need Helper/Fatalf).
type testingTB interface {
	Helper()
	Fatalf(format string, args ...any)
}
