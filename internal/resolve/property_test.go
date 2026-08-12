package resolve

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestResolveDeterministicProperty checks that Resolve returns byte-identical
// results for two invocations with the same valid input (REQ-211). Includes
// bare and distribution profile fixtures (bead go-foundry-cli-n0y.2).
func TestResolveDeterministicProperty(t *testing.T) {
	log := testutil.New(t)
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}

	cfg := &quick.Config{
		MaxCount: 50,
		Values: func(args []reflect.Value, r *rand.Rand) {
			args[0] = reflect.ValueOf(testutil.GenValidProfileFixture(r))
		},
	}

	prop := func(f testutil.SpecFixture) bool {
		vs, err := f.Validate(testutil.FieldOrderA)
		if err != nil {
			return true
		}
		rp1, err1 := Resolve(vs, cat)
		rp2, err2 := Resolve(vs, cat)
		if (err1 != nil) != (err2 != nil) {
			t.Logf("error mismatch for %s: err1=%v err2=%v", f, err1, err2)
			return false
		}
		if err1 != nil {
			return true
		}
		if !rp1.Equal(rp2) {
			t.Logf("non-equal results for %s", f)
			return false
		}
		return true
	}

	if err := quick.Check(prop, cfg); err != nil {
		t.Fatalf("property failed: %v", err)
	}
	log.Step("resolve_deterministic_property", testutil.OutcomeOK, "ok")
}

// TestResolveCollisionDetectionIdempotentProperty checks that
// detectFileCollisions and detectDependencyConflicts are idempotent and
// deterministic for random contribution slices (REQ-211).
func TestResolveCollisionDetectionIdempotentProperty(t *testing.T) {
	log := testutil.New(t)

	fileCfg := &quick.Config{
		MaxCount: 50,
		Values: func(args []reflect.Value, r *rand.Rand) {
			args[0] = reflect.ValueOf(genFileContributions(r))
		},
	}

	fileProp := func(files []FileContribution) bool {
		err1 := detectFileCollisions(files)
		err2 := detectFileCollisions(files)
		if (err1 == nil) != (err2 == nil) {
			t.Logf("file collision nil mismatch")
			return false
		}
		if err1 != nil && err2 != nil && err1.Error() != err2.Error() {
			t.Logf("file collision message mismatch: %q vs %q", err1.Error(), err2.Error())
			return false
		}
		return true
	}

	if err := quick.Check(fileProp, fileCfg); err != nil {
		t.Fatalf("file collision idempotence failed: %v", err)
	}

	depCfg := &quick.Config{
		MaxCount: 50,
		Values: func(args []reflect.Value, r *rand.Rand) {
			args[0] = reflect.ValueOf(genDependencyContributions(r))
		},
	}

	depProp := func(deps []DependencyContribution) bool {
		err1 := detectDependencyConflicts(deps)
		err2 := detectDependencyConflicts(deps)
		if (err1 == nil) != (err2 == nil) {
			t.Logf("dependency conflict nil mismatch")
			return false
		}
		if err1 != nil && err2 != nil && err1.Error() != err2.Error() {
			t.Logf("dependency conflict message mismatch: %q vs %q", err1.Error(), err2.Error())
			return false
		}
		return true
	}

	if err := quick.Check(depProp, depCfg); err != nil {
		t.Fatalf("dependency conflict idempotence failed: %v", err)
	}

	log.Step("collision_idempotent_property", testutil.OutcomeOK, "ok")
}

// TestResolveFlatProfileOrderIndependentProperty verifies that, when all
// requested profiles are in the implemented set, the order of the requested
// profile slice does not affect the resolved project.
func TestResolveFlatProfileOrderIndependentProperty(t *testing.T) {
	log := testutil.New(t)
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	// Phase 4 synthetic implemented set: only distribution is selectable.
	implemented := []string{"distribution"}

	cfg := &quick.Config{
		MaxCount: 30,
		Values: func(args []reflect.Value, r *rand.Rand) {
			args[0] = reflect.ValueOf(testutil.GenDistributionFixture(r))
		},
	}

	prop := func(f testutil.SpecFixture) bool {
		vs, err := f.Validate(testutil.FieldOrderA)
		if err != nil {
			return true
		}
		rp1, err1 := resolveFlat(vs, cat, implemented)
		if err1 != nil {
			return true
		}
		// Reverse profile order and re-resolve.
		reversed := make([]string, len(f.Profiles))
		for i := range f.Profiles {
			reversed[i] = f.Profiles[len(f.Profiles)-1-i]
		}
		f2 := f
		f2.Profiles = reversed
		vs2, err := f2.Validate(testutil.FieldOrderA)
		if err != nil {
			t.Logf("reverse spec build failed for %s: %v", f, err)
			return false
		}
		rp2, err2 := resolveFlat(vs2, cat, implemented)
		if err2 != nil {
			t.Logf("reverse resolve failed for %s: %v", f, err2)
			return false
		}
		if !rp1.Equal(rp2) {
			t.Logf("profile order affected result for %s", f)
			return false
		}
		return true
	}

	if err := quick.Check(prop, cfg); err != nil {
		t.Fatalf("profile order independence failed: %v", err)
	}
	log.Step("profile_order_independent_property", testutil.OutcomeOK, "ok")
}

