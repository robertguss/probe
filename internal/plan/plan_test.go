package plan_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestConstructMinimalCLI builds a plan for MVP profiles=[] CLI and checks
// structural invariants + step log contract.
func TestConstructMinimalCLI(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	cat := mustLoadCatalog(t)
	vs := minimalCLI(t)
	rp := mustResolve(t, vs, cat)
	dest := fixedDestination(vs.Destination(), plan.ObservationAbsent)
	log.Inputs(map[string]string{
		"fixture":   "minimal-cli",
		"archetype": vs.Archetype(),
		"verify":    "default",
		"profiles":  "[]",
	})
	log.Fixture("catalog_digest", string(cat.Digest()))
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	p := mustConstruct(t, defaultInputs(rp, cat, dest))
	log.Step("construct", testutil.OutcomeOK, plan.Summary(p))
	log.Step("file_count", testutil.OutcomeOK, "n="+itoa(p.FileCount()))
	log.Step("external_step_count", testutil.OutcomeOK, "n="+itoa(p.ExternalStepCount()))
	log.Step("plan_sha256", testutil.OutcomeOK, p.PlanSHA256())
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	log.Assert("schema", p.Schema() == plan.SchemaVersion, plan.SchemaVersion, p.Schema())
	log.Assert("name", p.Project().Name == "minimal-cli", "minimal-cli", p.Project().Name)
	log.Assert("archetype", p.Project().Archetype == "cli", "cli", p.Project().Archetype)
	log.Assert("profiles_empty", len(p.Profiles()) == 0, 0, len(p.Profiles()))
	log.Assert("profiles_non_nil", p.Profiles() != nil, true, p.Profiles() != nil)
	log.Assert("commit_model", p.CommitResultModel() == plan.CommitResultModelRef,
		plan.CommitResultModelRef, p.CommitResultModel())
	log.Assert("git_init", p.Git().Init == true, true, p.Git().Init)
	log.Assert("git_isolated", p.Git().Isolated == true, true, p.Git().Isolated)
	log.Assert("verify_default", p.Verification().Mode == plan.VerifyDefault,
		plan.VerifyDefault, p.Verification().Mode)

	// Files: Section 16.2 Core + Section 17.2 CLI shell + typed go.mod — sorted.
	files := p.Files()
	// 11 core + 9 CLI archetype + 1 go.mod = 21.
	log.Assert("file_count", len(files) == 21, 21, len(files))
	wantPaths := []string{
		".github/dependabot.yml",
		".github/workflows/ci.yml",
		".github/workflows/strict.yml",
		".gitignore",
		"AGENTS.md",
		"README.md",
		"cmd/minimal-cli/main.go",
		"docs/architecture.md",
		"docs/commands.md",
		"docs/testing.md",
		"go.mod",
		"internal/cli/completion.go",
		"internal/cli/root.go",
		"internal/cli/root_test.go",
		"internal/cli/script_test.go",
		"internal/cli/testdata/script/completion.txt",
		"internal/cli/testdata/script/help.txt",
		"internal/cli/testdata/script/version.txt",
		"internal/cli/version.go",
		"internal/version/version.go",
		"internal/version/version_test.go",
	}
	for i, want := range wantPaths {
		log.Assert("file_path_"+itoa(i), files[i].Path == want, want, files[i].Path)
	}
	for i := 1; i < len(files); i++ {
		log.Assert("files_sorted_"+itoa(i), files[i-1].Path < files[i].Path,
			files[i-1].Path, files[i].Path)
	}
	// Digests present.
	for _, f := range files {
		log.Assert("content_sha256_"+f.Path, len(f.ContentSHA256) == 64, 64, len(f.ContentSHA256))
		if f.Render != "gomod" {
			log.Assert("source_sha256_"+f.Path, len(f.SourceSHA256) == 64, 64, len(f.SourceSHA256))
		}
	}

	// Dependencies: core go-cmp (test) + cobra (+ testscript).
	deps := p.Dependencies()
	log.Assert("has_deps", len(deps) >= 2, 2, len(deps))
	log.Assert("cobra", containsDep(deps, "github.com/spf13/cobra"), true, false)
	log.Assert("go_cmp", containsDep(deps, "github.com/google/go-cmp"), true, false)

	// Tools: staticcheck + govulncheck.
	tools := p.Tools()
	log.Assert("tools_count", len(tools) == 2, 2, len(tools))

	// external_steps cwd + order.
	steps := p.ExternalSteps()
	log.Assert("steps_ge_5", len(steps) >= 5, 5, len(steps)) // tidy,verify,test,vet,git
	for _, s := range steps {
		log.Assert("cwd_"+s.ID, s.Cwd == plan.StageDescriptorCWD,
			plan.StageDescriptorCWD, s.Cwd)
		log.Assert("cap_"+s.ID, s.OutputCapBytes == plan.OutputCapBytes,
			plan.OutputCapBytes, s.OutputCapBytes)
	}
	log.Assert("first_tidy", steps[0].ID == "go-mod-tidy", "go-mod-tidy", steps[0].ID)
	log.Assert("has_git", steps[len(steps)-1].ID == "git-init", "git-init", steps[len(steps)-1].ID)

	// Network disclosure.
	log.Assert("network_may", p.Network().MayBeRequired == true, true, p.Network().MayBeRequired)
	log.Assert("network_reasons", len(p.Network().Reasons) >= 1, 1, len(p.Network().Reasons))

	// plan_sha256 length.
	log.Assert("sha_len", len(p.PlanSHA256()) == 64, 64, len(p.PlanSHA256()))
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestConstructStrictAddsAnalysisSteps locks strict external_steps + checks.
func TestConstructStrictAddsAnalysisSteps(t *testing.T) {
	log := testutil.New(t)
	log.Phase("strict")
	cat := mustLoadCatalog(t)
	vs := minimalCLI(t)
	rp := mustResolve(t, vs, cat)
	in := defaultInputs(rp, cat, fixedDestination(vs.Destination(), plan.ObservationAbsent))
	in.Verify = plan.VerifyStrict
	p := mustConstruct(t, in)
	log.Step("steps", testutil.OutcomeOK, "n="+itoa(p.ExternalStepCount()))

	ids := stepIDs(p)
	log.Assert("has_staticcheck", contains(ids, "go-staticcheck"), true, false)
	log.Assert("has_govulncheck", contains(ids, "go-govulncheck"), true, false)
	checks := p.Verification().Checks
	log.Assert("check_staticcheck", contains(checks, "go-staticcheck"), true, false)
	log.Assert("check_govulncheck", contains(checks, "go-govulncheck"), true, false)
	log.Assert("final_last", checks[len(checks)-1] == "final-conformance",
		"final-conformance", checks[len(checks)-1])
	// Network reasons include govulncheck.
	found := false
	for _, r := range p.Network().Reasons {
		if strings.Contains(r, "govulncheck") {
			found = true
		}
	}
	log.Assert("govulncheck_reason", found, true, false)
	log.PhaseEnd("strict", testutil.OutcomeOK)
}

// TestDeterminismCount2 proves identical inputs → byte-identical JSON + plan_sha256.
func TestDeterminismCount2(t *testing.T) {
	log := testutil.New(t)
	log.Phase("determinism")
	cat := mustLoadCatalog(t)
	vs := minimalCLI(t)
	rp := mustResolve(t, vs, cat)
	in := defaultInputs(rp, cat, fixedDestination(vs.Destination(), plan.ObservationAbsent))

	p1 := mustConstruct(t, in)
	p2 := mustConstruct(t, in)
	log.Step("plan_sha256_1", testutil.OutcomeOK, p1.PlanSHA256())
	log.Step("plan_sha256_2", testutil.OutcomeOK, p2.PlanSHA256())
	log.Assert("sha_equal", p1.PlanSHA256() == p2.PlanSHA256(), p1.PlanSHA256(), p2.PlanSHA256())
	log.Assert("json_equal", bytes.Equal(p1.JSON(), p2.JSON()), true, false)
	log.Assert("equal_method", p1.Equal(p2), true, false)

	// plan_sha256 algorithm: SHA-256 of body without plan_sha256 field.
	bodySHA := recomputeBodySHA(t, p1)
	log.Assert("algo_match", bodySHA == p1.PlanSHA256(), bodySHA, p1.PlanSHA256())
	log.PhaseEnd("determinism", testutil.OutcomeOK)
}

// TestSortedCollections verifies files, deps, profiles, tools sort order.
func TestSortedCollections(t *testing.T) {
	log := testutil.New(t)
	log.Phase("sort")
	cat := mustLoadCatalog(t)
	vs := minimalCLI(t)
	rp := mustResolve(t, vs, cat)
	p := mustConstruct(t, defaultInputs(rp, cat, fixedDestination(vs.Destination(), plan.ObservationAbsent)))

	files := p.Files()
	for i := 1; i < len(files); i++ {
		log.Assert("files_"+itoa(i), files[i-1].Path <= files[i].Path, files[i-1].Path, files[i].Path)
	}
	deps := p.Dependencies()
	for i := 1; i < len(deps); i++ {
		log.Assert("deps_"+itoa(i), deps[i-1].Module <= deps[i].Module, deps[i-1].Module, deps[i].Module)
	}
	tools := p.Tools()
	for i := 1; i < len(tools); i++ {
		log.Assert("tools_"+itoa(i), tools[i-1].Module <= tools[i].Module, tools[i-1].Module, tools[i].Module)
	}
	profiles := p.Profiles()
	for i := 1; i < len(profiles); i++ {
		log.Assert("profiles_"+itoa(i), profiles[i-1] <= profiles[i], profiles[i-1], profiles[i])
	}
	log.PhaseEnd("sort", testutil.OutcomeOK)
}

// TestImmutabilityAfterConstruct: mutating returned slices must not affect Plan.
func TestImmutabilityAfterConstruct(t *testing.T) {
	log := testutil.New(t)
	log.Phase("immutability")
	cat := mustLoadCatalog(t)
	vs := minimalCLI(t)
	rp := mustResolve(t, vs, cat)
	p := mustConstruct(t, defaultInputs(rp, cat, fixedDestination(vs.Destination(), plan.ObservationAbsent)))

	origJSON := p.JSON()
	origSHA := p.PlanSHA256()

	files := p.Files()
	if len(files) > 0 {
		files[0].Path = "MUTATED"
		files[0].ContentSHA256 = "deadbeef"
	}
	profiles := p.Profiles()
	if profiles != nil {
		// append to slice header only; copy should not share backing if we re-get
		_ = append(profiles, "evil-profile")
	}
	steps := p.ExternalSteps()
	if len(steps) > 0 {
		steps[0].ID = "mutated"
		if steps[0].Env != nil {
			steps[0].Env["GOENV"] = "evil"
		}
	}
	j := p.JSON()
	j[0] = 'X'

	log.Assert("sha_stable", p.PlanSHA256() == origSHA, origSHA, p.PlanSHA256())
	log.Assert("json_stable", bytes.Equal(p.JSON(), origJSON), true, false)
	// Re-fetch files must be unmutated.
	files2 := p.Files()
	log.Assert("file0_unmutated", files2[0].Path != "MUTATED", true, files2[0].Path == "MUTATED")
	steps2 := p.ExternalSteps()
	log.Assert("step0_unmutated", steps2[0].ID != "mutated", true, steps2[0].ID == "mutated")
	log.PhaseEnd("immutability", testutil.OutcomeOK)
}

// TestSharedConstructionPath proves Pipeline == Resolve+Construct and that
// validate can discard the plan from the same entrypoint (REQ-032/033).
func TestSharedConstructionPath(t *testing.T) {
	log := testutil.New(t)
	log.Phase("shared_path")
	cat := mustLoadCatalog(t)
	vs := minimalCLI(t)
	dest := fixedDestination(vs.Destination(), plan.ObservationAbsent)
	opts := defaultPipelineOpts(dest)

	// Path A: Pipeline (validate/plan/generate entry)
	pPipe, err := plan.Pipeline(vs, cat, opts)
	if err != nil {
		log.Fail("pipeline", err.Error())
	}
	// Path B: explicit Resolve + Construct
	rp := mustResolve(t, vs, cat)
	pExpl := mustConstruct(t, defaultInputs(rp, cat, dest))

	log.Step("pipe_sha", testutil.OutcomeOK, pPipe.PlanSHA256())
	log.Step("expl_sha", testutil.OutcomeOK, pExpl.PlanSHA256())
	log.Assert("shared_equal", pPipe.Equal(pExpl), true, false)
	log.Assert("json_equal", bytes.Equal(pPipe.JSON(), pExpl.JSON()), true, false)

	// validate discards plan: same construction, result unused.
	var discarded *plan.Plan
	discarded, err = plan.Pipeline(vs, cat, opts)
	if err != nil {
		log.Fail("validate_pipeline", err.Error())
	}
	_ = discarded // validate would drop this
	log.Assert("validate_would_succeed", discarded != nil && discarded.PlanSHA256() == pPipe.PlanSHA256(),
		true, false)
	log.PhaseEnd("shared_path", testutil.OutcomeOK)
}

// TestExternalStepsCWDIsStageDescriptor forbids host paths in cwd.
func TestExternalStepsCWDIsStageDescriptor(t *testing.T) {
	log := testutil.New(t)
	log.Phase("cwd")
	cat := mustLoadCatalog(t)
	vs := minimalCLI(t)
	rp := mustResolve(t, vs, cat)
	p := mustConstruct(t, defaultInputs(rp, cat, fixedDestination(vs.Destination(), plan.ObservationAbsent)))
	for _, s := range p.ExternalSteps() {
		log.Assert("cwd_"+s.ID, s.Cwd == plan.StageDescriptorCWD, plan.StageDescriptorCWD, s.Cwd)
		log.Assert("not_abs_"+s.ID, !strings.HasPrefix(s.Cwd, "/"), true, s.Cwd)
		log.Assert("not_tmp_"+s.ID, !strings.Contains(s.Cwd, "tmp"), true, s.Cwd)
	}
	log.PhaseEnd("cwd", testutil.OutcomeOK)
}

// TestConstructTUI builds MVP TUI plan (profiles=[]).
func TestConstructTUI(t *testing.T) {
	log := testutil.New(t)
	log.Phase("tui")
	cat := mustLoadCatalog(t)
	vs := minimalTUI(t)
	rp := mustResolve(t, vs, cat)
	p := mustConstruct(t, defaultInputs(rp, cat, fixedDestination(vs.Destination(), plan.ObservationAbsent)))
	log.Step("summary", testutil.OutcomeOK, plan.Summary(p))
	log.Assert("archetype", p.Project().Archetype == "tui", "tui", p.Project().Archetype)
	paths := make([]string, 0, p.FileCount())
	for _, f := range p.Files() {
		paths = append(paths, f.Path)
		if strings.HasPrefix(f.Path, "cmd/") {
			log.Assert("binary_expanded", strings.Contains(f.Path, "minimal-tui"), true, f.Path)
			log.Assert("no_token", !strings.Contains(f.Path, "{{"), true, f.Path)
		}
	}
	log.Assert("has_gomod", contains(paths, "go.mod"), true, false)
	log.PhaseEnd("tui", testutil.OutcomeOK)
}

// TestPlanSHA256AlgorithmNote documents and verifies the digest algorithm.
func TestPlanSHA256AlgorithmNote(t *testing.T) {
	// Algorithm (REQ-121 notes / Section 28.2 plan_digest):
	//   plan_sha256 = hex(SHA-256(canonical_json_without_plan_sha256_field))
	// Canonical JSON is encoding/json compact form, map keys sorted, no HTML
	// escape, no trailing newline in the hashed body.
	log := testutil.New(t)
	log.Phase("algo")
	cat := mustLoadCatalog(t)
	vs := minimalCLI(t)
	rp := mustResolve(t, vs, cat)
	p := mustConstruct(t, defaultInputs(rp, cat, fixedDestination(vs.Destination(), plan.ObservationAbsent)))
	got := recomputeBodySHA(t, p)
	log.Assert("match", got == p.PlanSHA256(), got, p.PlanSHA256())
	log.PhaseEnd("algo", testutil.OutcomeOK)
}

func recomputeBodySHA(t *testing.T, p *plan.Plan) string {
	t.Helper()
	// Strip plan_sha256 from sealed JSON and re-hash. Field order is fixed so
	// a single-field deletion yields the body that Construct hashed.
	raw := bytes.TrimSuffix(p.JSON(), []byte("\n"))
	const key = `,"plan_sha256":"`
	i := bytes.Index(raw, []byte(key))
	if i < 0 {
		t.Fatalf("plan_sha256 field not found in JSON")
	}
	// value is 64 hex chars + closing quote
	start := i
	end := i + len(key) + 64 + 1
	if end > len(raw) || raw[end-1] != '"' {
		t.Fatalf("plan_sha256 value bounds invalid end=%d len=%d", end, len(raw))
	}
	body := append(append([]byte{}, raw[:start]...), raw[end:]...)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func stepIDs(p *plan.Plan) []string {
	steps := p.ExternalSteps()
	ids := make([]string, len(steps))
	for i, s := range steps {
		ids[i] = s.ID
	}
	return ids
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func containsDep(deps []plan.DependencyEntry, mod string) bool {
	for _, d := range deps {
		if d.Module == mod {
			return true
		}
	}
	return false
}
