package fixtures_test

import "testing"

// skipIfShort skips multi-minute real-generate fixture cells under -short (ipk.5).
func skipIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("short: multi-minute real-generate fixture cell; use full go test ./integration/fixtures")
	}
}
