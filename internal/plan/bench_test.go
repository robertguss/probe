package plan_test

import (
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/spec"
)

func mustValidSpecForBench(b *testing.B) *spec.ValidatedSpecification {
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

// BenchmarkPipeline measures the full resolve+construct plan pipeline for a
// representative CLI specification.
func BenchmarkPipeline(b *testing.B) {
	cat, err := catalog.Load()
	if err != nil {
		b.Fatalf("catalog.Load: %v", err)
	}
	vs := mustValidSpecForBench(b)
	dest := fixedDestination(vs.Destination(), plan.ObservationAbsent)
	opts := defaultPipelineOpts(dest)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p, err := plan.Pipeline(vs, cat, opts)
		if err != nil {
			b.Fatalf("pipeline: %v", err)
		}
		if p == nil {
			b.Fatal("nil plan")
		}
	}
}
