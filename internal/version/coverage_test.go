package version_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/version"
)

func TestReadShapeAndWithCatalog(t *testing.T) {
	t.Parallel()
	log := testutil.New(t)
	log.Phase("read_shape")

	info := version.Read()
	log.Assert("version_set", info.Version != "", true, info.Version)
	log.Assert("go_matches_runtime", info.Go == runtime.Version(), true, info.Go)
	// Commit may be empty (no vcs settings) or revision[/dirty].
	if info.Commit != "" {
		log.Step("commit", testutil.OutcomeOK, info.Commit)
		// dirty suffix only when modified=true was present
		if strings.HasSuffix(info.Commit, "-dirty") {
			log.Assert("dirty_has_rev", len(info.Commit) > len("-dirty"), true, info.Commit)
		}
	} else {
		log.Step("commit_empty", testutil.OutcomeOK, "no vcs.revision in build info")
	}
	log.Assert("catalog_empty", info.CatalogDigest == "", true, info.CatalogDigest)

	// DefaultVersion constant is the unstamped fallback.
	log.Assert("default_const", version.DefaultVersion != "", true, version.DefaultVersion)

	// WithCatalog is a value copy.
	a := info.WithCatalog("deadbeef")
	b := info.WithCatalog("cafebabe")
	log.Assert("a_digest", a.CatalogDigest == "deadbeef", true, a.CatalogDigest)
	log.Assert("b_digest", b.CatalogDigest == "cafebabe", true, b.CatalogDigest)
	log.Assert("orig_untouched", info.CatalogDigest == "", true, info.CatalogDigest)
	log.Assert("version_preserved", a.Version == info.Version && a.Go == info.Go, true, a.Version)

	// Empty digest still sets field.
	c := info.WithCatalog("")
	log.Assert("empty_digest", c.CatalogDigest == "", true, c.CatalogDigest)

	// Multiple Read calls are stable for Go version.
	info2 := version.Read()
	log.Assert("stable_go", info2.Go == info.Go, true, info2.Go)
	log.Assert("stable_version", info2.Version == info.Version, true, info2.Version)

	log.PhaseEnd("read_shape", testutil.OutcomeOK)
}
