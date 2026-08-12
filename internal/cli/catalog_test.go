package cli_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/resolve"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// Phase-1 catalog inventory (core + archetypes + catalogued post-MVP profile).
// MVP selection (resolve.ImplementedProfileIDs) stays empty — distribution is
// listed/showable but not generation-selectable until Phase 4.
var phase1CatalogUnitIDs = []string{"cli", "core", "distribution", "tui"}

// TestCatalogListGolden locks text + JSON list output against the embedded
// catalog (REQ-035 acceptance: list goldens).
func TestCatalogListGolden(t *testing.T) {
	log := testutil.New(t)
	log.Phase("list_goldens")
	log.Fixture("catalog", "embedded")

	// Text
	res := runCLI(t, "catalog", "list")
	log.Assert("text_exit_0", res.Code == 0, 0, res.Code)
	log.Assert("text_stderr_empty", res.Stderr == "", "", res.Stderr)
	textOut := normalizeCatalogText(res.Stdout)
	textPath := testutil.GoldenPath("testdata", "catalog_list_text")
	testutil.CompareGolden(t, textPath, []byte(textOut))
	log.Step("text_golden", testutil.OutcomeOK, textPath)

	// Every expected unit appears in order (sorted by id).
	lines := nonEmptyLines(textOut)
	log.Assert("text_line_count", len(lines) == len(phase1CatalogUnitIDs),
		len(phase1CatalogUnitIDs), len(lines))
	for i, id := range phase1CatalogUnitIDs {
		if i >= len(lines) {
			break
		}
		log.Assert("text_id_"+id, strings.HasPrefix(lines[i], id+"\t"), true, lines[i])
	}

	// JSON
	res = runCLI(t, "catalog", "list", "--output", "json")
	log.Assert("json_exit_0", res.Code == 0, 0, res.Code)
	log.Assert("json_stderr_empty", res.Stderr == "", "", res.Stderr)
	jsonOut := normalizeCatalogJSON(t, res.Stdout)
	jsonPath := testutil.GoldenPath("testdata", "catalog_list_json")
	testutil.CompareGolden(t, jsonPath, []byte(jsonOut))
	log.Step("json_golden", testutil.OutcomeOK, jsonPath)

	// Schema fields stable
	var env map[string]any
	if err := json.Unmarshal([]byte(res.Stdout), &env); err != nil {
		log.Fail("json_parse", err.Error())
	}
	log.Assert("envelope_ok", env["ok"] == true, true, env["ok"])
	log.Assert("envelope_command", env["command"] == "catalog list", "catalog list", env["command"])
	log.Assert("envelope_schema", env["schema"] == float64(1), 1, env["schema"])
	result, _ := env["result"].(map[string]any)
	if result == nil {
		log.Fail("result", "missing result object")
	}
	for _, k := range []string{"catalog_digest", "units"} {
		_, ok := result[k]
		log.Assert("field_"+k, ok, true, ok)
	}
	units, _ := result["units"].([]any)
	log.Assert("units_len", len(units) == len(phase1CatalogUnitIDs),
		len(phase1CatalogUnitIDs), len(units))
	if len(units) > 0 {
		u0, _ := units[0].(map[string]any)
		for _, k := range []string{"id", "kind", "description", "manifest_path"} {
			_, ok := u0[k]
			log.Assert("unit_field_"+k, ok, true, ok)
		}
	}

	log.PhaseEnd("list_goldens", testutil.OutcomeOK)
}

