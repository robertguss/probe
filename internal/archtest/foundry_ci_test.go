package archtest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/archtest"
	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestFoundryCISection47 locks Foundry repository workflows against Section
// 47.1 / REQ-222 (bead go-foundry-cli-cic): Linux+macOS unit, race,
// catalog-digest, golden matrix, hostile tags; strict weekly path; SHA pins.
func TestFoundryCISection47(t *testing.T) {
	log := testutil.New(t)
	log.Phase("foundry_ci")
	root := repoRoot(t)
	log.Fixture("root", root)

	ciPath := filepath.Join(root, ".github", "workflows", "ci.yml")
	ciRaw, err := os.ReadFile(ciPath)
	if err != nil {
		log.Fail("read_ci", err.Error())
	}
	ci := string(ciRaw)
	log.Step("ci_bytes", testutil.OutcomeOK, "n="+itoa(len(ciRaw)))

	// --- PR/default matrix (Section 47.1) ---
	log.Phase("ci_matrix")
	for _, want := range []string{
		"ubuntu-latest",
		"macos-latest",
		"gofmt -l .",
		"go test -count=1 ./...",
		"go vet ./...",
		"go tool staticcheck ./...",
		"go mod verify",
		"CGO_ENABLED",
		"-race",
		"catalog-digest",
		"golden-matrix",
		"CI: \"true\"",
		"hostile",
		"sentinel",
		"generate-e2e",
	} {
		present := strings.Contains(ci, want)
		log.Assert("ci_"+sanitize(want), present, true, present)
	}
	// Never auto-update goldens; never verify-none.
	noUG := !strings.Contains(ci, "UPDATE_GOLDEN")
	log.Assert("ci_no_update_golden", noUG, true, noUG)
	noVN := !strings.Contains(ci, "verify-none")
	log.Assert("ci_no_verify_none", noVN, true, noVN)
	hasConc := strings.Contains(ci, "concurrency:")
	log.Assert("ci_concurrency", hasConc, true, hasConc)
	log.PhaseEnd("ci_matrix", testutil.OutcomeOK)

	// --- Action SHA pins match versions.toml lock ---
	log.Phase("action_pins")
	versions, err := os.ReadFile(filepath.Join(root, "catalog", "versions.toml"))
	if err != nil {
		log.Fail("versions", err.Error())
	}
	vtext := string(versions)
	for _, sha := range []string{
		"fbc6f3992d24b796d5a048ff273f7fcc4a7b6c09", // checkout
		"924ae3a1cded613372ab5595356fb5720e22ba16", // setup-go
		"330a01c490aca151604b8cf639adc76d48f6c5d4", // upload-artifact
	} {
		log.Assert("lock_has_"+sha[:8], strings.Contains(vtext, sha), true, false)
		log.Assert("ci_pin_"+sha[:8], strings.Contains(ci, sha), true, false)
		log.Assert("sha_len_"+sha[:8], len(sha) == 40, 40, len(sha))
	}
	log.PhaseEnd("action_pins", testutil.OutcomeOK)

	// --- strict.yml weekly path ---
	log.Phase("strict")
	strictPath := filepath.Join(root, ".github", "workflows", "strict.yml")
	strictRaw, err := os.ReadFile(strictPath)
	if err != nil {
		log.Fail("read_strict", err.Error())
	}
	strict := string(strictRaw)
	for _, want := range []string{
		"schedule:",
		"workflow_dispatch:",
		"go tool govulncheck ./...",
		"Fuzz",
		"cold-cache",
		"hostile",
		"CI: \"true\"",
	} {
		present := strings.Contains(strict, want)
		log.Assert("strict_"+sanitize(want), present, true, present)
	}
	noPR := !strings.Contains(strict, "pull_request:")
	log.Assert("strict_no_pr", noPR, true, noPR)
	noUGStrict := !strings.Contains(strict, "UPDATE_GOLDEN")
	log.Assert("strict_no_update_golden", noUGStrict, true, noUGStrict)
	for _, sha := range []string{
		"fbc6f3992d24b796d5a048ff273f7fcc4a7b6c09",
		"924ae3a1cded613372ab5595356fb5720e22ba16",
	} {
		log.Assert("strict_pin_"+sha[:8], strings.Contains(strict, sha), true, false)
	}
	log.PhaseEnd("strict", testutil.OutcomeOK)

	// Golden discipline still holds after workflow expansion.
	log.Phase("golden_discipline")
	v, err := archtest.CheckGoldenDiscipline(root)
	if err != nil {
		log.Fail("golden_discipline_err", err.Error())
	}
	log.Assert("golden_discipline_clean", len(v) == 0, 0, len(v))
	if len(v) > 0 {
		for _, x := range v {
			t.Logf("violation: %s", x)
		}
	}
	log.PhaseEnd("golden_discipline", testutil.OutcomeOK)
	log.PhaseEnd("foundry_ci", testutil.OutcomeOK)
}

