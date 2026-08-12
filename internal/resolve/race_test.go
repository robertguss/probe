package resolve_test

import (
	"sync"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/resolve"
)

// TestResolveConcurrentRace stresses resolve.Resolve with the race detector.
// All goroutines share the same immutable catalog and validated specification;
// no data races or diagnostic mutations should occur, and every call must
// return the same digest-bearing result (bead go-foundry-cli-wet.1.5).
func TestResolveConcurrentRace(t *testing.T) {
	t.Parallel()

	cat := mustLoadCatalog(t)
	vs := minimalCLI(t)
	wantDigest := string(cat.Digest())

	const n = 32
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			rp, err := resolve.Resolve(vs, cat)
			if err != nil {
				t.Errorf("Resolve: %v", err)
				return
			}
			if rp == nil {
				t.Error("Resolve returned nil result")
				return
			}
			if got := string(rp.CatalogDigest()); got != wantDigest {
				t.Errorf("catalog digest mismatch: got %q, want %q", got, wantDigest)
			}
		}()
	}
	wg.Wait()
}
