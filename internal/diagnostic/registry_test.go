package diagnostic_test

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestRegistryUniqueCompleteRedact is the multi-step harness case required by
// P1.1: load registry → assert unique → assert complete → assert redaction matrix.
func TestRegistryUniqueCompleteRedact(t *testing.T) {
	log := testutil.New(t)

	log.Phase("load_registry")
	entries := diagnostic.Entries()
	ids := diagnostic.AllIdentifiers()
	log.Inputs(map[string]string{
		"entry_count":      itoa(len(entries)),
		"identifier_count": itoa(len(ids)),
	})
	log.Assert("entries_nonempty", len(entries) > 0, ">0", len(entries))
	log.Assert("ids_match_entries", len(entries) == len(ids), len(ids), len(entries))
	log.PhaseEnd("load_registry", testutil.OutcomeOK)

	log.Phase("assert_unique")
	seen := make(map[diagnostic.Identifier]int, len(entries))
	var dups []string
	for i, e := range entries {
		seen[e.ID]++
		if seen[e.ID] > 1 {
			dups = append(dups, string(e.ID))
		}
		// Every entry must have actionable remediation (no stack-dump primary).
		log.Assert("remediation_nonempty_"+string(e.ID), e.Remediation != "", "nonempty", truncate(e.Remediation, 40))
		log.Assert("meaning_nonempty_"+string(e.ID), e.Meaning != "", "nonempty", e.Meaning)
		log.Assert("exit_valid_"+string(e.ID),
			e.ExitCode == diagnostic.ExitFailure || e.ExitCode == diagnostic.ExitUsage,
			"1|2", e.ExitCode)
		_ = i
	}
	if len(dups) > 0 {
		sort.Strings(dups)
		log.Fail("unique", "duplicate IDs (sorted): "+strings.Join(dups, ", "))
	}
	log.Assert("unique_count", len(seen) == len(entries), len(entries), len(seen))
	log.PhaseEnd("assert_unique", testutil.OutcomeOK)

	log.Phase("assert_complete")
	want := loadAppendixD(t)
	log.Fixture("appendix_d_ids", "testdata/appendix_d_ids.txt count="+itoa(len(want)))
	missing := diagnostic.MissingIDs(want)
	extra := diagnostic.ExtraIDs(want)
	if len(missing) > 0 {
		// On registry gap: print missing IDs sorted (acceptance logging).
		sorted := make([]string, len(missing))
		for i, id := range missing {
			sorted[i] = string(id)
		}
		log.Fail("complete_missing", "missing IDs (sorted): "+strings.Join(sorted, ", "))
	}
	if len(extra) > 0 {
		sorted := make([]string, len(extra))
		for i, id := range extra {
			sorted[i] = string(id)
		}
		log.Fail("complete_extra", "extra IDs (sorted): "+strings.Join(sorted, ", "))
	}
	log.Assert("complete_vs_testdata", len(missing) == 0 && len(extra) == 0, len(want), len(entries))
	// Completeness vs constant list.
	constMissing := diagnostic.MissingIDs(ids)
	log.Assert("complete_vs_constants", len(constMissing) == 0, 0, len(constMissing))
	log.PhaseEnd("assert_complete", testutil.OutcomeOK)

	log.Phase("assert_redaction_matrix")
	matrix := []struct {
		name string
		in   string
		hide string
	}{
		{"password_assign", "DB_PASSWORD=s3cr3t-value", "s3cr3t-value"},
		{"token_assign", "GITHUB_TOKEN=ghp_xxxdeadbeef", "ghp_xxxdeadbeef"},
		{"secret_assign", "APP_SECRET=topsecret", "topsecret"},
		{"proxy_userinfo", "https://user:secretpass@proxy.example:8080", "secretpass"},
		{"bearer", "Authorization: Bearer super-token-xyz", "super-token-xyz"},
		{"key_value_text", "password=hunter2 ok=1", "hunter2"},
	}
	for _, tc := range matrix {
		out := diagnostic.Redact(tc.in)
		okHide := !strings.Contains(out, tc.hide)
		okSentinel := strings.Contains(out, diagnostic.RedactedSentinel)
		if !okHide {
			// On redaction failure: print key name only (never values).
			log.Step("redact_fail_key", testutil.OutcomeFail, "key_or_case="+tc.name)
		}
		log.Assert("redact_hides_"+tc.name, okHide, "hidden", "see_key="+tc.name)
		log.Assert("redact_sentinel_"+tc.name, okSentinel, diagnostic.RedactedSentinel, truncate(out, 80))
	}
	// Env map form.
	env := map[string]string{
		"GITHUB_TOKEN": "tok_should_not_leak",
		"PATH":         "/usr/bin",
		"HTTPS_PROXY":  "http://alice:secretpass@proxy:3128",
	}
	red := diagnostic.RedactEnv(env)
	log.Assert("env_token_sentinel", red["GITHUB_TOKEN"] == diagnostic.RedactedSentinel,
		diagnostic.RedactedSentinel, red["GITHUB_TOKEN"])
	log.Assert("env_path_intact", red["PATH"] == "/usr/bin", "/usr/bin", red["PATH"])
	log.Assert("env_proxy_no_pass", !strings.Contains(red["HTTPS_PROXY"], "secretpass"),
		"no secretpass", red["HTTPS_PROXY"])
	log.PhaseEnd("assert_redaction_matrix", testutil.OutcomeOK)
}

