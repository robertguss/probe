//go:build foundrydev

package catalog_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestLoadDirMatchesEmbed loads the repo catalog/ tree via foundrydev LoadDir
// and asserts digest + inventory parity with the embedded catalog (REQ-091).
func TestLoadDirMatchesEmbed(t *testing.T) {
	log := testutil.New(t)
	log.Phase("load_dir")

	root := repoRoot(t)
	catDir := filepath.Join(root, "catalog")
	log.NotePath(catDir)

	disk, err := catalog.LoadDir(catDir)
	if err != nil {
		log.Fail("load_dir", err.Error())
	}
	embed, err := catalog.Load()
	if err != nil {
		log.Fail("load_embed", err.Error())
	}

	log.Step("file_count", testutil.OutcomeOK,
		"disk="+itoa(disk.FileCount())+" embed="+itoa(embed.FileCount()))
	log.Step("digest", testutil.OutcomeOK, "sha256="+string(disk.Digest()))
	log.Assert("digest_match", disk.Digest() == embed.Digest(),
		string(embed.Digest()), string(disk.Digest()))
	log.Assert("paths_match", equalStrings(disk.Paths(), embed.Paths()), true, false)
	log.Assert("roots_match", equalStrings(disk.Roots(), embed.Roots()), true, false)
	log.PhaseEnd("load_dir", testutil.OutcomeOK)
}

// TestLoadDirFailClosed covers empty/missing/non-dir paths (REQ-091 fail closed).
func TestLoadDirFailClosed(t *testing.T) {
	log := testutil.New(t)

	cases := []struct {
		name string
		dir  func(t *testing.T) string
	}{
		{"empty", func(t *testing.T) string { return "" }},
		{"missing", func(t *testing.T) string {
			return filepath.Join(t.TempDir(), "no-such-catalog")
		}},
		{"file_not_dir", func(t *testing.T) string {
			p := filepath.Join(t.TempDir(), "not-a-dir")
			if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			return p
		}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			log.Phase(tc.name)
			dir := tc.dir(t)
			log.NotePath(dir)
			_, err := catalog.LoadDir(dir)
			log.Assert("err", err != nil, true, err != nil)
			fe, ok := diagnostic.AsFoundryError(err)
			log.Assert("is_foundry", ok, true, ok)
			if ok {
				log.Assert("id", fe.ID() == diagnostic.IDCatalogInvalid,
					string(diagnostic.IDCatalogInvalid), string(fe.ID()))
				log.NoteID(string(fe.ID()))
			}
			log.PhaseEnd(tc.name, testutil.OutcomeOK)
		})
	}
}

// TestLoadDirRejectsInvalidTree ensures full validation on disk load.
func TestLoadDirRejectsInvalidTree(t *testing.T) {
	log := testutil.New(t)
	log.Phase("invalid_tree")

	dir := t.TempDir()
	// Minimal broken tree: missing required roots/files.
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := catalog.LoadDir(dir)
	log.Assert("err", err != nil, true, err != nil)
	fe, ok := diagnostic.AsFoundryError(err)
	log.Assert("is_foundry", ok, true, ok)
	if ok {
		log.Assert("id", fe.ID() == diagnostic.IDCatalogInvalid,
			string(diagnostic.IDCatalogInvalid), string(fe.ID()))
	}
	if err != nil && !strings.Contains(err.Error(), "missing") {
		// Message should mention missing required content when possible.
		log.Step("message", testutil.OutcomeOK, err.Error())
	}
	log.PhaseEnd("invalid_tree", testutil.OutcomeOK)
}
