package archtest_test

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/archtest"
	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/spec"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestP1QualitySuperSuite is the P1.8 (4hi) entry that ties architecture,
// write-free purity, golden CI discipline, and P1 registry completeness into
// one agent-legible suite (REQ-031, REQ-187, REQ-219, REQ-157/188).
//
// Failure output prints violating import edges, purity evidence, and missing
// registry ids — no debugger required.
func TestP1QualitySuperSuite(t *testing.T) {
	log := testutil.New(t)
	root := repoRoot(t)
	log.Fixture("repo_root", filepath.Base(root))

	// --- Architecture + red-lines (import edges) ---
	log.Phase("architecture")
	violations, err := archtest.Check(root)
	if err != nil {
		log.Fail("arch_check", err.Error())
	}
	for _, v := range violations {
		log.Step("arch_violation", testutil.OutcomeFail, v.String())
		t.Errorf("%s", v)
	}
	log.Assert("arch_clean", len(violations) == 0, 0, len(violations))
	if len(violations) > 0 {
		// Print import edges / redline ids for agents.
		var edges []string
		for _, v := range violations {
			edges = append(edges, v.Rule+":"+v.Package+":"+v.Detail)
		}
		sort.Strings(edges)
		log.Step("violating_edges", testutil.OutcomeFail, strings.Join(edges, " | "))
	}
	log.PhaseEnd("architecture", testutil.OutcomeOK)

	// --- Write-free purity (static; complements 5an.1 e2e) ---
	log.Phase("purity")
	present := archtest.PurityPackagePresence(root)
	log.Inputs(map[string]string{
		"write_free_packages": strings.Join(present, ","),
		"expected_count":      itoa(len(archtest.WriteFreePackages)),
	})
	log.Assert("all_p1_packages_present", len(present) == len(archtest.WriteFreePackages),
		len(archtest.WriteFreePackages), len(present))
	pvs, err := archtest.CheckPurity(root)
	if err != nil {
		log.Fail("purity_check", err.Error())
	}
	for _, v := range pvs {
		log.Step("purity_violation", testutil.OutcomeFail, v.String())
		t.Errorf("%s", v)
	}
	log.Assert("purity_clean", len(pvs) == 0, 0, len(pvs))
	log.PhaseEnd("purity", testutil.OutcomeOK)

	// --- Golden CI discipline (REQ-219) ---
	log.Phase("golden_discipline")
	log.Inputs(map[string]string{
		"bulk_threshold": itoa(archtest.GoldenBulkThreshold),
		"update_env":     archtest.EnvUpdateGolden,
		"ci_env":         archtest.EnvCI,
	})
	gvs, err := archtest.CheckGoldenDiscipline(root)
	if err != nil {
		log.Fail("golden_check", err.Error())
	}
	for _, v := range gvs {
		log.Step("golden_violation", testutil.OutcomeFail, v.String())
		t.Errorf("%s", v)
	}
	log.Assert("golden_clean", len(gvs) == 0, 0, len(gvs))
	// Runtime guard still refuses under CI.
	t.Setenv(archtest.EnvUpdateGolden, "1")
	t.Setenv(archtest.EnvCI, "true")
	if err := testutil.UpdateAllowed(); err == nil {
		log.Fail("update_allowed_under_ci", "expected refuse when CI=true and UPDATE_GOLDEN=1")
	} else {
		log.Step("update_refused_under_ci", testutil.OutcomeOK, err.Error())
	}
	t.Setenv(archtest.EnvUpdateGolden, "")
	t.Setenv(archtest.EnvCI, "")
	log.PhaseEnd("golden_discipline", testutil.OutcomeOK)

	// --- Registry completeness (P1 domains + full Appendix D) ---
	log.Phase("registry")
	p1IDs := archtest.P1RegisteredIDs()
	log.Inputs(map[string]string{
		"p1_id_count":   itoa(len(p1IDs)),
		"total_ids":     itoa(len(diagnostic.AllIdentifiers())),
		"p1_domains":    strings.Join(archtest.P1Domains, ","),
		"later_domains": strings.Join(archtest.LaterPhaseDomains, ","),
	})
	gaps := archtest.CheckP1RegistryCompleteness()
	if len(gaps) > 0 {
		var missing []string
		for _, g := range gaps {
			log.Step("registry_gap", testutil.OutcomeFail, g.String())
			t.Errorf("%s", g)
			if g.Kind == "missing_id" {
				missing = append(missing, g.ID)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			log.Step("missing_ids", testutil.OutcomeFail, strings.Join(missing, ", "))
		}
	}
	log.Assert("registry_complete", len(gaps) == 0, 0, len(gaps))
	log.Assert("p1_ids_nonempty", len(p1IDs) > 0, ">0", len(p1IDs))
	log.PhaseEnd("registry", testutil.OutcomeOK)
}

// TestWriteFreePurityStatic is a focused purity scan with step-log evidence
// (REQ-031 complement to e2e strace).
func TestWriteFreePurityStatic(t *testing.T) {
	log := testutil.New(t)
	root := repoRoot(t)
	log.Phase("scan")
	pvs, err := archtest.CheckPurity(root)
	if err != nil {
		log.Fail("check", err.Error())
	}
	for _, v := range pvs {
		log.Step("hit", testutil.OutcomeFail, v.String())
		t.Errorf("%s", v)
	}
	log.Assert("clean", len(pvs) == 0, 0, len(pvs))
	log.PhaseEnd("scan", testutil.OutcomeOK)
}

// TestGoldenDisciplineCIPolicy locks REQ-219 workflow + docs + runtime guard.
func TestGoldenDisciplineCIPolicy(t *testing.T) {
	log := testutil.New(t)
	root := repoRoot(t)
	log.Phase("policy")
	gvs, err := archtest.CheckGoldenDiscipline(root)
	if err != nil {
		log.Fail("check", err.Error())
	}
	for _, v := range gvs {
		log.Step("hit", testutil.OutcomeFail, v.String())
		t.Errorf("%s", v)
	}
	log.Assert("clean", len(gvs) == 0, 0, len(gvs))
	log.Assert("bulk_threshold_positive", archtest.GoldenBulkThreshold > 0,
		">0", archtest.GoldenBulkThreshold)
	log.PhaseEnd("policy", testutil.OutcomeOK)
}

// TestP1RegistryDomainsComplete lists P1 domain coverage on failure.
func TestP1RegistryDomainsComplete(t *testing.T) {
	log := testutil.New(t)
	log.Phase("domains")
	gaps := archtest.CheckP1RegistryCompleteness()
	p1 := archtest.P1RegisteredIDs()
	log.Step("p1_ids", testutil.OutcomeOK, strings.Join(p1, ","))
	for _, g := range gaps {
		log.Step("gap", testutil.OutcomeFail, g.String())
		t.Errorf("%s", g)
	}
	log.Assert("no_gaps", len(gaps) == 0, 0, len(gaps))
	log.PhaseEnd("domains", testutil.OutcomeOK)
}

// TestPlanJSONDeterminismProperty proves plan JSON bytes are stable under
// repeated pure construction (unit+property acceptance; -count=2 friendly).
func TestPlanJSONDeterminismProperty(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	root := repoRoot(t)
	// Prefer examples fixture when present; otherwise a minimal inline spec.
	specPath := filepath.Join(root, "examples", "minimal-cli.toml")
	data, err := os.ReadFile(specPath)
	if err != nil {
		data = []byte(`
schema = 1
name = "det-cli"
module = "example.com/det-cli"
description = "determinism property fixture"
archetype = "cli"
destination = "./det-cli"
profiles = []
`)
		log.Fixture("spec", "inline_minimal")
	} else {
		log.Fixture("spec", "examples/minimal-cli.toml")
	}
	raw, err := spec.Decode("determinism.toml", data)
	if err != nil {
		log.Fail("decode", err.Error())
	}
	vs, err := spec.Validate(raw)
	if err != nil {
		log.Fail("validate", err.Error())
	}
	cat, err := catalog.Load()
	if err != nil {
		log.Fail("catalog", err.Error())
	}
	dest := plan.DestinationInfo{
		Path:        "/home/foundry/projects/minimal-cli",
		Parent:      "/home/foundry/projects",
		Basename:    "minimal-cli",
		Observation: plan.ObservationAbsent,
	}
	opts := plan.PipelineOptions{
		FoundryVersion: plan.DefaultFoundryVersion,
		FoundryCommit:  "testcommit000000000000000000000000000001",
		GoVersion:      "go1.26.5",
		SpecSource:     plan.SpecSourcePath,
		SpecPath:       "determinism.toml",
		Destination:    dest,
		Verify:         plan.VerifyDefault,
		GoBinary:       "/usr/local/go/bin/go",
		GitBinary:      "/usr/bin/git",
		GitTemplateDir: "/tmp/foundry-git-template-scratch",
		Host: plan.HostEnv{
			PATH:       "/usr/bin",
			HOME:       "/home/foundry",
			TMPDIR:     "/tmp",
			GOMODCACHE: "/home/foundry/go/pkg/mod",
			GOCACHE:    "/home/foundry/.cache/go-build",
			GOPATH:     "/home/foundry/go",
			GOPROXY:    "https://proxy.golang.org,direct",
			GOSUMDB:    "sum.golang.org",
		},
	}
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("construct_twice")
	var digests []string
	var bodies [][]byte
	for i := 0; i < 2; i++ {
		p, err := plan.Pipeline(vs, cat, opts)
		if err != nil {
			log.Fail("pipeline_"+itoa(i), err.Error())
		}
		body, err := p.MarshalJSON()
		if err != nil {
			log.Fail("marshal_"+itoa(i), err.Error())
		}
		bodies = append(bodies, body)
		digests = append(digests, p.PlanSHA256())
		log.Step("run_"+itoa(i), testutil.OutcomeOK,
			"plan_sha256="+p.PlanSHA256()+" bytes="+itoa(len(body)))
	}
	log.Assert("sha_equal", digests[0] == digests[1], digests[0], digests[1])
	log.Assert("bytes_equal", bytes.Equal(bodies[0], bodies[1]), true, false)
	log.NoteID(digests[0])
	log.PhaseEnd("construct_twice", testutil.OutcomeOK)
}

// TestArchitectureConsumesPurityAndGolden wires the expanded Check surface
// (redlines already covered by TestSection58ConsumedByArchitectureCheck).
func TestArchitectureCheckIncludesPurityAndGolden(t *testing.T) {
	log := testutil.New(t)
	root := repoRoot(t)
	log.Phase("check")
	// Check() includes redlines + layer rules; purity and golden are separate
	// entry points also invoked from the super-suite. Assert both pure APIs
	// return clean on the real tree so CI cannot skip them.
	pvs, err := archtest.CheckPurity(root)
	if err != nil {
		log.Fail("purity", err.Error())
	}
	gvs, err := archtest.CheckGoldenDiscipline(root)
	if err != nil {
		log.Fail("golden", err.Error())
	}
	avs, err := archtest.Check(root)
	if err != nil {
		log.Fail("arch", err.Error())
	}
	log.Assert("purity_0", len(pvs) == 0, 0, len(pvs))
	log.Assert("golden_0", len(gvs) == 0, 0, len(gvs))
	log.Assert("arch_0", len(avs) == 0, 0, len(avs))
	log.PhaseEnd("check", testutil.OutcomeOK)
}

// TestPurityDetectsForbiddenImport is a negative test using a temp tree.
func TestPurityDetectsForbiddenImport(t *testing.T) {
	log := testutil.New(t)
	log.Phase("negative")
	tmp := t.TempDir()
	// Minimal module root + a fake pure package that imports net.
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"),
		[]byte("module github.com/robertguss/go-foundry-cli\n\ngo 1.26.5\n"), 0o644); err != nil {
		log.Fail("gomod", err.Error())
	}
	pkgDir := filepath.Join(tmp, "internal", "plan")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		log.Fail("mkdir", err.Error())
	}
	bad := `package plan
import "net"
func Dial() { _ = net.IPv4zero }
`
	if err := os.WriteFile(filepath.Join(pkgDir, "bad.go"), []byte(bad), 0o644); err != nil {
		log.Fail("write", err.Error())
	}
	pvs, err := archtest.CheckPurity(tmp)
	if err != nil {
		log.Fail("check", err.Error())
	}
	log.Assert("detected", len(pvs) > 0, ">0", len(pvs))
	found := false
	for _, v := range pvs {
		log.Step("hit", testutil.OutcomeOK, v.String())
		if strings.Contains(v.Evidence, "net") || strings.Contains(v.Detail, "net") {
			found = true
		}
	}
	log.Assert("net_import_named", found, true, found)
	log.PhaseEnd("negative", testutil.OutcomeOK)
}