func TestRegistryUniquenessTable(t *testing.T) {
	entries := diagnostic.Entries()
	seen := map[string]struct{}{}
	for _, e := range entries {
		id := string(e.ID)
		if _, ok := seen[id]; ok {
			t.Errorf("duplicate identifier %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestRegistryReflectionOverConstants(t *testing.T) {
	// Table + "reflection": every exported ID* constant must be registered.
	// We walk AllIdentifiers (built from the same constants) and Lookup.
	for _, id := range diagnostic.AllIdentifiers() {
		e, ok := diagnostic.Lookup(id)
		if !ok {
			t.Errorf("constant %q not in registry", id)
			continue
		}
		if e.ID != id {
			t.Errorf("Lookup(%q).ID = %q", id, e.ID)
		}
		if !diagnostic.Known(id) {
			t.Errorf("Known(%q) = false", id)
		}
	}
}

func TestRegistryDumpDeterminism(t *testing.T) {
	// -count=2 will run this twice in separate processes; within one process
	// we also dump twice and compare.
	a := dumpRegistry()
	b := dumpRegistry()
	if a != b {
		t.Fatalf("registry dump not deterministic:\n---a---\n%s\n---b---\n%s", a, b)
	}
}

func TestRegistryConcurrentLookup(t *testing.T) {
	// No package-level mutable maps after init: concurrent readers under race.
	ids := diagnostic.AllIdentifiers()
	var wg sync.WaitGroup
	errCh := make(chan string, runtime.GOMAXPROCS(0)*4)
	for i := 0; i < runtime.GOMAXPROCS(0)*4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 200; n++ {
				id := ids[n%len(ids)]
				e, ok := diagnostic.Lookup(id)
				if !ok || e.ID != id {
					errCh <- string(id)
					return
				}
				_ = diagnostic.Entries()
				_ = diagnostic.ExitCodeFor(id)
				_ = diagnostic.RemediationFor(id)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for id := range errCh {
		t.Errorf("concurrent lookup failed for %s", id)
	}
}

func TestRemediationAgentLegible(t *testing.T) {
	// Remediation must say what failed / how to fix — not stack dumps.
	banned := []string{"goroutine ", "runtime/debug", "stack trace", "panic:"}
	for _, e := range diagnostic.Entries() {
		r := strings.ToLower(e.Remediation)
		if len(e.Remediation) < 20 {
			t.Errorf("%s: remediation too short for agents: %q", e.ID, e.Remediation)
		}
		for _, b := range banned {
			if strings.Contains(r, strings.ToLower(b)) {
				t.Errorf("%s: remediation must not contain %q", e.ID, b)
			}
		}
	}
}

func dumpRegistry() string {
	var b strings.Builder
	for _, e := range diagnostic.Entries() {
		b.WriteString(string(e.ID))
		b.WriteByte('\t')
		b.WriteString(itoa(e.ExitCode))
		b.WriteByte('\t')
		b.WriteString(e.Meaning)
		b.WriteByte('\n')
	}
	return b.String()
}

func loadAppendixD(t *testing.T) []diagnostic.Identifier {
	t.Helper()
	path := filepath.Join("testdata", "appendix_d_ids.txt")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	var out []diagnostic.Identifier
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, diagnostic.Identifier(line))
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("appendix_d_ids.txt empty")
	}
	return out
}
