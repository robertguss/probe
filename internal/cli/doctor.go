package cli

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/toolrun"
	"github.com/spf13/cobra"
)

// doctorResult is the stable JSON payload for foundry doctor.
type doctorResult struct {
	CatalogGoPin     string `json:"catalog_go_pin"`
	FoundryGo        string `json:"foundry_go"`
	FoundryGoBin     string `json:"foundry_go_bin"`  // env value or ""
	PathGo           string `json:"path_go"`         // LookPath result or ""
	PinnedGoFound    string `json:"pinned_go_found"` // absolute pin path when discoverable
	Status           string `json:"status"`          // ok | warn
	Guidance         string `json:"guidance"`
	MatchForGenerate bool   `json:"match_for_generate"`
}

func newDoctorCmd(r *root) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check Go toolchain pin and FOUNDRY_GO_BIN guidance (advisory)",
		Long: strings.TrimSpace(`
doctor reports the catalog Go pin, this Foundry binary's runtime Go version,
FOUNDRY_GO_BIN, PATH's go (LookPath), and whether a pinned go1.x binary is
discoverable for generate. It is advisory: exit 0 even on warnings so scripts
can print guidance without failing validate/plan.

doctor may probe candidate go binaries with a closed GOTOOLCHAIN=local
environment (the same probe generate uses). It does not write files or use
the network.

Examples:
  foundry doctor
  foundry doctor --output json
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkCancelled(cmd.Context()); err != nil {
				return err
			}
			res := r.runDoctor()
			return r.emitTextOrJSON(cmd, "doctor", res, formatDoctorText(res))
		},
	}
}

func (r *root) runDoctor() doctorResult {
	pin := toolrun.DefaultPinnedGoTag
	info := r.versionInfo()
	envBin := strings.TrimSpace(os.Getenv(toolrun.EnvFoundryGoBin))
	pathGo, _ := exec.LookPath("go")
	// Same discovery generate uses (may probe candidates under GOTOOLCHAIN=local).
	pinned := toolrun.FindPinnedGoBinary(pin)
	match := pinned != ""

	status := "ok"
	guidance := fmt.Sprintf("generate can resolve catalog pin %s", pin)
	switch {
	case match && envBin != "" && pinned == envBin:
		guidance = fmt.Sprintf("FOUNDRY_GO_BIN points at pin %s (%s)", pin, pinned)
	case match && envBin == "":
		guidance = fmt.Sprintf("pinned go found at %s; optional: export FOUNDRY_GO_BIN=%s", pinned, pinned)
	case !match:
		status = "warn"
		guidance = toolrun.WrongVersionRemediation(pin, pathGo, "")
	}

	return doctorResult{
		CatalogGoPin:     pin,
		FoundryGo:        info.Go,
		FoundryGoBin:     envBin,
		PathGo:           pathGo,
		PinnedGoFound:    pinned,
		Status:           status,
		Guidance:         guidance,
		MatchForGenerate: match,
	}
}

func formatDoctorText(r doctorResult) string {
	envLabel := "(unset)"
	if r.FoundryGoBin != "" {
		envLabel = r.FoundryGoBin
	}
	pathLabel := "(not found)"
	if r.PathGo != "" {
		pathLabel = r.PathGo
	}
	pinFound := "(none)"
	if r.PinnedGoFound != "" {
		pinFound = r.PinnedGoFound
	}
	var b strings.Builder
	fmt.Fprintf(&b, "doctor: status=%s match_for_generate=%v\n", r.Status, r.MatchForGenerate)
	fmt.Fprintf(&b, "catalog_go_pin: %s\n", r.CatalogGoPin)
	fmt.Fprintf(&b, "foundry_go: %s\n", r.FoundryGo)
	fmt.Fprintf(&b, "runtime: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "FOUNDRY_GO_BIN: %s\n", envLabel)
	fmt.Fprintf(&b, "path_go: %s\n", pathLabel)
	fmt.Fprintf(&b, "pinned_go_found: %s\n", pinFound)
	fmt.Fprintf(&b, "guidance: %s", r.Guidance)
	return b.String()
}
