//go:build hostile && unix

package hostilefsx

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/robertguss/go-foundry-cli/internal/fsx"
)

// KernelVersion returns the uname release string for probe logs.
func KernelVersion() string {
	var uts unix.Utsname
	if err := unix.Uname(&uts); err != nil {
		return "unknown"
	}
	return unix.ByteSliceToString(uts.Release[:])
}

// DetectFSType returns the platform filesystem label for path.
func DetectFSType(path string) string {
	return detectFSType(path)
}

// FSRoots returns label→writable directory pairs for the matrix.
// FOUNDRY_FSX_ROOTS preferred; E1_FS_ROOTS accepted for promotion compatibility.
// Format: "ext4:/path,xfs:/path,btrfs:/path,vfat:/path".
// Default: a private 0700 dir on the current filesystem labeled by DetectFSType.
func FSRoots(t *testing.T) map[string]string {
	t.Helper()
	env := os.Getenv("FOUNDRY_FSX_ROOTS")
	if env == "" {
		env = os.Getenv("E1_FS_ROOTS")
	}
	if env != "" {
		out := make(map[string]string)
		for _, part := range strings.Split(env, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			label, path, ok := strings.Cut(part, ":")
			if !ok {
				t.Fatalf("bad FS roots entry %q (want label:path)", part)
			}
			out[label] = path
		}
		return out
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	label := detectFSType(dir)
	return map[string]string{label: dir}
}

// PrivateParent creates a 0700 directory under root for custody-safe tests.
func PrivateParent(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

// IsNegativeFS reports FAT-family labels used for rename-unsupported probes.
func IsNegativeFS(label string) bool {
	switch strings.ToLower(label) {
	case "vfat", "fat", "exfat", "msdos":
		return true
	default:
		return false
	}
}

// newProbeBridge builds an fsx.StepLogger backed by ProbeLog and an optional
// step hook. The hook is supplied from test files so production sources never
// import internal/testutil.
func newProbeBridge(pl *ProbeLog, fsLabel, kernel string, step func(name, outcome, detail string)) fsx.StepLogger {
	return &probeBridge{pl: pl, step: step, fs: fsLabel, kernel: kernel}
}

// recordHarnessMeta writes the env/meta probe line shared by NewHarness.
func recordHarnessMeta(pl *ProbeLog, fsLabel, kernel string) {
	pl.Record(ProbeEntry{
		FS:      fsLabel,
		Kernel:  kernel,
		Probe:   "env",
		Step:    "meta",
		Outcome: "info",
		Detail:  fmt.Sprintf("go=%s goos=%s goarch=%s syscall=%s", runtime.Version(), runtime.GOOS, runtime.GOARCH, fsx.RenameSyscallName()),
		Syscall: fsx.RenameSyscallName(),
	})
}

// attachHarnessCleanup dumps probe artifacts on test failure.
func attachHarnessCleanup(t *testing.T, pl *ProbeLog) {
	t.Helper()
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		path, err := pl.DumpArtifact(t.Name())
		if err != nil {
			t.Logf("artifact dump: %v", err)
			return
		}
		if path != "" {
			t.Logf("hostile_fsx_artifact=%s", path)
		}
		pass, fail := pl.SummaryPassFail()
		t.Logf("probe_summary pass=%d fail=%d entries=%d", pass, fail, len(pl.Entries))
	})
}

// ArrangeDest builds a private parent and an absent destination path under root.
func ArrangeDest(t *testing.T, root, label string) (dest, parentDir string) {
	t.Helper()
	parentDir = PrivateParent(t, root, label+"-parent")
	dest = filepath.Join(parentDir, "proj")
	return dest, parentDir
}

// PlantSentinel writes an unrelated sentinel file next to the destination parent.
func PlantSentinel(t *testing.T, parentDir string) (path string, body []byte) {
	t.Helper()
	path = filepath.Join(parentDir, "SENTINEL-outside-stage.txt")
	body = []byte("hostile-fsx-do-not-touch\n")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(parentDir, "unrelated-sibling")
	if err := os.Mkdir(sibling, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibling, "keep.txt"), []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, body
}

// AssertSentinelAlive fails if the sentinel or sibling was scavenged/mutated.
func AssertSentinelAlive(t *testing.T, path string, body []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("sentinel missing: %v", err)
	}
	if string(data) != string(body) {
		t.Fatalf("sentinel mutated: %q", data)
	}
	sib := filepath.Join(filepath.Dir(path), "unrelated-sibling", "keep.txt")
	if _, err := os.Lstat(sib); err != nil {
		t.Fatalf("sibling scavenged: %v", err)
	}
}

// StageStillPresent reports whether stage basename exists under parentDir.
func StageStillPresent(t *testing.T, parentDir, stageName string) bool {
	t.Helper()
	_, err := os.Lstat(filepath.Join(parentDir, stageName))
	return err == nil
}
