package spec_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/spec"
)

// BenchmarkDecodeValidate measures the hot path of decoding and validating a
// representative project specification.
func BenchmarkDecodeValidate(b *testing.B) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		b.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "examples", "minimal-cli.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		raw, err := spec.Decode("foundry.toml", data)
		if err != nil {
			b.Fatalf("decode: %v", err)
		}
		_, err = spec.Validate(raw)
		if err != nil {
			b.Fatalf("validate: %v", err)
		}
	}
}