// TestCatalogShowGoldens locks show text+JSON for core and each archetype,
// and asserts distribution is showable as catalogued post-MVP content.
func TestCatalogShowGoldens(t *testing.T) {
	log := testutil.New(t)
	log.Phase("show_goldens")
	log.Fixture("catalog", "embedded")

	// Core + archetypes get full text+JSON goldens (acceptance).
	for _, id := range []string{"core", "cli", "tui"} {
		t.Run(id, func(t *testing.T) {
			sub := testutil.New(t)
			sub.NoteID(id)
			sub.Phase("show_hit")

			// Text
			res := runCLI(t, "catalog", "show", id)
			sub.Assert("text_exit_0", res.Code == 0, 0, res.Code)
			sub.Assert("text_stderr_empty", res.Stderr == "", "", res.Stderr)
			textOut := normalizeCatalogText(res.Stdout)
			textPath := testutil.GoldenPath("testdata", "catalog_show_"+id+"_text")
			testutil.CompareGolden(t, textPath, []byte(textOut))
			sub.Step("text_golden", testutil.OutcomeOK, textPath)
			sub.Assert("text_has_id", strings.Contains(textOut, "id="+id), true, textOut)
			sub.Assert("text_has_digest", strings.Contains(textOut, "catalog_digest="), true, textOut)

			// JSON
			res = runCLI(t, "catalog", "show", id, "--output", "json")
			sub.Assert("json_exit_0", res.Code == 0, 0, res.Code)
			sub.Assert("json_stderr_empty", res.Stderr == "", "", res.Stderr)
			jsonOut := normalizeCatalogJSON(t, res.Stdout)
			jsonPath := testutil.GoldenPath("testdata", "catalog_show_"+id+"_json")
			testutil.CompareGolden(t, jsonPath, []byte(jsonOut))
			sub.Step("json_golden", testutil.OutcomeOK, jsonPath)

			var env map[string]any
			if err := json.Unmarshal([]byte(res.Stdout), &env); err != nil {
				sub.Fail("json_parse", err.Error())
			}
			sub.Assert("ok", env["ok"] == true, true, env["ok"])
			sub.Assert("command", env["command"] == "catalog show", "catalog show", env["command"])
			result, _ := env["result"].(map[string]any)
			if result == nil {
				sub.Fail("result", "missing")
			}
			for _, k := range []string{
				"catalog_digest", "id", "kind", "schema", "description",
				"manifest_path", "files", "dependencies",
			} {
				_, ok := result[k]
				sub.Assert("field_"+k, ok, true, ok)
			}
			sub.Assert("id_match", result["id"] == id, id, result["id"])
			sub.PhaseEnd("show_hit", testutil.OutcomeOK)
		})
	}

	// distribution is catalogued (list/show) but not MVP-selectable.
	log.Phase("distribution_catalogued")
	log.NoteID("distribution")
	res := runCLI(t, "catalog", "show", "distribution", "--output", "json")
	log.Assert("dist_show_0", res.Code == 0, 0, res.Code)
	log.Assert("dist_kind_profile", strings.Contains(res.Stdout, `"kind":"profile"`), true, res.Stdout)
	log.Assert("dist_compat", strings.Contains(res.Stdout, "compatible_archetypes"), true, res.Stdout)
	log.PhaseEnd("distribution_catalogued", testutil.OutcomeOK)

	log.PhaseEnd("show_goldens", testutil.OutcomeOK)
}

