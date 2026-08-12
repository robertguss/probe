package resolve_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/spec"
)

// mustLoadCatalog loads the embedded production catalog or fails the test.
func mustLoadCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	return c
}

// mustValidate decodes+validates a TOML specification body.
func mustValidate(t *testing.T, toml string) *spec.ValidatedSpecification {
	t.Helper()
	raw, err := spec.Decode("foundry.toml", []byte(toml))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	vs, err := spec.Validate(raw)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return vs
}

// minimalCLI returns a validated MVP CLI specification (profiles = []).
func minimalCLI(t *testing.T) *spec.ValidatedSpecification {
	t.Helper()
	return mustValidate(t, `
schema = 1
name = "demo-cli"
module = "github.com/example/demo-cli"
description = "A valid demo CLI project"
archetype = "cli"
destination = "./demo-cli"
`)
}

// minimalTUI returns a validated MVP TUI specification (profiles = []).
func minimalTUI(t *testing.T) *spec.ValidatedSpecification {
	t.Helper()
	return mustValidate(t, `
schema = 1
name = "demo-tui"
module = "github.com/example/demo-tui"
description = "A valid demo TUI project"
archetype = "tui"
destination = "./demo-tui"
`)
}

// specWith builds a validated spec from overrides (profiles as TOML array text).
func specWith(t *testing.T, overrides map[string]string) *spec.ValidatedSpecification {
	t.Helper()
	fields := map[string]string{
		"schema":      "1",
		"name":        "demo-cli",
		"module":      "github.com/example/demo-cli",
		"description": "A valid demo CLI project",
		"archetype":   "cli",
		"destination": "./demo-cli",
		"profiles":    "[]",
	}
	for k, v := range overrides {
		if v == "" && k != "description" {
			delete(fields, k)
			continue
		}
		fields[k] = v
	}
	// Keep destination basename == name when name is overridden.
	if name, ok := overrides["name"]; ok && overrides["destination"] == "" {
		fields["destination"] = "./" + name
	}
	if name, ok := overrides["name"]; ok && overrides["module"] == "" {
		// Preserve github host; replace final segment.
		fields["module"] = "github.com/example/" + name
	}
	order := []string{
		"schema", "name", "module", "description", "archetype",
		"destination", "binary", "visibility", "profiles",
	}
	var b strings.Builder
	for _, k := range order {
		v, ok := fields[k]
		if !ok {
			continue
		}
		switch k {
		case "schema":
			b.WriteString("schema = ")
			b.WriteString(v)
			b.WriteByte('\n')
		case "profiles":
			b.WriteString("profiles = ")
			b.WriteString(v)
			b.WriteByte('\n')
		default:
			b.WriteString(k)
			b.WriteString(" = ")
			b.WriteString(strconvQuote(v))
			b.WriteByte('\n')
		}
	}
	return mustValidate(t, b.String())
}

func strconvQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString("\\n")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
