package catalog_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestCatalogStringAndNilAccessors(t *testing.T) {
	log := testutil.New(t)
	log.Phase("catalog_residual")
	c, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	s := c.String()
	log.Assert("string", strings.Contains(s, "catalog"), true, s)
	log.Assert("digest", c.Digest() != "", true, c.Digest())
	log.Assert("files", c.FileCount() > 0, true, c.FileCount())
	log.Assert("lock", c.Lock() != nil, true, c.Lock() != nil)
	log.Assert("list", len(c.List()) >= 4, true, len(c.List()))
	// nil catalog accessors
	var n *catalog.Catalog
	log.Assert("nil_digest", n.Digest() == "", true, n.Digest())
	log.Assert("nil_count", n.FileCount() == 0, 0, n.FileCount())
	log.Assert("nil_paths", n.Paths() == nil, true, n.Paths())
	log.Assert("nil_roots", n.Roots() == nil, true, n.Roots())
	log.Assert("nil_lock", n.Lock() == nil, true, n.Lock() != nil)
	log.Assert("nil_manifests", n.Manifests() == nil, true, n.Manifests())
	_, ok := n.Manifest("x")
	log.Assert("nil_manifest", !ok, true, ok)
	_, err = n.Read("x")
	log.Assert("nil_read", err != nil, true, err != nil)
	log.Assert("nil_list", n.List() == nil, true, n.List())
	// DigestFS empty map path via empty FS if available
	if d, err := catalog.DigestFS(fstest.MapFS{}); err == nil {
		log.Assert("empty_digest", d != "", true, d)
	}
	log.PhaseEnd("catalog_residual", testutil.OutcomeOK)
}
