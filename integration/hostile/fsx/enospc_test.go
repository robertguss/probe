//go:build hostile && unix

package hostilefsx_test

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	hostilefsx "github.com/robertguss/go-foundry-cli/integration/hostile/fsx"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/fsx"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// enospcVolume provisions a genuinely tiny, real filesystem and returns its
// mount point, ready to be filled to ENOSPC (bead go-foundry-cli-wet.4.1 /
// REQ-213). It skips gracefully — never fails the suite — when the platform
// tooling required to provision one isn't available, per the documented
// policy in docs/dev/testing.md.
//
// darwin: a 2 MiB HFS+ disk image via `hdiutil create`/`attach` (no sudo).
// linux: a 2 MiB tmpfs via `mount -t tmpfs -o size=2m` (best effort; many
// hosts allow unprivileged/passwordless mount for tmpfs, but this is not
// guaranteed — skip when it fails rather than requiring sudo).
func enospcVolume(t *testing.T) string {
	t.Helper()
	switch runtime.GOOS {
	case "darwin":
		hdiutil, err := exec.LookPath("hdiutil")
		if err != nil {
			t.Skip("hdiutil not on PATH; cannot provision a real ENOSPC volume")
		}
		dmg := filepath.Join(t.TempDir(), "foundry-enospc.dmg")
		create := exec.Command(hdiutil, "create", "-size", "2m", "-fs", "HFS+",
			"-volname", "foundryenospc", "-quiet", dmg)
		if out, err := create.CombinedOutput(); err != nil {
			t.Skipf("hdiutil create failed (no ENOSPC harness available): %v: %s", err, out)
		}
		mnt := t.TempDir()
		attach := exec.Command(hdiutil, "attach", dmg, "-mountpoint", mnt, "-nobrowse", "-quiet")
		if out, err := attach.CombinedOutput(); err != nil {
			t.Skipf("hdiutil attach failed (no ENOSPC harness available): %v: %s", err, out)
		}
		t.Cleanup(func() {
			_ = exec.Command(hdiutil, "detach", mnt, "-quiet", "-force").Run()
		})
		return mnt
	case "linux":
		mountBin, err := exec.LookPath("mount")
		if err != nil {
			t.Skip("mount not on PATH; cannot provision a real ENOSPC volume")
		}
		mnt := t.TempDir()
		args := []string{"-t", "tmpfs", "-o", "size=2m", "tmpfs", mnt}
		mount := exec.Command(mountBin, args...)
		out, mountErr := mount.CombinedOutput()
		umountBin := "umount"
		if mountErr != nil {
			// Unprivileged tmpfs mount is not guaranteed; CI runners
			// (ubuntu-latest) typically grant passwordless sudo, so retry
			// once via sudo -n (non-interactive: never prompts/hangs)
			// before giving up.
			if sudoBin, sudoErr := exec.LookPath("sudo"); sudoErr == nil {
				sudoArgs := append([]string{"-n", mountBin}, args...)
				sudoMount := exec.Command(sudoBin, sudoArgs...)
				if sudoOut, err2 := sudoMount.CombinedOutput(); err2 == nil {
					mountErr = nil
					umountBin = "sudo"
				} else {
					out = append(out, sudoOut...)
				}
			}
		}
		if mountErr != nil {
			t.Skipf("tmpfs mount unavailable (needs privilege this host/container doesn't grant); "+
				"no ENOSPC harness available: %v: %s", mountErr, out)
		}
		t.Cleanup(func() {
			if umountBin == "sudo" {
				_ = exec.Command("sudo", "-n", "umount", mnt).Run()
				return
			}
			_ = exec.Command(umountBin, mnt).Run()
		})
		return mnt
	default:
		t.Skipf("no ENOSPC harness for GOOS=%s", runtime.GOOS)
		return ""
	}
}

// TestHostileFSX_ENOSPC_StagePreserved fills a genuinely tiny real
// filesystem during a staged write and asserts: the write fails with a
// stable fs.* classification carrying the ENOSPC cause, the stage is
// preserved (never deleted), and no destination is created (REQ-213 /
// Section 31.6).
func TestHostileFSX_ENOSPC_StagePreserved(t *testing.T) {
	mnt := enospcVolume(t)
	log, tl, pl := hostilefsx.NewHarness(t, "enospc")
	tl.Phase("arrange")
	parentDir := hostilefsx.PrivateParent(t, mnt, "enospc-parent")
	dest := filepath.Join(parentDir, "proj")
	tl.PhaseEnd("arrange", testutil.OutcomeOK)

	tl.Phase("act")
	txn, err := fsx.Begin(dest, fsx.Options{Log: log, Project: "proj"})
	if err != nil {
		t.Fatalf("Begin failed before the volume was filled: %v", err)
	}
	t.Cleanup(func() { _ = txn.Close() })
	stageName := txn.StageName()

	// 8 MiB payload against a 2 MiB volume guarantees ENOSPC regardless of
	// filesystem metadata overhead.
	payload := make([]byte, 8*1024*1024)
	writeErr := txn.RootedWriter().WriteFile("bigfile.bin", "0644", payload)
	tl.Step("write_oversized", testutil.OutcomeOK, "payload_bytes=8388608")
	tl.PhaseEnd("act", testutil.OutcomeOK)

	tl.Phase("assert")
	if writeErr == nil {
		t.Fatal("expected ENOSPC write failure on a 2MiB volume with an 8MiB payload")
	}
	fe, ok := diagnostic.AsFoundryError(writeErr)
	tl.Assert("foundry_error", ok, true, ok)
	if ok {
		tl.Assert("id", fe.ID() == diagnostic.IDRenderFailed, diagnostic.IDRenderFailed, fe.ID())
	}
	enospc := errors.Is(writeErr, syscall.ENOSPC) ||
		strings.Contains(strings.ToLower(writeErr.Error()), "no space")
	tl.Assert("enospc_cause", enospc, true, writeErr.Error())

	// Stage preserved: never deleted on failure (Section 31.6 / REQ-130).
	stagePresent := hostilefsx.StageStillPresent(t, parentDir, stageName)
	tl.Assert("stage_preserved", stagePresent, true, stagePresent)

	// No destination placement on failure.
	destExists := hostilefsx.StageStillPresent(t, parentDir, "proj")
	tl.Assert("no_destination", !destExists, false, destExists)

	pl.Record(hostilefsx.ProbeEntry{
		FS: "enospc", Kernel: hostilefsx.KernelVersion(),
		Probe: "enospc_write", Step: "write", Outcome: "pass",
		StageName: stageName, Detail: "id=" + string(fe.ID()),
	})
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}
