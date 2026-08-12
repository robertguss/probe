package catalog_test

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestFoundrydevLoadDirIsBuildTagged audits REQ-091: the OS-path catalog loader
// source is gated behind //go:build foundrydev so release binaries (default
// build) do not compile it in. This is the CI/release-surface hook companion
// to TestEmbedMatchesRepoTree.
func TestFoundrydevLoadDirIsBuildTagged(t *testing.T) {
	log := testutil.New(t)
	log.Phase("audit_source_tag")

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "catalog", "load_dir.go")
	log.NotePath(path)

	f, err := os.Open(path)
	if err != nil {
		log.Fail("open", err.Error())
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	var firstNonEmpty string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		firstNonEmpty = line
		break
	}
	if err := sc.Err(); err != nil {
		log.Fail("scan", err.Error())
	}
	log.Step("build_line", testutil.OutcomeOK, firstNonEmpty)
	// Accept either //go:build foundrydev or // +build foundrydev (legacy).
	ok := firstNonEmpty == "//go:build foundrydev" ||
		firstNonEmpty == "// +build foundrydev"
	log.Assert("tagged_foundrydev", ok, "//go:build foundrydev", firstNonEmpty)

	// Companion test file must also be tagged so default `go test` stays clean.
	testPath := filepath.Join(root, "internal", "catalog", "load_dir_test.go")
	log.NotePath(testPath)
	tf, err := os.Open(testPath)
	if err != nil {
		log.Fail("open_test", err.Error())
	}
	defer tf.Close()
	tsc := bufio.NewScanner(tf)
	var testFirst string
	for tsc.Scan() {
		line := strings.TrimSpace(tsc.Text())
		if line == "" {
			continue
		}
		testFirst = line
		break
	}
	log.Step("test_build_line", testutil.OutcomeOK, testFirst)
	testOK := testFirst == "//go:build foundrydev" || testFirst == "// +build foundrydev"
	log.Assert("test_tagged_foundrydev", testOK, "//go:build foundrydev", testFirst)

	log.PhaseEnd("audit_source_tag", testutil.OutcomeOK)
}
