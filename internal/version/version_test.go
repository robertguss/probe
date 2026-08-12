package version_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/version"
)

func TestReadFieldsPresent(t *testing.T) {
	log := testutil.New(t)
	log.Phase("read")
	info := version.Read()
	log.Assert("version_nonempty", info.Version != "", true, info.Version)
	log.Assert("go_prefix", strings.HasPrefix(info.Go, "go"), true, info.Go)
	// Unstamped builds use DefaultVersion.
	if info.Version == version.DefaultVersion {
		log.Step("default_version", testutil.OutcomeOK, version.DefaultVersion)
	}
	log.PhaseEnd("read", testutil.OutcomeOK)

	log.Phase("with_catalog")
	with := info.WithCatalog("abc")
	log.Assert("digest", with.CatalogDigest == "abc", "abc", with.CatalogDigest)
	log.Assert("orig_empty", info.CatalogDigest == "", "", info.CatalogDigest)
	log.PhaseEnd("with_catalog", testutil.OutcomeOK)
}