// TestGoldenDisciplineDetectsMissingCI is a negative workflow check.
func TestGoldenDisciplineDetectsMissingCI(t *testing.T) {
	log := testutil.New(t)
	log.Phase("negative")
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"),
		[]byte("module github.com/robertguss/go-foundry-cli\n\ngo 1.26.5\n"), 0o644); err != nil {
		log.Fail("gomod", err.Error())
	}
	// Copy-like testing doc with required needles.
	docDir := filepath.Join(tmp, "docs", "dev")
	if err := os.MkdirAll(docDir, 0o755); err != nil {
		log.Fail("mkdir_doc", err.Error())
	}
	doc := "# testing\nUPDATE_GOLDEN=1 one named suite at a time; CI must never auto-update goldens.\n"
	if err := os.WriteFile(filepath.Join(docDir, "testing.md"), []byte(doc), 0o644); err != nil {
		log.Fail("doc", err.Error())
	}
	wfDir := filepath.Join(tmp, ".github", "workflows")
	if err := os.MkdirAll(wfDir, 0o755); err != nil {
		log.Fail("mkdir_wf", err.Error())
	}
	// Workflow deliberately omits CI=true and sets UPDATE_GOLDEN.
	badWF := "name: ci\njobs:\n  t:\n    runs-on: ubuntu-latest\n    env:\n      UPDATE_GOLDEN: \"1\"\n    steps:\n      - run: go test ./...\n"
	if err := os.WriteFile(filepath.Join(wfDir, "ci.yml"), []byte(badWF), 0o644); err != nil {
		log.Fail("wf", err.Error())
	}
	gvs, err := archtest.CheckGoldenDiscipline(tmp)
	if err != nil {
		log.Fail("check", err.Error())
	}
	log.Assert("detected", len(gvs) >= 2, ">=2", len(gvs))
	var rules []string
	for _, v := range gvs {
		rules = append(rules, v.Rule)
		log.Step("hit", testutil.OutcomeOK, v.String())
	}
	sort.Strings(rules)
	log.Step("rules", testutil.OutcomeOK, strings.Join(rules, ","))
	hasNoUpdate := false
	hasCI := false
	for _, r := range rules {
		if r == "ci_no_update_golden" {
			hasNoUpdate = true
		}
		if r == "ci_sets_ci_env" {
			hasCI = true
		}
	}
	log.Assert("flags_update_golden", hasNoUpdate, true, hasNoUpdate)
	log.Assert("flags_missing_ci", hasCI, true, hasCI)
	log.PhaseEnd("negative", testutil.OutcomeOK)
}

// TestQualitySuiteDeterminismCount2 ensures sorted outputs stay stable.
func TestQualitySuiteDeterminismCount2(t *testing.T) {
	log := testutil.New(t)
	root := repoRoot(t)
	log.Phase("determinism")
	a, err := archtest.CheckPurity(root)
	if err != nil {
		log.Fail("purity_a", err.Error())
	}
	b, err := archtest.CheckPurity(root)
	if err != nil {
		log.Fail("purity_b", err.Error())
	}
	log.Assert("purity_len", len(a) == len(b), len(a), len(b))
	ga, err := archtest.CheckGoldenDiscipline(root)
	if err != nil {
		log.Fail("golden_a", err.Error())
	}
	gb, err := archtest.CheckGoldenDiscipline(root)
	if err != nil {
		log.Fail("golden_b", err.Error())
	}
	log.Assert("golden_len", len(ga) == len(gb), len(ga), len(gb))
	ra := archtest.P1RegisteredIDs()
	rb := archtest.P1RegisteredIDs()
	log.Assert("ids_equal", strings.Join(ra, ",") == strings.Join(rb, ","), true, false)
	log.PhaseEnd("determinism", testutil.OutcomeOK)
}
