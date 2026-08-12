package archtest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/archtest"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestSection58RedlinesRegisterComplete(t *testing.T) {
	log := testutil.New(t)
	log.Phase("assert_register")
	log.Inputs(map[string]string{
		"doc":   archtest.DocRedlinesPath,
		"count": itoa(len(archtest.Redlines)),
		"suite": "section58",
	})

	required := []string{
		"RL-58-VERIFY-BYPASS",
		"RL-58-OFFLINE",
		"RL-58-FORCE",
		"RL-58-EXISTING-MOD",
		"RL-58-PLUGINS",
		"RL-58-PROFILE-FRAMEWORK",
		"RL-58-TYPED-EMITTERS",
		"RL-58-DEMO",
		"RL-58-PROVENANCE-FILE",
		"RL-58-WINDOWS",
		"RL-58-CLAUDE",
		"RL-58-MISC-STACK",
		"RL-58-STAGE-DELETE",
		"RL-58-DRY-RUN",
	}
	ids := map[string]bool{}
	for _, r := range archtest.Redlines {
		if r.ID == "" || r.ShortName == "" || r.Why == "" || r.Detection == "" {
			log.Fail("redline_fields", r.ID+" missing required fields")
		}
		if ids[r.ID] {
			log.Fail("redline_unique", "duplicate id "+r.ID)
		}
		ids[r.ID] = true
		log.NoteID(r.ID)
	}
	for _, id := range required {
		_, ok := ids[id]
		log.Assert("has_"+id, ok, true, ok)
	}
	// Deterministic ID list under -count=2.
	a := archtest.RedlineIDs()
	b := archtest.RedlineIDs()
	log.Assert("ids_stable", strings.Join(a, ",") == strings.Join(b, ","), strings.Join(a, ","), strings.Join(b, ","))
	log.PhaseEnd("assert_register", testutil.OutcomeOK)
}

