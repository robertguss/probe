package generatee2e_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestGenerateConcurrentDestinationConflict runs two generates targeting the
// same destination concurrently. Exactly one must succeed and commit; the other
// must fail deterministically without overwriting the committed destination
// (bead go-foundry-cli-wet.3.5).
func TestGenerateConcurrentDestinationConflict(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("concurrent_dest_conflict")

	parent := privateParent(t)
	dest := filepath.Join(parent, "minimal-cli")
	spec := examplesSpec(t, "minimal-cli.toml")

	const n = 2
	var wg sync.WaitGroup
	wg.Add(n)
	results := make([]proc, n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = runCLI(t, context.Background(), cli.Options{}, nil,
				"generate", "--spec", spec, "--dest", dest, "--output", "json")
		}(i)
	}
	wg.Wait()

	successes := 0
	failures := 0
	for i, res := range results {
		log.Subprocess("generate_"+itoa(i), res.Args, "-", res.Code, len(res.Stdout), len(res.Stderr),
			firstLine(res.Stdout+res.Stderr), lastLine(res.Stdout+res.Stderr), false)
		log.Step("generate_"+itoa(i)+"_timing", testutil.OutcomeInfo,
			"elapsed_ms="+itoa(int(res.Duration.Milliseconds())))
		if res.Code == 0 {
			successes++
			if commitOutcomeFromEnvelope(mustEnvelope(t, log, res.Stdout)) != "committed" {
				t.Fatalf("successful run did not commit: %s", res.Stdout)
			}
		} else {
			failures++
			env := mustEnvelope(t, log, res.Stdout)
			id := errorIDFromEnvelope(env)
			if id != "fs.destination_exists" && id != "fs.parent_conflict" && !strings.Contains(res.Stderr, "destination") && !strings.Contains(res.Stderr, "parent") {
				t.Fatalf("unexpected concurrent failure error id: %q stderr=%q", id, res.Stderr)
			}
		}
	}
	log.Assert("one_success", successes == 1, 1, successes)
	log.Assert("one_failure", failures == 1, 1, failures)

	if _, err := os.Stat(filepath.Join(dest, "go.mod")); err != nil {
		log.Fail("committed_dest_intact", err.Error())
	}
	log.PhaseEnd("concurrent_dest_conflict", testutil.OutcomeOK)
}

// TestGenerateSpecMutationDetected plans with the original spec, mutates a
// non-identity field (description), and verifies the plan digest changes.
// A subsequent generate into the existing destination is refused with
// fs.destination_exists, proving the mutation is harmless but overwrite is
// still forbidden (bead go-foundry-cli-wet.3.5).
func TestGenerateSpecMutationDetected(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("spec_mutation_detected")

	parent := privateParent(t)
	dest := filepath.Join(parent, "minimal-cli")
	spec := examplesSpec(t, "minimal-cli.toml")

	res1 := runCLI(t, context.Background(), cli.Options{}, nil,
		"generate", "--spec", spec, "--dest", dest, "--output", "json")
	logProc(log, "generate_original", res1, 0)
	shaOrig := planSHAFromEnvelope(mustEnvelope(t, log, res1.Stdout))

	body, err := os.ReadFile(spec)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	mutated := strings.ReplaceAll(string(body), "Minimal Foundry CLI example for validate, plan, and generate", "Mutated description for e2e")
	mutatedPath := filepath.Join(parent, "mutated.toml")
	if err := os.WriteFile(mutatedPath, []byte(mutated), 0o644); err != nil {
		t.Fatalf("write mutated spec: %v", err)
	}

	resPlan := runCLI(t, context.Background(), cli.Options{}, nil,
		"plan", "--spec", mutatedPath, "--dest", dest, "--output", "json")
	logProc(log, "plan_mutated", resPlan, 0)
	shaMut := planSHAFromEnvelope(mustEnvelope(t, log, resPlan.Stdout))
	log.Assert("plan_digest_changes", shaMut != shaOrig, true, shaMut != shaOrig)

	res2 := runCLI(t, context.Background(), cli.Options{}, nil,
		"generate", "--spec", mutatedPath, "--dest", dest, "--output", "json")
	logProc(log, "generate_mutated", res2, diagnostic.ExitUsage)
	env := mustEnvelope(t, log, res2.Stdout)
	id := errorIDFromEnvelope(env)
	log.Assert("error_id", id == "fs.destination_exists", "fs.destination_exists", id)

	if _, err := os.Stat(filepath.Join(dest, "go.mod")); err != nil {
		log.Fail("dest_intact", err.Error())
	}
	log.PhaseEnd("spec_mutation_detected", testutil.OutcomeOK)
}

