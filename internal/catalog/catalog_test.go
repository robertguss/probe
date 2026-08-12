package catalog_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestEmbedLoadsExpectedRoots verifies the embedded catalog contains Section
// 24.2 roots, required files, and a golden file inventory (count + paths).
func TestEmbedLoadsExpectedRoots(t *testing.T) {
	log := testutil.New(t)
	log.Phase("load")
	log.Fixture("catalog", "embedded")

	c, err := catalog.Load()
	if err != nil {
		log.Fail("load", err.Error())
	}
	log.Step("file_count", testutil.OutcomeOK, "count="+itoa(c.FileCount()))
	log.Step("digest", testutil.OutcomeOK, "sha256="+string(c.Digest()))
	log.Step("root_list", testutil.OutcomeOK, "roots="+strings.Join(c.Roots(), ","))
	log.PhaseEnd("load", testutil.OutcomeOK)

	log.Phase("assert_roots")
	roots := c.Roots()
	for _, want := range catalog.RequiredRoots {
		log.Assert("root_"+want, contains(roots, want), true, contains(roots, want))
	}
	log.Assert("root_versions.toml", contains(roots, "versions.toml"), true, contains(roots, "versions.toml"))
	// testdata must not be embedded (Section 24.2).
	log.Assert("no_testdata_root", !contains(roots, "testdata"), true, !contains(roots, "testdata"))
	log.PhaseEnd("assert_roots", testutil.OutcomeOK)

	log.Phase("assert_required_files")
	for _, req := range catalog.RequiredFiles {
		_, err := c.Read(req)
		log.Assert("required_"+req, err == nil, true, err == nil)
	}
	log.PhaseEnd("assert_required_files", testutil.OutcomeOK)

	log.Phase("assert_inventory")
	paths := c.Paths()
	// Golden inventory: committed sorted path list under testdata/.
	wantPaths := loadInventory(t)
	log.Assert("file_count", c.FileCount() == len(wantPaths), len(wantPaths), c.FileCount())
	log.Assert("paths_equal", equalStrings(paths, wantPaths), true, false)
	if !equalStrings(paths, wantPaths) {
		t.Logf("got paths:\n%s", strings.Join(paths, "\n"))
		t.Logf("want paths:\n%s", strings.Join(wantPaths, "\n"))
	}
	// No configuration / local-persistence profiles.
	forbidden := 0
	for _, p := range paths {
		if strings.HasPrefix(p, "profiles/configuration") || strings.HasPrefix(p, "profiles/local-persistence") {
			forbidden++
			log.Fail("forbidden_recipe_profile", p)
		}
	}
	log.Assert("no_recipe_profiles", forbidden == 0, 0, forbidden)
	log.PhaseEnd("assert_inventory", testutil.OutcomeOK)
}

// TestCatalogDigestStable asserts digest is non-empty and stable across loads
// (also run under -count=2 externally).
func TestCatalogDigestStable(t *testing.T) {
	log := testutil.New(t)
	log.Phase("digest")

	c1, err := catalog.Load()
	if err != nil {
		log.Fail("load1", err.Error())
	}
	c2, err := catalog.Load()
	if err != nil {
		log.Fail("load2", err.Error())
	}
	d1, d2 := c1.Digest(), c2.Digest()
	log.Step("digest", testutil.OutcomeOK, "sha256="+string(d1))
	log.Assert("non_empty", len(d1) == 64, 64, len(d1))
	log.Assert("stable_across_load", d1 == d2, string(d1), string(d2))
	// Default() must match Load().
	def, err := catalog.Default()
	if err != nil {
		log.Fail("default", err.Error())
	}
	log.Assert("default_matches", def.Digest() == d1, string(d1), string(def.Digest()))
	log.PhaseEnd("digest", testutil.OutcomeOK)
}

// TestDigestMutationChangesHash flips one byte in an in-memory copy of every
// embedded file (one at a time) and asserts the digest always changes.
func TestDigestMutationChangesHash(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	c, err := catalog.Load()
	if err != nil {
		log.Fail("load", err.Error())
	}
	orig := c.Digest()
	files := c.Files()
	paths := c.Paths()
	log.Step("file_count", testutil.OutcomeOK, "count="+itoa(len(paths)))
	log.Step("digest", testutil.OutcomeOK, "sha256="+string(orig))
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("mutate")
	mutated := 0
	for _, p := range paths {
		// Deep copy map for this mutation.
		clone := cloneMap(files)
		b := clone[p]
		if len(b) == 0 {
			// Empty file: append a byte.
			clone[p] = []byte{0x01}
		} else {
			nb := make([]byte, len(b))
			copy(nb, b)
			nb[0] ^= 0x01
			clone[p] = nb
		}
		got := catalog.DigestMap(clone)
		if got == orig {
			log.Fail("mutation_unchanged", "path="+p)
		}
		mutated++
	}
	log.Assert("all_files_mutated", mutated == len(paths), len(paths), mutated)
	log.Step("mutations", testutil.OutcomeOK, "n="+itoa(mutated))
	log.PhaseEnd("mutate", testutil.OutcomeOK)
}

