package spec_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/spec"
)

// FuzzDecode ensures the decoder never panics on arbitrary input.
// Time is bounded per input via the testing harness (no unbounded loops
// in Decode itself). Seed corpus includes inline bytes plus testdata/fuzz/*.toml.
func FuzzDecode(f *testing.F) {
	// Seed corpus: valid, boundary-ish, and hostile snippets.
	seeds := [][]byte{
		[]byte(""),
		[]byte("schema = 1\n"),
		[]byte("schema = 1\nname = \"x\"\n"),
		[]byte("a = 1\na = 2\n"),
		[]byte("schema = \"1\"\n"),
		[]byte{0xEF, 0xBB, 0xBF, 's', '=', '1', '\n'},
		[]byte("schema = \"\xff\"\n"),
		[]byte("schema = 1\x00\n"),
		[]byte("[git]\ninit = true\n[git]\n"),
		[]byte("@@@"),
		[]byte("description = \"${HOME} $PATH include other.toml\"\n"),
		bytesOfSize(1024),
		bytesOfSize(spec.MaxSpecBytes),
		bytesOfSize(spec.MaxSpecBytes + 1),
	}
	for _, s := range seeds {
		f.Add(s)
	}
	addFuzzSeedFiles(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Bound wall time per case so a pathological hang fails the fuzz.
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic: %v", r)
				}
			}()
			_, _ = spec.Decode("fuzz.toml", data)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Decode exceeded 2s timeout")
		}
	})
}

// FuzzValidate exercises the full Decode→Validate pipeline without panicking.
// Validation errors are expected for most inputs; only panics fail.
// Named without a FuzzDecode* prefix so -fuzz=FuzzDecode stays unambiguous.
func FuzzValidate(f *testing.F) {
	seeds := [][]byte{
		[]byte(`
schema = 1
name = "demo-cli"
module = "github.com/example/demo-cli"
description = "fuzz seed valid"
archetype = "cli"
destination = "./demo-cli"
profiles = []
`),
		[]byte(`
schema = 1
name = "BAD"
module = "github.com/example/BAD"
description = ""
archetype = "web"
destination = "../x"
profiles = ["a", "a"]
`),
		[]byte("schema = 2\n"),
		[]byte("@@@"),
		[]byte{0xEF, 0xBB, 0xBF},
	}
	for _, s := range seeds {
		f.Add(s)
	}
	addFuzzSeedFiles(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Cap extremely large inputs so Validate path stays responsive under fuzz.
		if len(data) > spec.MaxSpecBytes+4096 {
			data = data[:spec.MaxSpecBytes+4096]
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic: %v", r)
				}
			}()
			raw, err := spec.Decode("fuzz.toml", data)
			if err != nil || raw == nil {
				return
			}
			_, _ = spec.Validate(raw)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Decode+Validate exceeded 2s timeout")
		}
	})
}

// addFuzzSeedFiles loads optional checked-in corpus files under testdata/fuzz/.
func addFuzzSeedFiles(f *testing.F) {
	f.Helper()
	dir := filepath.Join("testdata", "fuzz")
	entries, err := os.ReadDir(dir)
	if err != nil {
		// Corpus directory is optional at first clone; inline seeds still apply.
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		f.Add(b)
	}
}

func bytesOfSize(n int) []byte {
	if n <= 0 {
		return nil
	}
	b := make([]byte, n)
	// Valid-ish TOML padding: comments only.
	copy(b, []byte("schema = 1\n"))
	for i := 11; i < n; i++ {
		b[i] = '#'
	}
	b[n-1] = '\n'
	return b
}
