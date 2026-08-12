package resolve_test

import (
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/resolve"
	"github.com/robertguss/go-foundry-cli/internal/spec"
)

func mustValidSpec(b *testing.B) *spec.ValidatedSpecification {
	b.Helper()
	raw, err := spec.Decode("foundry.toml", []byte(`schema = 1
name = "foundry-bench"
module = "github.com/example/foundry-bench"
description = "benchmark fixture"
archetype = "cli"
destination = "./foundry-bench"
visibility = "private"
profiles = []
`))
	if err != nil {
		b.Fatalf("decode: %v", err)
	}
	vs, err := spec.Validate(raw)
	if err != nil {
		b.Fatalf("validate: %v", err)
	}
	return vs
}

// BenchmarkResolve measures resolving a representative CLI specification
// against the embedded catalog.
func BenchmarkResolve(b *testing.B) {
	cat, err := catalog.Load()
	if err != nil {
		b.Fatalf("catalog.Load: %v", err)
	}
	vs := mustValidSpec(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rp, err := resolve.Resolve(vs, cat)
		if err != nil {
			b.Fatalf("resolve: %v", err)
		}
		if rp == nil {
			b.Fatal("nil resolved project")
		}
	}
}