// TestMissingRequiredRootFails covers empty/missing required roots and files.
func TestMissingRequiredRootFails(t *testing.T) {
	log := testutil.New(t)
	log.Phase("missing_file")

	base := mustEmbeddedMap(t)
	// Drop core/manifest.toml.
	delete(base, "core/manifest.toml")
	fsys := toMapFS(base)
	err := catalog.ValidateLayout(fsys)
	log.Assert("err_non_nil", err != nil, true, err != nil)
	assertCatalogInvalid(t, log, err, "core/manifest.toml")
	log.PhaseEnd("missing_file", testutil.OutcomeOK)

	log.Phase("missing_root")
	base2 := mustEmbeddedMap(t)
	for p := range base2 {
		if strings.HasPrefix(p, "schemas/") || p == "schemas" {
			delete(base2, p)
		}
	}
	err = catalog.ValidateLayout(toMapFS(base2))
	log.Assert("err_non_nil", err != nil, true, err != nil)
	assertCatalogInvalid(t, log, err, "schemas")
	log.PhaseEnd("missing_root", testutil.OutcomeOK)

	log.Phase("empty_catalog")
	err = catalog.ValidateLayout(toMapFS(map[string][]byte{}))
	log.Assert("err_non_nil", err != nil, true, err != nil)
	assertCatalogInvalid(t, log, err, "")
	log.PhaseEnd("empty_catalog", testutil.OutcomeOK)
}

// TestForbiddenRecipeProfilesFails rejects configuration / local-persistence
// trees and dual lock files in layout validation.
func TestForbiddenRecipeProfilesFails(t *testing.T) {
	log := testutil.New(t)
	cases := []struct {
		name string
		path string
	}{
		{"configuration", "profiles/configuration/manifest.toml"},
		{"local_persistence", "profiles/local-persistence/manifest.toml"},
		{"dual_lock", "versions.lock"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			log := testutil.New(t)
			log.Phase("forbidden_" + tc.name)
			base := mustEmbeddedMap(t)
			base[tc.path] = []byte("forbidden\n")
			err := catalog.ValidateLayout(toMapFS(base))
			log.Assert("err_non_nil", err != nil, true, err != nil)
			assertCatalogInvalid(t, log, err, tc.path)
			log.PhaseEnd("forbidden_"+tc.name, testutil.OutcomeOK)
		})
	}
	_ = log
}

// TestLockParseAndConsumption covers versions.toml parse + exactly-once
// consumption helper. Manifest pin matching is covered in manifest_test.go.
func TestLockParseAndConsumption(t *testing.T) {
	log := testutil.New(t)
	log.Phase("parse_lock")
	c, err := catalog.Load()
	if err != nil {
		log.Fail("load", err.Error())
	}
	lock := c.Lock()
	log.Assert("schema_1", lock.Schema == 1, 1, lock.Schema)
	log.Assert("toolchain_go_set", lock.Toolchain.Go != "", true, lock.Toolchain.Go != "")
	ids := lock.EntryIDs()
	log.Step("entry_count", testutil.OutcomeOK, "n="+itoa(len(ids)))
	log.Assert("has_toolchain", contains(ids, "toolchain.go"), true, false)
	log.Assert("has_modules", len(lock.Modules) > 0, true, len(lock.Modules) > 0)
	log.Assert("has_tools", len(lock.Tools) > 0, true, len(lock.Tools) > 0)
	log.Assert("has_actions", len(lock.Actions) > 0, true, len(lock.Actions) > 0)
	// Action SHAs are full 40-char.
	for _, a := range lock.Actions {
		log.Assert("sha40_"+a.ID, len(a.SHA) == 40, 40, len(a.SHA))
	}
	log.PhaseEnd("parse_lock", testutil.OutcomeOK)

	log.Phase("consumption_unused")
	// Empty consumption → all unused.
	err = catalog.ValidateConsumption(lock, map[string]int{})
	log.Assert("unused_fails", err != nil, true, err != nil)
	assertCatalogInvalid(t, log, err, "versions.toml")
	if err != nil && !strings.Contains(err.Error(), "unused") {
		log.Fail("unused_message", err.Error())
	}
	log.PhaseEnd("consumption_unused", testutil.OutcomeOK)

	log.Phase("consumption_missing_pin")
	// Exactly-once for all known, plus unknown pin.
	okMap := map[string]int{}
	for _, id := range ids {
		okMap[id] = 1
	}
	err = catalog.ValidateConsumption(lock, okMap)
	log.Assert("exact_once_ok", err == nil, true, err == nil)

	bad := cloneIntMap(okMap)
	bad["module:does-not-exist"] = 1
	err = catalog.ValidateConsumption(lock, bad)
	log.Assert("unknown_pin_fails", err != nil, true, err != nil)
	assertCatalogInvalid(t, log, err, "versions.toml")
	if err != nil && !strings.Contains(err.Error(), "missing from lock") {
		log.Fail("missing_message", err.Error())
	}
	log.PhaseEnd("consumption_missing_pin", testutil.OutcomeOK)

	log.Phase("consumption_double")
	dbl := cloneIntMap(okMap)
	dbl[ids[0]] = 2
	err = catalog.ValidateConsumption(lock, dbl)
	log.Assert("double_fails", err != nil, true, err != nil)
	assertCatalogInvalid(t, log, err, "versions.toml")
	log.PhaseEnd("consumption_double", testutil.OutcomeOK)
}

