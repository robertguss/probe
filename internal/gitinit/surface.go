package gitinit

import "strings"

// StepID is the plan external_steps id for isolated git init (Appendix E).
const StepID = "git-init"

// ForbiddenGitSubcommands are Git verbs the Foundry must never invoke
// (Section 39 / REQ-160 / DEC-004). Process-tree audits reject any argv
// whose subcommand is in this set.
var ForbiddenGitSubcommands = []string{
	"commit",
	"add",
	"push",
	"pull",
	"fetch",
	"clone",
	"remote",
	"tag",
	"checkout",
	"branch", // except via --initial-branch= on init
	"config",
	"rebase",
	"merge",
	"stash",
	"submodule",
	"hook",
	"alias",
	"credential",
	"send-pack",
	"receive-pack",
	"request-pull",
	"release", // not a git verb but product surface ban
}

// OnlyGitSubcommand is the sole permitted Git subcommand.
const OnlyGitSubcommand = "init"

// PlannedArgs returns argv[1:] for the isolated git init invocation
// (Section 34.2 / 39). Binary basename "git" is separate (toolrun StepRequest).
//
//	init --initial-branch=<branch> --template=<scratch> .
func PlannedArgs(branch, templateDir string) []string {
	if branch == "" {
		branch = "main"
	}
	return []string{
		"init",
		"--initial-branch=" + branch,
		"--template=" + templateDir,
		".",
	}
}

// PlannedArgv returns the full argv including the "git" basename for logging
// and process-tree comparison (never absolute host homes in the binary slot).
func PlannedArgv(branch, templateDir string) []string {
	args := PlannedArgs(branch, templateDir)
	out := make([]string, 0, 1+len(args))
	out = append(out, "git")
	out = append(out, args...)
	return out
}

// SubcommandOf extracts the git subcommand from argv (argv[0] may be "git"
// or an absolute path). Returns "" when no subcommand is present.
func SubcommandOf(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	// Skip leading binary.
	i := 0
	if base := argv[0]; base == "git" || strings.HasSuffix(base, "/git") {
		i = 1
	}
	for ; i < len(argv); i++ {
		a := argv[i]
		if a == "" || strings.HasPrefix(a, "-") {
			continue
		}
		return a
	}
	return ""
}

// IsForbiddenSubcommand reports whether sub is in ForbiddenGitSubcommands.
func IsForbiddenSubcommand(sub string) bool {
	for _, f := range ForbiddenGitSubcommands {
		if sub == f {
			return true
		}
	}
	return false
}

// AssertOnlyInit returns an error detail if argv is not exactly the planned
// isolated init shape (process-tree unit helper).
func AssertOnlyInit(argv []string, branch, templateDir string) string {
	want := PlannedArgv(branch, templateDir)
	if len(argv) != len(want) {
		return "argv length mismatch"
	}
	for i := range want {
		if argv[i] != want[i] {
			// Template path may differ only when caller passes absolute vs relative;
			// require exact match for process-tree equality.
			return "argv mismatch at " + itoa(i)
		}
	}
	sub := SubcommandOf(argv)
	if sub != OnlyGitSubcommand {
		return "subcommand " + sub + " != init"
	}
	if IsForbiddenSubcommand(sub) {
		return "forbidden subcommand " + sub
	}
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
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
