package dogfood_test

import "testing"

// skipIfShort skips multi-minute dogfood under go test -short (ipk.5).
func skipIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("short: multi-minute dogfood e2e; use go test without -short or make integration")
	}
}
