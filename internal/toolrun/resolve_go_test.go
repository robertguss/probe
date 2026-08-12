package toolrun_test

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

func TestWrongVersionRemediation_NamesNearbyPin(t *testing.T) {
	log := testutil.New(t)
	obs := "/home/x/.local/share/mise/installs/go/1.26.4/bin/go"
	pin := "/home/x/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.5.linux-amd64/bin/go"
	msg := toolrun.WrongVersionRemediation("go1.26.5", obs, pin)
	log.Inputs(map[string]string{"remediation": msg})
	if !strings.Contains(msg, pin) {
		t.Fatalf("remediation must name nearby pin: %s", msg)
	}
	if !strings.Contains(msg, toolrun.EnvFoundryGoBin) {
		t.Fatalf("remediation must name %s: %s", toolrun.EnvFoundryGoBin, msg)
	}
	if !strings.Contains(msg, "go1.26.5") {
		t.Fatalf("remediation must name required tag: %s", msg)
	}
	log.Step("nearby_hint", testutil.OutcomeOK, "named_pin")
}

func TestWrongVersionRemediation_NoNearby(t *testing.T) {
	msg := toolrun.WrongVersionRemediation("go1.26.5", "/usr/bin/go", "")
	if !strings.Contains(msg, toolrun.EnvFoundryGoBin) {
		t.Fatalf("want FOUNDRY_GO_BIN guidance: %s", msg)
	}
	if strings.Contains(msg, "A matching binary was found") {
		t.Fatalf("must not claim a nearby pin: %s", msg)
	}
}

func TestWrongVersionRemediation_SamePath(t *testing.T) {
	p := "/opt/go/bin/go"
	msg := toolrun.WrongVersionRemediation("go1.26.5", p, p)
	if strings.Contains(msg, "A matching binary was found") {
		t.Fatalf("same path is not a useful hint: %s", msg)
	}
}

func TestProbeGoVersionLocal_FakeRunner(t *testing.T) {
	log := testutil.New(t)
	goBin := "/fake/go"
	fake := &toolrun.FakeRunner{
		PathMap: map[string]string{goBin: goBin},
		Outputs: map[string]string{goBin: "go version go1.26.5 linux/amd64\n"},
	}
	tag, err := toolrun.ProbeGoVersionLocalWithRunner(context.Background(), goBin, fake)
	if err != nil {
		t.Fatal(err)
	}
	log.Assert("tag", tag == "go1.26.5", "go1.26.5", tag)
}

func TestFindPinnedGoBinary_FromFOUNDRY_GO_BIN_FakeRunner(t *testing.T) {
	log := testutil.New(t)
	goBin := "/fake/from-env/go"
	t.Setenv(toolrun.EnvFoundryGoBin, goBin)
	t.Setenv("GOMODCACHE", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", "/bin:/usr/bin")
	fake := &toolrun.FakeRunner{
		PathMap: map[string]string{goBin: goBin},
		Outputs: map[string]string{goBin: "go version go1.26.5 linux/amd64\n"},
	}

	got := toolrun.FindPinnedGoBinaryWithRunner("go1.26.5", fake)
	log.Assert("found", got == goBin, goBin, got)
}

func TestFindPinnedGoBinary_ModuleCacheLayout_FakeRunner(t *testing.T) {
	log := testutil.New(t)
	modcache := t.TempDir()
	name := "golang.org/toolchain@v0.0.1-go1.26.5." + runtime.GOOS + "-" + runtime.GOARCH
	goBin := filepath.Join(modcache, name, "bin", "go")
	t.Setenv(toolrun.EnvFoundryGoBin, "")
	t.Setenv("GOMODCACHE", modcache)
	// Empty HOME so $HOME/sdk and mise candidates under the real home are skipped.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", "/bin:/usr/bin")
	fake := &toolrun.FakeRunner{
		PathMap: map[string]string{goBin: goBin},
		Outputs: map[string]string{goBin: "go version go1.26.5 linux/amd64\n"},
	}

	got := toolrun.FindPinnedGoBinaryWithRunner("go1.26.5", fake)
	log.Assert("found", got == goBin, goBin, got)
}

func TestFindPinnedGoBinary_RejectsNearbyVersion_FakeRunner(t *testing.T) {
	goBin := "/fake/nearby/go"
	t.Setenv(toolrun.EnvFoundryGoBin, goBin)
	t.Setenv("GOMODCACHE", t.TempDir())
	t.Setenv("PATH", "/bin:/usr/bin")
	t.Setenv("HOME", t.TempDir())
	fake := &toolrun.FakeRunner{
		PathMap: map[string]string{goBin: goBin},
		Outputs: map[string]string{goBin: "go version go1.26.4 linux/amd64\n"},
	}

	got := toolrun.FindPinnedGoBinaryWithRunner("go1.26.5", fake)
	if got != "" {
		t.Fatalf("nearby version must not match: got %q", got)
	}
}

func TestPreflight_WrongVersion_RemediationMentionsEnv(t *testing.T) {
	log := testutil.New(t)
	// Isolate host pins so remediation takes the generic FOUNDRY_GO_BIN path
	// (or names a host pin — either still mentions FOUNDRY_GO_BIN).
	t.Setenv(toolrun.EnvFoundryGoBin, "")
	t.Setenv("GOMODCACHE", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", "/bin:/usr/bin")

	goPath := "/fake/bin/go"
	fake := &toolrun.FakeRunner{
		PathMap: map[string]string{"go": goPath},
		Outputs: map[string]string{
			goPath: "go version go1.26.4 linux/amd64\n",
		},
	}
	res := toolrun.Preflight(context.Background(), toolrun.PreflightOptions{
		Host:             testHost(),
		RequireGoVersion: "go1.26.5",
		Runner:           fake,
	})
	if res.OK() {
		t.Fatal("expected wrong version")
	}
	fe, ok := diagnostic.AsFoundryError(res.Err())
	if !ok {
		t.Fatalf("want FoundryError, got %T %v", res.Err(), res.Err())
	}
	rem := fe.Remediation()
	log.Inputs(map[string]string{"remediation": rem})
	if !strings.Contains(rem, toolrun.EnvFoundryGoBin) {
		t.Fatalf("remediation should guide %s: %s", toolrun.EnvFoundryGoBin, rem)
	}
	log.Step("wrong_version_env_hint", testutil.OutcomeOK, "ok")
}