// TestCatalogShowUnknown asserts exact catalog.invalid for unknown ids
// (acceptance: unknown id errors; step log hit/miss).
func TestCatalogShowUnknown(t *testing.T) {
	log := testutil.New(t)
	log.Phase("show_miss")

	missIDs := []string{
		"does-not-exist-unit",
		"configuration",
		"local-persistence",
		"CLI", // case-sensitive
		"nope",
	}
	for _, id := range missIDs {
		t.Run("text_"+id, func(t *testing.T) {
			sub := testutil.New(t)
			sub.NoteID(id)
			sub.Inputs(map[string]string{"id": id, "mode": "text"})
			res := runCLI(t, "catalog", "show", id)
			sub.Assert("exit_1", res.Code == diagnostic.ExitFailure, diagnostic.ExitFailure, res.Code)
			sub.Assert("id_catalog_invalid", strings.Contains(res.Stderr, "catalog.invalid"), true, res.Stderr)
			sub.Assert("names_available", strings.Contains(res.Stderr, "available units"), true, res.Stderr)
			for _, want := range phase1CatalogUnitIDs {
				if !strings.Contains(res.Stderr, want) {
					sub.Fail("available_mentions_"+want, res.Stderr)
				}
			}
			sub.Assert("remediation_list", strings.Contains(res.Stderr, "catalog list"), true, res.Stderr)
			// No did-you-mean / fuzzy suggestions.
			sub.Assert("no_did_you_mean", !strings.Contains(strings.ToLower(res.Stderr), "did you mean"),
				false, strings.Contains(strings.ToLower(res.Stderr), "did you mean"))
			sub.Step("miss", testutil.OutcomeOK, "id="+id)
		})

		t.Run("json_"+id, func(t *testing.T) {
			sub := testutil.New(t)
			sub.NoteID(id)
			sub.Inputs(map[string]string{"id": id, "mode": "json"})
			res := runCLI(t, "catalog", "show", id, "--output", "json")
			sub.Assert("exit_1", res.Code == diagnostic.ExitFailure, diagnostic.ExitFailure, res.Code)
			sub.Assert("stderr_empty", res.Stderr == "", "", res.Stderr)
			var env map[string]any
			if err := json.Unmarshal([]byte(res.Stdout), &env); err != nil {
				sub.Fail("json_parse", err.Error())
			}
			sub.Assert("ok_false", env["ok"] == false, false, env["ok"])
			sub.Assert("command", env["command"] == "catalog show", "catalog show", env["command"])
			errObj, _ := env["error"].(map[string]any)
			if errObj == nil {
				sub.Fail("error", "missing error object")
			}
			sub.Assert("error_id", errObj["error_id"] == string(diagnostic.IDCatalogInvalid),
				string(diagnostic.IDCatalogInvalid), errObj["error_id"])
			sub.Assert("exit_code", errObj["exit_code"] == float64(diagnostic.ExitFailure),
				diagnostic.ExitFailure, errObj["exit_code"])
			msg, _ := errObj["message"].(string)
			sub.Assert("msg_available", strings.Contains(msg, "available units"), true, msg)
			sub.Step("miss_json", testutil.OutcomeOK, "id="+id)
		})
	}

	// Missing arg → usage (exit 2)
	res := runCLI(t, "catalog", "show")
	log.Assert("missing_arg_2", res.Code == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Code)

	log.PhaseEnd("show_miss", testutil.OutcomeOK)
}

// TestCatalogDigestMatchesVersion ensures list/show catalog_digest equals
// version's catalog_digest and the direct catalog.Load digest.
func TestCatalogDigestMatchesVersion(t *testing.T) {
	log := testutil.New(t)
	log.Phase("digest_match")

	cat, err := catalog.Load()
	if err != nil {
		log.Fail("catalog_load", err.Error())
	}
	want := string(cat.Digest())
	log.Fixture("catalog_digest", want)
	log.Assert("digest_len", len(want) == 64, 64, len(want))

	ver := runCLI(t, "version", "--output", "json")
	log.Assert("version_0", ver.Code == 0, 0, ver.Code)
	var verEnv map[string]any
	if err := json.Unmarshal([]byte(ver.Stdout), &verEnv); err != nil {
		log.Fail("version_parse", err.Error())
	}
	verResult, _ := verEnv["result"].(map[string]any)
	verDig, _ := verResult["catalog_digest"].(string)
	log.Assert("version_digest", verDig == want, want, verDig)

	list := runCLI(t, "catalog", "list", "--output", "json")
	log.Assert("list_0", list.Code == 0, 0, list.Code)
	var listEnv map[string]any
	if err := json.Unmarshal([]byte(list.Stdout), &listEnv); err != nil {
		log.Fail("list_parse", err.Error())
	}
	listResult, _ := listEnv["result"].(map[string]any)
	listDig, _ := listResult["catalog_digest"].(string)
	log.Assert("list_digest", listDig == want, want, listDig)

	for _, id := range []string{"core", "cli", "tui", "distribution"} {
		log.NoteID(id)
		show := runCLI(t, "catalog", "show", id, "--output", "json")
		log.Assert("show_0_"+id, show.Code == 0, 0, show.Code)
		var showEnv map[string]any
		if err := json.Unmarshal([]byte(show.Stdout), &showEnv); err != nil {
			log.Fail("show_parse_"+id, err.Error())
		}
		showResult, _ := showEnv["result"].(map[string]any)
		showDig, _ := showResult["catalog_digest"].(string)
		log.Assert("show_digest_"+id, showDig == want, want, showDig)
		log.Step("hit", testutil.OutcomeOK, "id="+id)
	}

	log.PhaseEnd("digest_match", testutil.OutcomeOK)
}