func TestSection58RedlinesDocExists(t *testing.T) {
	log := testutil.New(t)
	root := repoRoot(t)
	log.Phase("fixture")
	doc := filepath.Join(root, archtest.DocRedlinesPath)
	log.Fixture("redlines_doc", archtest.DocRedlinesPath)
	log.NotePath(doc)
	log.PhaseEnd("fixture", testutil.OutcomeOK)

	log.Phase("assert")
	data, err := os.ReadFile(doc)
	if err != nil {
		log.Fail("read_doc", err.Error())
	}
	body := string(data)
	for _, id := range archtest.RedlineIDs() {
		ok := strings.Contains(body, id)
		log.Assert("doc_contains_"+id, ok, true, ok)
	}
	ok := strings.Contains(body, "Section 58")
	log.Assert("doc_names_section", ok, true, ok)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestSection58MechanicalGreen(t *testing.T) {
	log := testutil.New(t)
	root := repoRoot(t)
	log.Phase("act")
	log.Step("check_redlines", testutil.OutcomeOK, "root="+filepath.Base(root))
	violations, err := archtest.CheckRedlines(root)
	if err != nil {
		log.Fail("check_error", err.Error())
	}
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	if len(violations) > 0 {
		for _, v := range violations {
			log.Step("violation", testutil.OutcomeFail, v.String())
			t.Errorf("%s", v)
		}
		log.Fail("redlines_clean", itoa(len(violations))+" violation(s); each names redline id")
	}
	log.Assert("zero_violations", true, 0, len(violations))
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestSection58ConsumedByArchitectureCheck(t *testing.T) {
	// P1.8 (4hi) consumes Check(); redline IDs must surface as Violation.Rule.
	log := testutil.New(t)
	root := repoRoot(t)
	log.Phase("act")
	vs, err := archtest.Check(root)
	if err != nil {
		log.Fail("arch_check", err.Error())
	}
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	for _, v := range vs {
		if strings.HasPrefix(v.Rule, "RL-58-") {
			log.Step("unexpected_redline", testutil.OutcomeFail, v.String())
			t.Errorf("architecture check redline hit: %s", v)
		}
	}
	log.Assert("arch_green", len(vs) == 0, 0, len(vs))
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestSection58TokenScanDetectsOffline(t *testing.T) {
	log := testutil.New(t)
	root := t.TempDir()
	log.Phase("arrange")
	// Build a mini module tree with a planted forbidden token.
	cmdDir := filepath.Join(root, "cmd", "foundry")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		log.Fail("mkdir", err.Error())
	}
	// Assemble token so this test file itself stays clean under product scan
	// (this file lives under internal/archtest and IS scanned).
	tok := "--" + "off" + "line"
	src := "package main\n\nfunc help() string { return \"use " + tok + "\" }\n"
	plant := filepath.Join(cmdDir, "planted.go")
	if err := os.WriteFile(plant, []byte(src), 0o644); err != nil {
		log.Fail("write", err.Error())
	}
	log.Fixture("planted", "cmd/foundry/planted.go")
	log.NotePath(plant)
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	vs, err := archtest.CheckRedlines(root)
	if err != nil {
		log.Fail("check", err.Error())
	}
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	found := false
	for _, v := range vs {
		log.Step("hit", testutil.OutcomeInfo, v.String())
		if v.ID == "RL-58-OFFLINE" {
			found = true
			if !strings.Contains(v.String(), "RL-58-OFFLINE") {
				t.Errorf("violation must name redline id: %s", v)
			}
			if !strings.Contains(v.String(), archtest.RedlineRemediation) {
				t.Errorf("violation must name remediation: %s", v)
			}
			if v.Line <= 0 {
				t.Errorf("expected line number, got %d", v.Line)
			}
		}
	}
	log.Assert("detected_offline", found, true, found)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestSection58TokenScanDetectsForceAndVerifyNone(t *testing.T) {
	log := testutil.New(t)
	root := t.TempDir()
	log.Phase("arrange")
	dir := filepath.Join(root, "internal", "cli")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fail("mkdir", err.Error())
	}
	force := "--" + "force"
	vnone := "--" + "verify" + " " + "none"
	dry := "--" + "dry" + "-" + "run"
	src := "package cli\nconst flags = `" + force + " " + vnone + " " + dry + "`\n"
	if err := os.WriteFile(filepath.Join(dir, "flags.go"), []byte(src), 0o644); err != nil {
		log.Fail("write", err.Error())
	}
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	vs, err := archtest.CheckRedlines(root)
	if err != nil {
		log.Fail("check", err.Error())
	}
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	want := map[string]bool{
		"RL-58-FORCE":         false,
		"RL-58-VERIFY-BYPASS": false,
		"RL-58-DRY-RUN":       false,
	}
	for _, v := range vs {
		if _, ok := want[v.ID]; ok {
			want[v.ID] = true
			log.Step("named_"+v.ID, testutil.OutcomeOK, v.String())
		}
	}
	for id, hit := range want {
		log.Assert("detect_"+id, hit, true, hit)
	}
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestSection58ForbiddenPackageDir(t *testing.T) {
	log := testutil.New(t)
	root := t.TempDir()
	log.Phase("arrange")
	compose := filepath.Join(root, "internal", "compose")
	if err := os.MkdirAll(compose, 0o755); err != nil {
		log.Fail("mkdir", err.Error())
	}
	// Directory existence is enough; no need for .go files.
	log.Fixture("compose_dir", "internal/compose")
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	vs, err := archtest.CheckRedlines(root)
	if err != nil {
		log.Fail("check", err.Error())
	}
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	found := false
	for _, v := range vs {
		if v.ID == "RL-58-PROFILE-FRAMEWORK" && strings.Contains(v.File, "compose") {
			found = true
			log.Step("compose_hit", testutil.OutcomeOK, v.String())
		}
	}
	log.Assert("compose_forbidden", found, true, found)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestSection58StageDeleteIdentifier(t *testing.T) {
	log := testutil.New(t)
	root := t.TempDir()
	log.Phase("arrange")
	dir := filepath.Join(root, "internal", "fsx")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fail("mkdir", err.Error())
	}
	// Forbidden API name assembled at runtime for the planted file only.
	name := "Delete" + "Stage"
	src := "package fsx\n\nfunc " + name + "() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "delete.go"), []byte(src), 0o644); err != nil {
		log.Fail("write", err.Error())
	}
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	vs, err := archtest.CheckRedlines(root)
	if err != nil {
		log.Fail("check", err.Error())
	}
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	found := false
	for _, v := range vs {
		if v.ID == "RL-58-STAGE-DELETE" {
			found = true
			if !strings.Contains(v.String(), "RL-58-STAGE-DELETE") {
				t.Errorf("must name redline id: %s", v)
			}
		}
	}
	log.Assert("stage_delete_hit", found, true, found)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestSection58WindowsBuildTag(t *testing.T) {
	log := testutil.New(t)
	root := t.TempDir()
	log.Phase("arrange")
	dir := filepath.Join(root, "internal", "fsx")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fail("mkdir", err.Error())
	}
	// Plant windows build tag without writing the constraint via a single
	// static product-like pattern in this test's own non-planted body beyond
	// the string we write to the temp tree.
	tag := "//go:build " + "windows"
	src := tag + "\n\npackage fsx\n"
	if err := os.WriteFile(filepath.Join(dir, "platform.go"), []byte(src), 0o644); err != nil {
		log.Fail("write", err.Error())
	}
	// Also plant a windows-suffixed filename.
	winName := "path_" + "windows" + ".go"
	if err := os.WriteFile(filepath.Join(dir, winName), []byte("package fsx\n"), 0o644); err != nil {
		log.Fail("write_win_file", err.Error())
	}
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	vs, err := archtest.CheckRedlines(root)
	if err != nil {
		log.Fail("check", err.Error())
	}
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	hits := 0
	for _, v := range vs {
		if v.ID == "RL-58-WINDOWS" {
			hits++
			log.Step("windows_hit", testutil.OutcomeOK, v.String())
		}
	}
	log.Assert("windows_hits_ge_2", hits >= 2, ">=2", hits)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestSection58WindowsNegationAllowed(t *testing.T) {
	log := testutil.New(t)
	root := t.TempDir()
	log.Phase("arrange")
	dir := filepath.Join(root, "internal", "fsx")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fail("mkdir", err.Error())
	}
	// !windows is the unix product surface — allowed.
	src := "//go:build " + "!" + "windows" + "\n\npackage fsx\n"
	if err := os.WriteFile(filepath.Join(dir, "unix.go"), []byte(src), 0o644); err != nil {
		log.Fail("write", err.Error())
	}
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	vs, err := archtest.CheckRedlines(root)
	if err != nil {
		log.Fail("check", err.Error())
	}
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	var windowsHits int
	for _, v := range vs {
		if v.ID == "RL-58-WINDOWS" {
			windowsHits++
			log.Fail("unexpected_windows", v.String())
		}
	}
	log.Assert("no_windows_on_negation", windowsHits == 0, 0, windowsHits)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestSection58TestdataAllowlisted(t *testing.T) {
	log := testutil.New(t)
	root := t.TempDir()
	log.Phase("arrange")
	// Token under testdata must NOT fire.
	td := filepath.Join(root, "internal", "cli", "testdata")
	if err := os.MkdirAll(td, 0o755); err != nil {
		log.Fail("mkdir", err.Error())
	}
	tok := "--" + "off" + "line"
	if err := os.WriteFile(filepath.Join(td, "help.go"), []byte("package testdata\nvar s = \""+tok+"\"\n"), 0o644); err != nil {
		log.Fail("write", err.Error())
	}
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	vs, err := archtest.CheckRedlines(root)
	if err != nil {
		log.Fail("check", err.Error())
	}
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	var offlineHits int
	for _, v := range vs {
		if v.ID == "RL-58-OFFLINE" {
			offlineHits++
			log.Fail("testdata_not_skipped", v.String())
		}
	}
	log.Assert("testdata_allowlisted", offlineHits == 0, 0, offlineHits)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestSection58ViperImportForbidden(t *testing.T) {
	log := testutil.New(t)
	root := t.TempDir()
	log.Phase("arrange")
	dir := filepath.Join(root, "internal", "cli")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fail("mkdir", err.Error())
	}
	// Import path assembled so this test source does not need the full literal
	// for the planted file only — we write the full import into the temp tree.
	imp := "github.com/spf13/" + "viper"
	src := "package cli\n\nimport \"" + imp + "\"\n\nvar _ = viper.New\n"
	if err := os.WriteFile(filepath.Join(dir, "cfg.go"), []byte(src), 0o644); err != nil {
		log.Fail("write", err.Error())
	}
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	vs, err := archtest.CheckRedlines(root)
	if err != nil {
		log.Fail("check", err.Error())
	}
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	found := false
	for _, v := range vs {
		if v.ID == "RL-58-MISC-STACK" {
			found = true
		}
	}
	log.Assert("viper_import", found, true, found)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestSection58DeterminismCount2(t *testing.T) {
	// Property: CheckRedlines result is order-stable across repeated calls.
	log := testutil.New(t)
	root := repoRoot(t)
	log.Phase("act")
	a, err := archtest.CheckRedlines(root)
	if err != nil {
		log.Fail("a", err.Error())
	}
	b, err := archtest.CheckRedlines(root)
	if err != nil {
		log.Fail("b", err.Error())
	}
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	sa := stringifyReds(a)
	sb := stringifyReds(b)
	log.Assert("stable", sa == sb, sa, sb)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func stringifyReds(vs []archtest.RedlineViolation) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = v.String()
	}
	return strings.Join(parts, "\n")
}

func itoa(n int) string {
	// tiny helper to avoid strconv noise in logs
	if n == 0 {
		return "0"
	}
	var neg bool
	if n < 0 {
		neg = true
		n = -n
	}
	var b [20]byte
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
