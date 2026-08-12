package cli_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestFlagMatrixQuietVerbose(t *testing.T) {
	log := testutil.New(t)
	log.Phase("quiet_verbose_mutex")

	cases := []struct {
		name string
		args []string
		want int
	}{
		{name: "quiet_ok", args: []string{"version", "--quiet"}, want: 0},
		{name: "verbose_ok", args: []string{"version", "--verbose"}, want: 0},
		{name: "both_reject", args: []string{"version", "--quiet", "--verbose"}, want: diagnostic.ExitUsage},
		{name: "both_reverse", args: []string{"version", "--verbose", "--quiet"}, want: diagnostic.ExitUsage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub := testutil.New(t)
			sub.Inputs(map[string]string{"argv": strings.Join(tc.args, " ")})
			res := runCLI(t, tc.args...)
			sub.Subprocess("run", tc.args, "-", res.Code, len(res.Stdout), len(res.Stderr),
				firstLine(res.Stdout+res.Stderr), lastLine(res.Stdout+res.Stderr), res.Code != tc.want)
			sub.Assert("exit", res.Code == tc.want, tc.want, res.Code)
			if tc.want == diagnostic.ExitUsage {
				sub.Assert("stderr_or_usage", res.Stderr != "" || res.Stdout != "", true, res.Stderr != "")
				combined := res.Stderr + res.Stdout
				sub.Assert("mentions_quiet_or_verbose",
					strings.Contains(combined, "quiet") || strings.Contains(combined, "verbose") ||
						strings.Contains(combined, "usage.invalid"),
					true, combined)
			}
		})
	}
	log.PhaseEnd("quiet_verbose_mutex", testutil.OutcomeOK)
}

func TestFlagMatrixColor(t *testing.T) {
	log := testutil.New(t)
	log.Phase("color_values")
	for _, c := range []string{"auto", "always", "never"} {
		args := []string{"version", "--color", c}
		res := runCLI(t, args...)
		log.Step("color_"+c, testutil.OutcomeOK, "exit="+itoa(res.Code))
		log.Assert("exit_"+c, res.Code == 0, 0, res.Code)
	}
	res := runCLI(t, "version", "--color", "rainbow")
	log.Assert("invalid_color_exit", res.Code == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Code)
	log.PhaseEnd("color_values", testutil.OutcomeOK)
}

func TestJSONRejectsHumanFlags(t *testing.T) {
	log := testutil.New(t)
	log.Phase("json_human_reject")

	cases := []struct {
		name string
		args []string
	}{
		{name: "json_quiet", args: []string{"version", "--output", "json", "--quiet"}},
		{name: "json_verbose", args: []string{"version", "--output", "json", "--verbose"}},
		{name: "json_color_auto", args: []string{"version", "--output", "json", "--color", "auto"}},
		{name: "json_color_never", args: []string{"version", "--output", "json", "--color", "never"}},
		{name: "json_color_always", args: []string{"version", "--output", "json", "--color", "always"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub := testutil.New(t)
			sub.Inputs(map[string]string{"argv": strings.Join(tc.args, " ")})
			res := runCLI(t, tc.args...)
			sub.Subprocess("run", tc.args, "-", res.Code, len(res.Stdout), len(res.Stderr),
				firstLine(res.Stderr), lastLine(res.Stderr), res.Code != diagnostic.ExitUsage)
			sub.Assert("exit_2", res.Code == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Code)
			// JSON mode established → envelope on stdout, empty stderr (Section 37).
			sub.Assert("stderr_empty", res.Stderr == "", "", res.Stderr)
			sub.Assert("stdout_json", strings.Contains(res.Stdout, `"ok":false`) || strings.Contains(res.Stdout, "usage.invalid"),
				true, res.Stdout)
			sub.Assert("error_id", strings.Contains(res.Stdout, "usage.invalid"), true, res.Stdout)
		})
	}
	// JSON without human flags is fine.
	res := runCLI(t, "version", "--output", "json")
	log.Assert("json_alone_ok", res.Code == 0, 0, res.Code)
	log.Assert("json_stdout", strings.Contains(res.Stdout, `"ok":true`), true, res.Stdout)
	log.Assert("json_stderr_empty", res.Stderr == "", "", res.Stderr)
	log.PhaseEnd("json_human_reject", testutil.OutcomeOK)
}

func TestNoBannedFlagsRegistered(t *testing.T) {
	log := testutil.New(t)
	log.Phase("banned_flags")
	root := cli.NewRoot(cli.Options{SkipCatalogLoad: true})

	registered := map[string]bool{}
	for _, n := range cli.CollectFlagNames(root) {
		registered[n] = true
		log.Step("flag_"+n, testutil.OutcomeInfo, n)
	}
	for _, banned := range cli.BannedFlagNames() {
		log.Assert("not_registered_"+banned, !registered[banned], false, registered[banned])
	}

	// Attempt to use banned flag tokens — must be unknown flag (exit 2).
	for _, tok := range bannedSurfaceTokens() {
		name := strings.TrimPrefix(tok, "--")
		res := runCLI(t, "version", tok)
		log.Step("try_"+name, testutil.OutcomeOK, "exit="+itoa(res.Code))
		log.Assert("reject_"+name, res.Code == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Code)
	}
	log.PhaseEnd("banned_flags", testutil.OutcomeOK)
}

func TestInvalidOutputMode(t *testing.T) {
	log := testutil.New(t)
	res := runCLI(t, "version", "--output", "yaml")
	log.Assert("exit", res.Code == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Code)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [16]byte
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