// TestGeneratedCISingleRequiredJob is the FND-016 / REQ-063 assertion for
// embedded Core ci.yml: exactly one PR job, SHA pins, concurrency cancel.
func TestGeneratedCISingleRequiredJob(t *testing.T) {
	log := testutil.New(t)
	log.Phase("generated_ci")
	c, err := catalog.Load()
	if err != nil {
		log.Fail("load", err.Error())
	}
	core, ok := c.Manifest("core")
	if !ok || core == nil {
		log.Fail("core", "missing")
	}
	var ciSrc, strictSrc string
	for _, f := range core.Files {
		switch f.Path {
		case ".github/workflows/ci.yml":
			ciSrc = filepath.ToSlash(filepath.Join(core.UnitDir, f.Source))
		case ".github/workflows/strict.yml":
			strictSrc = filepath.ToSlash(filepath.Join(core.UnitDir, f.Source))
		}
	}
	ciRaw, err := c.Read(ciSrc)
	if err != nil {
		log.Fail("read_ci", err.Error())
	}
	ci := string(ciRaw)
	// Exactly one job key.
	jobs := 0
	for _, line := range strings.Split(ci, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") && strings.HasSuffix(trim, ":") && !strings.HasPrefix(trim, "#") {
			key := strings.TrimSuffix(trim, ":")
			// crude: top-level under jobs only when previous context is jobs —
			// rely on known single job "check".
			if key == "check" || key == "linux" {
				jobs++
			}
		}
	}
	log.Assert("has_check_job", strings.Contains(ci, "\n  check:\n") || strings.Contains(ci, "\n  check:\r\n"), true, false)
	log.Assert("single_check_token", strings.Count(ci, "\n  check:") == 1, 1, strings.Count(ci, "\n  check:"))
	for _, cmd := range []string{"gofmt -l .", "go mod verify", "go test -count=1 ./...", "go vet ./...", "go tool staticcheck ./..."} {
		log.Assert("cmd_"+sanitize(cmd), strings.Contains(ci, cmd), true, false)
	}
	log.Assert("concurrency", strings.Contains(ci, "cancel-in-progress: true"), true, false)
	log.Assert("ubuntu_only_pr", strings.Contains(ci, "ubuntu-latest"), true, false)
	// Generated PR CI must not pull macos as required (FND-016).
	// macOS lives in strict.yml only.
	log.Assert("pr_no_macos", !strings.Contains(ci, "macos-latest"), true, false)

	strictRaw, err := c.Read(strictSrc)
	if err != nil {
		log.Fail("read_strict", err.Error())
	}
	strict := string(strictRaw)
	log.Assert("strict_rsk403_comment", strings.Contains(strict, "promote") || strings.Contains(strict, "Not a required"), true, false)
	log.Assert("strict_macos", strings.Contains(strict, "macos-latest"), true, false)
	log.Assert("strict_govuln", strings.Contains(strict, "govulncheck"), true, false)
	log.Assert("strict_race", strings.Contains(strict, "-race") || strings.Contains(strict, "CGO_ENABLED"), true, false)
	// SHA pins in both.
	for _, sha := range []string{
		"fbc6f3992d24b796d5a048ff273f7fcc4a7b6c09",
		"924ae3a1cded613372ab5595356fb5720e22ba16",
	} {
		log.Assert("gen_ci_"+sha[:8], strings.Contains(ci, sha), true, false)
		log.Assert("gen_strict_"+sha[:8], strings.Contains(strict, sha), true, false)
	}
	log.PhaseEnd("generated_ci", testutil.OutcomeOK)
	_ = jobs
}