// TestResolveInvalidProfileRejectedProperty ensures unknown profiles and
// private+distribution fail closed at Resolve (never succeed).
func TestResolveInvalidProfileRejectedProperty(t *testing.T) {
	log := testutil.New(t)
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}

	cfg := &quick.Config{
		MaxCount: 30,
		Values: func(args []reflect.Value, r *rand.Rand) {
			f := testutil.GenBareSpecFixture(r)
			if r.Intn(2) == 0 {
				f.Profiles = []string{"not-a-real-profile"}
			} else {
				f.Visibility = "private"
				f.Archetype = "cli"
				f.Profiles = []string{"distribution"}
			}
			args[0] = reflect.ValueOf(f)
		},
	}

	prop := func(f testutil.SpecFixture) bool {
		vs, err := f.Validate(testutil.FieldOrderA)
		if err != nil {
			return true
		}
		_, err = Resolve(vs, cat)
		if err == nil {
			t.Logf("expected Resolve failure for %s", f)
			return false
		}
		return true
	}

	if err := quick.Check(prop, cfg); err != nil {
		t.Fatalf("invalid profile property failed: %v", err)
	}
	log.Step("resolve_invalid_profile_rejected_property", testutil.OutcomeOK, "ok")
}

// TestResolveDistributionIncludesProfileFilesProperty asserts distribution
// resolution always contributes profile-owned file paths.
func TestResolveDistributionIncludesProfileFilesProperty(t *testing.T) {
	log := testutil.New(t)
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}

	cfg := &quick.Config{
		MaxCount: 15,
		Values: func(args []reflect.Value, r *rand.Rand) {
			args[0] = reflect.ValueOf(testutil.GenDistributionFixture(r))
		},
	}

	prop := func(f testutil.SpecFixture) bool {
		vs, err := f.Validate(testutil.FieldOrderA)
		if err != nil {
			t.Logf("validate: %v", err)
			return false
		}
		rp, err := Resolve(vs, cat)
		if err != nil {
			t.Logf("Resolve: %v", err)
			return false
		}
		hasProfileOwner := false
		for _, fc := range rp.Files() {
			if fc.Owner == "profile:distribution" || fc.OwnerID == "distribution" ||
				strings.Contains(fc.Source, "profiles/distribution") ||
				strings.Contains(fc.Owner, "distribution") {
				hasProfileOwner = true
				break
			}
		}
		if !hasProfileOwner {
			t.Logf("no distribution-owned files for %s; files=%d", f, len(rp.Files()))
			return false
		}
		return true
	}

	if err := quick.Check(prop, cfg); err != nil {
		t.Fatalf("distribution files property failed: %v", err)
	}
	log.Step("resolve_distribution_files_property", testutil.OutcomeOK, "ok")
}

func genFileContributions(r *rand.Rand) []FileContribution {
	paths := []string{
		"main.go",
		"README.md",
		"go.mod",
		"cmd/foo/main.go",
		"internal/cli/root.go",
		"docs/README.md",
		"go.sum",
		".github/workflows/ci.yml",
	}
	owners := []string{"core", "archetype:cli", "profile:distribution"}
	n := r.Intn(8) + 1
	out := make([]FileContribution, n)
	for i := 0; i < n; i++ {
		path := paths[r.Intn(len(paths))]
		owner := owners[r.Intn(len(owners))]
		out[i] = FileContribution{
			Path:      path,
			Owner:     owner,
			OwnerKind: catalog.KindCore,
			OwnerID:   "core",
			Render:    catalog.RenderStatic,
			Source:    "core/files/" + path,
			Mode:      "0644",
		}
	}
	return out
}

func genDependencyContributions(r *rand.Rand) []DependencyContribution {
	modules := []string{
		"github.com/spf13/cobra",
		"github.com/charmbracelet/bubbletea",
		"github.com/charmbracelet/lipgloss",
		"golang.org/x/mod",
	}
	versions := []string{"v1.0.0", "v1.1.0", "v2.0.0", "v0.5.0"}
	scopes := []catalog.DependencyScope{catalog.ScopeRuntime, catalog.ScopeTest, catalog.ScopeTool}
	owners := []string{"core", "archetype:cli", "profile:distribution"}
	n := r.Intn(6) + 1
	out := make([]DependencyContribution, n)
	for i := 0; i < n; i++ {
		out[i] = DependencyContribution{
			Module:  modules[r.Intn(len(modules))],
			Version: versions[r.Intn(len(versions))],
			Scope:   scopes[r.Intn(len(scopes))],
			Owner:   owners[r.Intn(len(owners))],
		}
	}
	return out
}
