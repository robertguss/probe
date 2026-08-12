//go:build unix

package toolrun_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

// FuzzEnvKeys never panics and returns sorted unique keys without values
// (ipk.14 — env construction surface without secret leakage).
func FuzzEnvKeys(f *testing.F) {
	f.Add("PATH=/bin")
	f.Add("HOME=/tmp\nPATH=/usr/bin")
	f.Add("")
	f.Add("A=1\nB=2\nA=3")
	f.Fuzz(func(t *testing.T, blob string) {
		env := map[string]string{}
		for _, line := range strings.Split(blob, "\n") {
			if i := strings.IndexByte(line, '='); i > 0 {
				env[line[:i]] = line[i+1:]
			}
		}
		keys := toolrun.EnvKeys(env)
		// Keys must be sorted and match map length.
		if len(keys) != len(env) {
			t.Fatalf("len keys=%d env=%d", len(keys), len(env))
		}
		for i := 1; i < len(keys); i++ {
			if keys[i-1] > keys[i] {
				t.Fatalf("unsorted: %v", keys)
			}
		}
		// Never include values in the key list.
		for _, k := range keys {
			if strings.Contains(k, "=") {
				t.Fatalf("key contains =: %q", k)
			}
			if _, ok := env[k]; !ok {
				t.Fatalf("unknown key %q", k)
			}
		}
	})
}