// TestCatalogPhase4DistributionSelectable documents that distribution is
// catalogued and generation-selectable (Phase 4) while recipes are not.
func TestCatalogPhase4DistributionSelectable(t *testing.T) {
	log := testutil.New(t)
	log.Phase("p4_profiles")

	impl := resolve.ImplementedProfileIDs()
	log.Assert("implemented_has_distribution", resolve.IsImplementedProfile("distribution"), true, false)
	log.Assert("implemented_len", len(impl) == 1, 1, len(impl))

	res := runCLI(t, "catalog", "list", "--output", "json")
	log.Assert("list_0", res.Code == 0, 0, res.Code)
	log.Assert("lists_distribution", strings.Contains(res.Stdout, `"id":"distribution"`), true, res.Stdout)
	log.Assert("lists_cli", strings.Contains(res.Stdout, `"id":"cli"`), true, res.Stdout)
	log.Assert("lists_tui", strings.Contains(res.Stdout, `"id":"tui"`), true, res.Stdout)
	log.Assert("lists_core", strings.Contains(res.Stdout, `"id":"core"`), true, res.Stdout)

	// Recipe-only IDs never appear.
	log.Assert("no_configuration", !strings.Contains(res.Stdout, `"id":"configuration"`), true, false)
	log.Assert("no_local_persistence", !strings.Contains(res.Stdout, `"id":"local-persistence"`), true, false)

	help := runCLI(t, "catalog", "list", "--help")
	log.Assert("help_0", help.Code == 0, 0, help.Code)
	log.Assert("help_distribution",
		strings.Contains(help.Stdout, "distribution"),
		true, help.Stdout)
	log.Assert("help_phase4_or_public",
		strings.Contains(help.Stdout, "Phase 4") || strings.Contains(help.Stdout, "public"),
		true, help.Stdout)

	log.PhaseEnd("p4_profiles", testutil.OutcomeOK)
}

// TestCatalogCommandsWriteFree proves list/show are write-free product paths:
// success with only embedded catalog; no --spec; quiet success; no host paths
// in output. Full FS/syscall audit is e2e (5an.1); this unit locks the surface.
func TestCatalogCommandsWriteFree(t *testing.T) {
	log := testutil.New(t)
	log.Phase("write_free")

	// list requires no path flags — pure embed read.
	res := runCLI(t, "catalog", "list")
	log.Assert("list_0", res.Code == 0, 0, res.Code)
	log.Assert("list_no_home", !strings.Contains(res.Stdout, "/home/"), false, strings.Contains(res.Stdout, "/home/"))
	log.Assert("list_no_tmp", !strings.Contains(res.Stdout, "/tmp/"), false, strings.Contains(res.Stdout, "/tmp/"))

	res = runCLI(t, "catalog", "show", "core")
	log.Assert("show_0", res.Code == 0, 0, res.Code)
	log.Assert("show_no_home", !strings.Contains(res.Stdout, "/home/"), false, strings.Contains(res.Stdout, "/home/"))

	// Quiet suppresses text body; still exit 0 (no side effects).
	res = runCLI(t, "catalog", "list", "--quiet")
	log.Assert("quiet_list_0", res.Code == 0, 0, res.Code)
	log.Assert("quiet_list_empty", strings.TrimSpace(res.Stdout) == "", "", res.Stdout)

	// JSON owns stdout exclusively (empty stderr after flag recognition).
	res = runCLI(t, "catalog", "list", "--output", "json")
	log.Assert("json_list_stderr", res.Stderr == "", "", res.Stderr)
	res = runCLI(t, "catalog", "show", "cli", "--output", "json")
	log.Assert("json_show_stderr", res.Stderr == "", "", res.Stderr)

	// Help mentions write-free contract.
	help := runCLI(t, "catalog", "--help")
	log.Assert("catalog_help_write_free",
		strings.Contains(help.Stdout, "no network") || strings.Contains(help.Stdout, "no writes"),
		true, help.Stdout)

	log.PhaseEnd("write_free", testutil.OutcomeOK)
}