// TestGenerateConcurrentSameParentDistinctBasenames runs several generates
// that share a parent directory but use distinct project names (basename must
// equal name) — all must succeed with no stage debris — plus one concurrent
// conflict against an already-claimed basename (bead go-foundry-cli-n0y.6).
func TestGenerateConcurrentSameParentDistinctBasenames(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("concurrent_same_parent_distinct")

	parent := privateParent(t)

	// Per-name specs: destination basename must equal the project name field.
	writeNamedSpec := func(name string) string {
		t.Helper()
		body := fmt.Sprintf(`schema = 1
name = %q
module = "github.com/example/%s"
description = "concurrent same-parent fixture"
archetype = "cli"
destination = "./%s"
profiles = []
[git]
init = false
`, name, name, name)
		path := filepath.Join(parent, name+"-spec.toml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write spec: %v", err)
		}
		return path
	}

	// Seed one claimed project so a later concurrent attempt conflicts.
	claimedName := "claimedcli"
	claimedSpec := writeNamedSpec(claimedName)
	claimed := filepath.Join(parent, claimedName)
	res0 := runCLI(t, context.Background(), cli.Options{}, nil,
		"generate", "--spec", claimedSpec, "--dest", claimed, "--output", "json")
	logProc(log, "seed_claimed", res0, 0)

	type job struct {
		name string
		spec string
		dest string
	}
	jobs := []job{
		{name: "cella", spec: writeNamedSpec("cella"), dest: filepath.Join(parent, "cella")},
		{name: "cellb", spec: writeNamedSpec("cellb"), dest: filepath.Join(parent, "cellb")},
		{name: "cellc", spec: writeNamedSpec("cellc"), dest: filepath.Join(parent, "cellc")},
		{name: "conflict", spec: claimedSpec, dest: claimed},
	}

	var wg sync.WaitGroup
	wg.Add(len(jobs))
	results := make([]proc, len(jobs))
	for i, j := range jobs {
		go func(idx int, j job) {
			defer wg.Done()
			results[idx] = runCLI(t, context.Background(), cli.Options{}, nil,
				"generate", "--spec", j.spec, "--dest", j.dest, "--output", "json")
		}(i, j)
	}
	wg.Wait()

	for i, j := range jobs {
		res := results[i]
		log.Subprocess("job_"+j.name, res.Args, "-", res.Code, len(res.Stdout), len(res.Stderr),
			firstLine(res.Stdout+res.Stderr), lastLine(res.Stdout+res.Stderr), false)
		if j.name == "conflict" {
			log.Assert(j.name+"_fail", res.Code != 0, true, res.Code != 0)
			if res.Code == 0 {
				t.Fatalf("conflict job unexpectedly succeeded")
			}
			env := mustEnvelope(t, log, res.Stdout)
			id := errorIDFromEnvelope(env)
			if id != "fs.destination_exists" && id != "fs.parent_conflict" {
				if id != "" && !strings.Contains(res.Stdout+res.Stderr, "destination") {
					t.Fatalf("unexpected conflict error id: %q", id)
				}
			}
			continue
		}
		log.Assert(j.name+"_ok", res.Code == 0, 0, res.Code)
		if res.Code != 0 {
			t.Fatalf("distinct basename %s failed: code=%d out=%s", j.name, res.Code, res.Stdout)
		}
		if _, err := os.Stat(filepath.Join(j.dest, "go.mod")); err != nil {
			log.Fail(j.name+"_gomod", err.Error())
		}
	}

	// No leftover stage directories under the shared parent.
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("readdir parent: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".foundry-stage") {
			t.Fatalf("stage debris left in parent: %s", e.Name())
		}
	}

	if _, err := os.Stat(filepath.Join(claimed, "go.mod")); err != nil {
		log.Fail("claimed_intact", err.Error())
	}
	log.PhaseEnd("concurrent_same_parent_distinct", testutil.OutcomeOK)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	bp := len(b)
	for i > 0 {
		bp--
		b[bp] = byte('0' + i%10)
		i /= 10
	}
	return string(b[bp:])
}
