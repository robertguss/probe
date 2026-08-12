package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func repoRootForTest(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestRunVersion(t *testing.T) {
	log := testutil.New(t)
	log.Phase("run_version")
	var out, errB bytes.Buffer

	code := run(context.Background(), []string{"version"}, cli.Streams{
		In: strings.NewReader(""), Out: &out, Err: &errB,
	})

	log.Assert("exit_0", code == diagnostic.ExitSuccess, diagnostic.ExitSuccess, code)
	log.Assert("stdout_has_version", strings.Contains(out.String(), "foundry ") || strings.Contains(out.String(), "version"),
		true, out.String())
	log.Assert("stderr_empty", errB.Len() == 0, true, errB.String())
	log.PhaseEnd("run_version", testutil.OutcomeOK)
}

func TestRunValidateExample(t *testing.T) {
	log := testutil.New(t)
	log.Phase("run_validate")
	spec := filepath.Join(repoRootForTest(t), "examples", "minimal-cli.toml")
	if _, err := os.Stat(spec); err != nil {
		t.Skip(err)
	}
	var out, errB bytes.Buffer

	code := run(context.Background(), []string{"validate", "--spec", spec}, cli.Streams{
		In: strings.NewReader(""), Out: &out, Err: &errB,
	})

	log.Assert("exit_0", code == diagnostic.ExitSuccess, diagnostic.ExitSuccess, code)
	log.Assert("ok", strings.Contains(out.String(), "validate: ok") || strings.Contains(out.String(), "plan_sha256"),
		true, out.String())
	log.PhaseEnd("run_validate", testutil.OutcomeOK)
}

// TestRun_ExitCodeMatrix exercises run's IO-injected entry point across the
// documented exit-code contract without spawning a process (bead
// go-foundry-cli-wet.2.5): success, usage errors (bad flag / invalid spec),
// catalog-class failure, and pre-cancelled context.
func TestRun_ExitCodeMatrix(t *testing.T) {
	repo := repoRootForTest(t)
	spec := filepath.Join(repo, "examples", "minimal-cli.toml")
	if _, err := os.Stat(spec); err != nil {
		t.Skip(err)
	}

	cases := []struct {
		name    string
		ctx     func() (context.Context, context.CancelFunc)
		args    []string
		want    int
		wantErr string // substring expected somewhere in stdout+stderr; empty skips check
	}{
		{
			name: "success_version",
			args: []string{"version"},
			want: diagnostic.ExitSuccess,
		},
		{
			name:    "usage_unknown_flag",
			args:    []string{"version", "--not-a-real-flag"},
			want:    diagnostic.ExitUsage,
			wantErr: "usage.invalid",
		},
		{
			name:    "usage_conflicting_flags",
			args:    []string{"version", "--quiet", "--verbose"},
			want:    diagnostic.ExitUsage,
			wantErr: "usage.invalid",
		},
		{
			name:    "usage_invalid_spec",
			args:    []string{"validate", "--spec", "does-not-exist.toml"},
			want:    diagnostic.ExitUsage,
			wantErr: "usage.invalid",
		},
		{
			name:    "catalog_unknown_unit",
			args:    []string{"catalog", "show", "does-not-exist-unit"},
			want:    diagnostic.ExitFailure,
			wantErr: "catalog.invalid",
		},
		{
			name: "cancelled_before_dispatch",
			ctx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, func() {}
			},
			args: []string{"version"},
			want: diagnostic.ExitCancelled,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			log := testutil.New(t)
			log.Phase(tc.name)
			log.Inputs(map[string]string{"args": strings.Join(tc.args, " ")})

			ctx := context.Background()
			if tc.ctx != nil {
				var cancel context.CancelFunc
				ctx, cancel = tc.ctx()
				defer cancel()
			}

			var out, errB bytes.Buffer
			code := run(ctx, tc.args, cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errB})

			log.Step("run", testutil.OutcomeOK, "exit="+strconv.Itoa(code))
			log.Assert("exit_code", code == tc.want, tc.want, code)
			if tc.wantErr != "" {
				combined := out.String() + errB.String()
				log.Assert("error_id", strings.Contains(combined, tc.wantErr), tc.wantErr, combined)
			}
			log.PhaseEnd(tc.name, testutil.OutcomeOK)
		})
	}
}

func TestInstallDrainedSIGPIPE(t *testing.T) {
	// Idempotent install must not panic.
	installDrainedSIGPIPE()
	installDrainedSIGPIPE()
}