// TestCatalogListShowStepLog exercises hit/miss step logging (acceptance).
func TestCatalogListShowStepLog(t *testing.T) {
	log := testutil.New(t)
	log.Phase("step_log_hits")
	for _, id := range phase1CatalogUnitIDs {
		log.NoteID(id)
		res := runCLI(t, "catalog", "show", id)
		log.Assert("hit_"+id, res.Code == 0, 0, res.Code)
		log.Step("show_hit", testutil.OutcomeOK, "id="+id)
	}
	log.PhaseEnd("step_log_hits", testutil.OutcomeOK)

	log.Phase("step_log_miss")
	log.NoteID("unknown-xyz")
	res := runCLI(t, "catalog", "show", "unknown-xyz")
	log.Assert("miss_exit", res.Code == diagnostic.ExitFailure, diagnostic.ExitFailure, res.Code)
	log.Step("show_miss", testutil.OutcomeOK, "id=unknown-xyz")
	log.PhaseEnd("step_log_miss", testutil.OutcomeOK)

	log.Phase("list_step")
	res = runCLI(t, "catalog", "list")
	log.Assert("list_ok", res.Code == 0, 0, res.Code)
	log.Step("list", testutil.OutcomeOK, "units="+itoa(len(nonEmptyLines(res.Stdout))))
	log.PhaseEnd("list_step", testutil.OutcomeOK)
}

// TestCatalogHelpListShowGolden updates/locks list and show help when present
// under catalog --help (parent) and list/show --help.
func TestCatalogHelpListShow(t *testing.T) {
	log := testutil.New(t)
	log.Phase("help")
	for _, args := range [][]string{
		{"catalog", "list", "--help"},
		{"catalog", "show", "--help"},
	} {
		name := strings.Join(args[:2], "_") // catalog_list / catalog_show
		res := runCLI(t, args...)
		log.Assert(name+"_0", res.Code == 0, 0, res.Code)
		got := normalizeHelp(res.Stdout)
		path := testutil.GoldenPath("testdata", "help_"+name)
		testutil.CompareGolden(t, path, []byte(got))
		log.Step("golden_"+name, testutil.OutcomeOK, path)
		// Absolute host homes must never appear.
		log.Assert(name+"_no_home", !strings.Contains(got, "/home/"), false, strings.Contains(got, "/home/"))
	}
	// Parent catalog help golden already covered by TestHelpGoldens; re-check.
	res := runCLI(t, "catalog", "--help")
	got := normalizeHelp(res.Stdout)
	path := filepath.Join("testdata", "help_catalog.golden")
	testutil.CompareGolden(t, path, []byte(got))
	log.Step("parent_help", testutil.OutcomeOK, path)
	log.PhaseEnd("help", testutil.OutcomeOK)
}

// normalizeCatalogText trims trailing whitespace per line and ensures a trailing LF.
func normalizeCatalogText(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(ln, " \t")
	}
	out := strings.Join(lines, "\n")
	return strings.TrimRight(out, "\n") + "\n"
}

// normalizeCatalogJSON re-encodes JSON with stable indentation for goldens.
// Uses compact single-line form as emitted by the CLI (already deterministic
// via struct field order); only normalizes trailing newline.
func normalizeCatalogJSON(t *testing.T, s string) string {
	t.Helper()
	s = strings.TrimSpace(s)
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("normalizeCatalogJSON: %v\n%s", err, s)
	}
	// Re-marshal compact to drop any accidental whitespace drift; field order
	// for maps is sorted by encoding/json, but success envelopes use structs
	// so original order is preferred — keep original if re-parse succeeds.
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("remarshal: %v", err)
	}
	// Prefer original CLI output when it is already valid compact JSON with
	// trailing newline contract — use original for golden fidelity of key order.
	_ = b
	return strings.TrimRight(s, "\n") + "\n"
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, ln := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if strings.TrimSpace(ln) != "" {
			out = append(out, ln)
		}
	}
	return out
}