// TestEmbedMatchesRepoTree compares embedded bytes to the Git-visible catalog/
// tree (excluding testdata and package .go sources) — CI tree-vs-embed hook.
func TestEmbedMatchesRepoTree(t *testing.T) {
	log := testutil.New(t)
	log.Phase("compare")
	c, err := catalog.Load()
	if err != nil {
		log.Fail("load", err.Error())
	}
	root := repoRoot(t)
	catDir := filepath.Join(root, "catalog")
	log.NotePath(catDir)

	disk := map[string][]byte{}
	for _, top := range catalog.EmbeddedTopLevel {
		p := filepath.Join(catDir, top)
		st, err := os.Stat(p)
		if err != nil {
			log.Fail("stat_"+top, err.Error())
		}
		if !st.IsDir() {
			b, err := os.ReadFile(p)
			if err != nil {
				log.Fail("read_"+top, err.Error())
			}
			disk[filepath.ToSlash(top)] = b
			continue
		}
		err = filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(catDir, path)
			if err != nil {
				return err
			}
			relSlash := filepath.ToSlash(rel)
			// Skip only package-level catalog/*.go sources (fs.go etc.), not
			// catalog unit content that happens to be .go (e.g. static
			// completion.go under archetypes/*/files/).
			if strings.Count(relSlash, "/") == 0 && strings.HasSuffix(relSlash, ".go") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			disk[relSlash] = b
			return nil
		})
		if err != nil {
			log.Fail("walk_"+top, err.Error())
		}
	}

	embedPaths := c.Paths()
	diskPaths := sortedKeys(disk)
	log.Step("file_count", testutil.OutcomeOK, "embed="+itoa(len(embedPaths))+" disk="+itoa(len(diskPaths)))
	log.Assert("path_sets_equal", equalStrings(embedPaths, diskPaths), true, false)
	if !equalStrings(embedPaths, diskPaths) {
		t.Logf("embed only: %v", diffLeft(embedPaths, diskPaths))
		t.Logf("disk only: %v", diffLeft(diskPaths, embedPaths))
	}
	for _, p := range embedPaths {
		got, _ := c.Read(p)
		want := disk[p]
		log.Assert("bytes_"+p, bytes.Equal(got, want), true, false)
	}
	log.Step("digest", testutil.OutcomeOK, "sha256="+string(c.Digest()))
	log.PhaseEnd("compare", testutil.OutcomeOK)
}

// TestLoadFSRejectsBadLock ensures invalid versions.toml fails LoadFS.
func TestLoadFSRejectsBadLock(t *testing.T) {
	log := testutil.New(t)
	log.Phase("bad_schema")
	base := mustEmbeddedMap(t)
	base["versions.toml"] = []byte("schema = 99\n[toolchain]\ngo = \"1.26.5\"\n")
	_, err := catalog.LoadFS(toMapFS(base))
	log.Assert("err", err != nil, true, err != nil)
	assertCatalogInvalid(t, log, err, "versions.toml")
	log.PhaseEnd("bad_schema", testutil.OutcomeOK)
}

// --- helpers ---

func assertCatalogInvalid(t *testing.T, log *testutil.Logger, err error, locHint string) {
	t.Helper()
	fe, ok := diagnostic.AsFoundryError(err)
	log.Assert("is_foundry_error", ok, true, ok)
	if !ok {
		return
	}
	log.Assert("id_catalog_invalid", fe.ID() == diagnostic.IDCatalogInvalid,
		string(diagnostic.IDCatalogInvalid), string(fe.ID()))
	if locHint != "" {
		loc := fe.Location().String()
		log.Assert("loc_mentions_hint", strings.Contains(loc, locHint) || strings.Contains(err.Error(), locHint),
			locHint, loc)
	}
	if !errors.Is(err, fe) && fe == nil {
		// keep staticcheck happy
	}
}

func mustEmbeddedMap(t *testing.T) map[string][]byte {
	t.Helper()
	c, err := catalog.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return c.Files()
}

func toMapFS(m map[string][]byte) fstest.MapFS {
	out := fstest.MapFS{}
	for p, b := range m {
		out[p] = &fstest.MapFile{Data: b}
	}
	return out
}

func loadInventory(t *testing.T) []string {
	t.Helper()
	p := filepath.Join(testdataDir(t), "inventory.txt")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read inventory: %v (generate via listing Load().Paths())", err)
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func testdataDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "testdata")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/catalog → repo root
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func cloneMap(m map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(m))
	for k, v := range m {
		cp := make([]byte, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}

func cloneIntMap(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func sortedKeys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func diffLeft(a, b []string) []string {
	set := map[string]bool{}
	for _, x := range b {
		set[x] = true
	}
	var out []string
	for _, x := range a {
		if !set[x] {
			out = append(out, x)
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var neg bool
	if n < 0 {
		neg = true
		n = -n
	}
	var b [32]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
