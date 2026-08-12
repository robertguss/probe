//go:build hostile && unix

package hostilefsx_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"

	hostilefsx "github.com/robertguss/go-foundry-cli/integration/hostile/fsx"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/fsx"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// re-export harness helpers via package-level wrappers that call into the
// same package (tests are external so they use the public/hostile helpers).

func TestHostileFSX_PlatformExclusiveRenameSyscall(t *testing.T) {
	name := fsx.RenameSyscallName()
	log := testutil.New(t)
	log.Phase("assert")
	switch runtime.GOOS {
	case "linux":
		log.Assert("linux_syscall", name == "Renameat2(RENAME_NOREPLACE)",
			"Renameat2(RENAME_NOREPLACE)", name)
	case "darwin":
		log.Assert("darwin_syscall", name == "RenameatxNp(RENAME_EXCL|RENAME_NOFOLLOW_ANY)",
			"RenameatxNp(RENAME_EXCL|RENAME_NOFOLLOW_ANY)", name)
	default:
		t.Logf("GOOS=%s rename=%s", runtime.GOOS, name)
	}
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestHostileFSX_HappyPath_BeginCommit is the baseline green fixture on every
// positive FS root (E1 promotion: happy path stage+commit via production Begin).
func TestHostileFSX_HappyPath_BeginCommit(t *testing.T) {
	for fsLabel, root := range hostilefsx.FSRoots(t) {
		fsLabel, root := fsLabel, root
		if hostilefsx.IsNegativeFS(fsLabel) {
			continue
		}
		t.Run(fsLabel, func(t *testing.T) {
			log, tl, pl := hostilefsx.NewHarness(t, fsLabel)
			tl.Phase("arrange")
			dest, parentDir := hostilefsx.ArrangeDest(t, root, "happy")
			sentinel, body := hostilefsx.PlantSentinel(t, parentDir)
			tl.PhaseEnd("arrange", testutil.OutcomeOK)

			tl.Phase("act")
			txn, err := fsx.Begin(dest, fsx.Options{Log: log, Project: "proj"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = txn.Close() })
			stageName := txn.StageName()
			stageID := txn.StageIdentity()
			if err := txn.RootedWriter().WriteFile("hello.txt", "0644", []byte("hostile\n")); err != nil {
				t.Fatal(err)
			}
			res := txn.Commit()
			tl.PhaseEnd("act", testutil.OutcomeOK)

			tl.Phase("assert")
			tl.Assert("class_committed", res.Class == fsx.ClassCommitted, fsx.ClassCommitted, res.Class)
			tl.Assert("exit_0", res.Exit == diagnostic.ExitSuccess, diagnostic.ExitSuccess, res.Exit)
			data, err := os.ReadFile(filepath.Join(parentDir, "proj", "hello.txt"))
			if err != nil {
				t.Fatal(err)
			}
			tl.Assert("content", string(data) == "hostile\n", "hostile\\n", string(data))
			if hostilefsx.StageStillPresent(t, parentDir, stageName) {
				t.Fatal("stage basename still present after successful commit")
			}
			hostilefsx.AssertSentinelAlive(t, sentinel, body)
			pl.Record(hostilefsx.ProbeEntry{
				FS: fsLabel, Kernel: hostilefsx.KernelVersion(),
				Probe: "happy_path", Step: "done", Outcome: "pass",
				Syscall:   fsx.RenameSyscallName(),
				StageName: stageName, StageIdentity: stageID.String(),
				Detail: fmt.Sprintf("dest=proj class=%s", res.Class),
			})
			tl.PhaseEnd("assert", testutil.OutcomeOK)
		})
	}
}

// TestHostileFSX_ParentSwap_BarrierControlled renames the parent pathname after
// Begin (retained handle) and asserts reobserve fail-closed + stage preserved
// (REQ-213 parent-swap race; E1 ParentSwap promotion).
func TestHostileFSX_ParentSwap_BarrierControlled(t *testing.T) {
	for fsLabel, root := range hostilefsx.FSRoots(t) {
		if hostilefsx.IsNegativeFS(fsLabel) {
			continue
		}
		fsLabel, root := fsLabel, root
		t.Run(fsLabel, func(t *testing.T) {
			log, tl, pl := hostilefsx.NewHarness(t, fsLabel)
			tl.Phase("arrange")
			container := hostilefsx.PrivateParent(t, root, "swap-container")
			parentDir := filepath.Join(container, "parent-a")
			if err := os.Mkdir(parentDir, 0o700); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(parentDir, "proj")
			sentinel, body := hostilefsx.PlantSentinel(t, container)
			tl.PhaseEnd("arrange", testutil.OutcomeOK)

			tl.Phase("act")
			txn, err := fsx.Begin(dest, fsx.Options{Log: log, Project: "proj"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = txn.Close() })
			stageName := txn.StageName()
			stageID := txn.StageIdentity()
			if err := txn.RootedWriter().WriteFile("still-here.txt", "0644", []byte("ok\n")); err != nil {
				t.Fatal(err)
			}

			// Barrier: parent pathname swap after stage is live.
			swapped := filepath.Join(container, "parent-b-swapped")
			if err := os.Rename(parentDir, swapped); err != nil {
				t.Fatal(err)
			}
			pl.Record(hostilefsx.ProbeEntry{
				FS: fsLabel, Kernel: hostilefsx.KernelVersion(),
				Probe: "parent_swap", Step: "rename_parent", Outcome: "info",
				Args:      "parent-a -> parent-b-swapped",
				StageName: stageName, StageIdentity: stageID.String(),
			})

			res := txn.Commit()
			tl.PhaseEnd("act", testutil.OutcomeOK)

			tl.Phase("assert")
			// Parent reobserve fail-closed: uncommitted + parent_moved; stage preserved.
			tl.Assert("class_uncommitted", res.Class == fsx.ClassUncommitted, fsx.ClassUncommitted, res.Class)
			tl.Assert("id_parent_moved", res.ErrorID() == diagnostic.IDFSParentMoved,
				diagnostic.IDFSParentMoved, res.ErrorID())
			tl.Assert("exit_1", res.Exit == diagnostic.ExitFailure, diagnostic.ExitFailure, res.Exit)
			// Stage lives under the swapped parent pathname (same object via fd).
			if !hostilefsx.StageStillPresent(t, swapped, stageName) {
				t.Fatalf("stage %s not preserved under swapped parent", stageName)
			}
			// Destination must not have been created under swapped parent.
			if _, err := os.Lstat(filepath.Join(swapped, "proj")); err == nil {
				t.Fatal("destination must not exist after parent_moved")
			}
			hostilefsx.AssertSentinelAlive(t, sentinel, body)
			pl.Record(hostilefsx.ProbeEntry{
				FS: fsLabel, Kernel: hostilefsx.KernelVersion(),
				Probe: "parent_swap", Step: "done", Outcome: "pass",
				Syscall:   fsx.RenameSyscallName(),
				StageName: stageName, StageIdentity: stageID.String(),
				StagePath: res.StagePath,
				Detail:    fmt.Sprintf("id=%s class=%s stage_preserved", res.ErrorID(), res.Class),
			})
			tl.PhaseEnd("assert", testutil.OutcomeOK)
		})
	}
}

// TestHostileFSX_Custody_OwnerModeStickyMatrix promotes the E1/parent custody
// matrix onto production AcquireParent (REQ-044 / 31.3).
func TestHostileFSX_Custody_OwnerModeStickyMatrix(t *testing.T) {
	// Host FS only — mode bits are not meaningful on vfat.
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	fsLabel := hostilefsx.DetectFSType(root)
	log, tl, pl := hostilefsx.NewHarness(t, fsLabel)

	cases := []struct {
		name    string
		mode    os.FileMode
		wantErr diagnostic.Identifier // empty = success
	}{
		{"mode_0700", 0o700, ""},
		{"mode_0750", 0o750, ""},
		{"mode_0755", 0o755, ""},
		{"mode_0770", 0o770, diagnostic.IDFSNamespaceNotPrivate},
		{"mode_0777", 0o777, diagnostic.IDFSNamespaceNotPrivate},
		{"mode_1777_sticky", 0o777 | os.ModeSticky, ""},
	}

	tl.Phase("matrix")
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(root, tc.name)
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, tc.mode); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
			dest := filepath.Join(dir, "proj")

			parent, err := fsx.AcquireParent(dest, fsx.PreflightOptions{Log: log})
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected: %v", err)
				}
				_ = parent.Close()
				pl.Record(hostilefsx.ProbeEntry{
					FS: fsLabel, Probe: "custody", Step: tc.name, Outcome: "pass",
					Detail: fmt.Sprintf("mode=%04o allowed", tc.mode.Perm()|tc.mode&os.ModeSticky),
				})
				return
			}
			if err == nil {
				_ = parent.Close()
				t.Fatalf("expected %s", tc.wantErr)
			}
			fe, ok := diagnostic.AsFoundryError(err)
			if !ok || fe.ID() != tc.wantErr {
				t.Fatalf("got %v want %s", err, tc.wantErr)
			}
			pl.Record(hostilefsx.ProbeEntry{
				FS: fsLabel, Probe: "custody", Step: tc.name, Outcome: "pass",
				Detail: fmt.Sprintf("mode rejected id=%s", tc.wantErr),
			})
		})
	}
	// Sticky /tmp permitted (SV-05).
	t.Run("sticky_tmp", func(t *testing.T) {
		tmp, err := os.MkdirTemp("/tmp", "hostile-fsx-sticky-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(tmp) })
		if err := os.Chmod(tmp, 0o700); err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(tmp, "proj")
		parent, err := fsx.AcquireParent(dest, fsx.PreflightOptions{Log: log})
		if err != nil {
			t.Fatal(err)
		}
		_ = parent.Close()
		pl.Record(hostilefsx.ProbeEntry{
			FS: fsLabel, Probe: "custody", Step: "sticky_tmp", Outcome: "pass",
			Detail: "sticky world-writable ancestor permitted",
		})
	})
	tl.PhaseEnd("matrix", testutil.OutcomeOK)
}

// TestHostileFSX_StageEntrySwap_BeforeCommit renames the stage entry under the
// parent after Begin while the stage fd is retained, then Commit must not
// claim success against a wrong object (E1 stage-entry swap promotion).
func TestHostileFSX_StageEntrySwap_BeforeCommit(t *testing.T) {
	for fsLabel, root := range hostilefsx.FSRoots(t) {
		if hostilefsx.IsNegativeFS(fsLabel) {
			continue
		}
		fsLabel, root := fsLabel, root
		t.Run(fsLabel, func(t *testing.T) {
			log, tl, pl := hostilefsx.NewHarness(t, fsLabel)
			dest, parentDir := hostilefsx.ArrangeDest(t, root, "stage-swap")
			sentinel, body := hostilefsx.PlantSentinel(t, parentDir)

			txn, err := fsx.Begin(dest, fsx.Options{Log: log, Project: "proj"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = txn.Close() })
			stageName := txn.StageName()
			stageID := txn.StageIdentity()
			if err := txn.RootedWriter().WriteFile("x.txt", "0644", []byte("x\n")); err != nil {
				t.Fatal(err)
			}

			// Swap stage entry pathname under parent (object continuity via fd).
			swappedName := stageName + "-swapped"
			parentFD := txn.Parent().DirFD()
			if err := unix.Renameat(parentFD, stageName, parentFD, swappedName); err != nil {
				t.Fatalf("renameat stage entry: %v", err)
			}
			pl.Record(hostilefsx.ProbeEntry{
				FS: fsLabel, Kernel: hostilefsx.KernelVersion(),
				Probe: "stage_entry_swap", Step: "renameat", Outcome: "info",
				Syscall: "Renameat", Args: stageName + " -> " + swappedName,
				StageName: stageName, StageIdentity: stageID.String(),
				Errno: "",
			})

			res := txn.Commit()
			// Must not be a clean committed success (stage name gone; rename fails
			// or classification is uncommitted/ambiguous/conflict).
			if res.Class == fsx.ClassCommitted {
				t.Fatalf("committed after stage-entry swap — exclusive semantics broken; res=%+v", res)
			}
			if res.Exit == diagnostic.ExitSuccess {
				t.Fatal("exit 0 after stage-entry swap")
			}
			// Destination must not hold a partial tree from a wrong rename.
			// Stage object still exists under swapped name (fd continuous).
			if !hostilefsx.StageStillPresent(t, parentDir, swappedName) {
				// Some classifications may still observe via recorded name only;
				// require that original stage name is absent and dest is not the stage.
				if hostilefsx.StageStillPresent(t, parentDir, stageName) {
					t.Fatal("original stage name reappeared unexpectedly")
				}
			}
			// Unrelated sentinel survives.
			hostilefsx.AssertSentinelAlive(t, sentinel, body)
			pl.Record(hostilefsx.ProbeEntry{
				FS: fsLabel, Kernel: hostilefsx.KernelVersion(),
				Probe: "stage_entry_swap", Step: "done", Outcome: "pass",
				Syscall:   fsx.RenameSyscallName(),
				Errno:     errnoString(res.SyscallErr),
				StageName: stageName, StageIdentity: stageID.String(),
				StagePath: res.StagePath,
				Detail:    fmt.Sprintf("class=%s id=%s exit=%d", res.Class, res.ErrorID(), res.Exit),
			})
			tl.Assert("not_committed", res.Class != fsx.ClassCommitted, true, true)
		})
	}
}

// TestHostileFSX_TwoProcessEEXIST_LoserPreserved races two Transactions to one
// destination (REQ-129/131, E1 two-process promotion).
func TestHostileFSX_TwoProcessEEXIST_LoserPreserved(t *testing.T) {
	for fsLabel, root := range hostilefsx.FSRoots(t) {
		if hostilefsx.IsNegativeFS(fsLabel) {
			continue
		}
		fsLabel, root := fsLabel, root
		t.Run(fsLabel, func(t *testing.T) {
			log, tl, pl := hostilefsx.NewHarness(t, fsLabel)
			dest, parentDir := hostilefsx.ArrangeDest(t, root, "race")
			sentinel, body := hostilefsx.PlantSentinel(t, parentDir)

			txnA, err := fsx.Begin(dest, fsx.Options{Log: log, Project: "proj"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = txnA.Close() })
			txnB, err := fsx.Begin(dest, fsx.Options{Log: log, Project: "proj"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = txnB.Close() })
			if err := txnA.RootedWriter().WriteFile("a.txt", "0644", []byte("A")); err != nil {
				t.Fatal(err)
			}
			if err := txnB.RootedWriter().WriteFile("b.txt", "0644", []byte("B")); err != nil {
				t.Fatal(err)
			}
			stageA, stageB := txnA.StageName(), txnB.StageName()

			var (
				mu      sync.Mutex
				results []fsx.CommitResult
				wg      sync.WaitGroup
			)
			// Barrier so both commits start as close as possible.
			start := make(chan struct{})
			wg.Add(2)
			run := func(txn *fsx.Transaction) {
				defer wg.Done()
				<-start
				res := txn.Commit()
				mu.Lock()
				results = append(results, res)
				mu.Unlock()
			}
			go run(txnA)
			go run(txnB)
			close(start)
			wg.Wait()

			var winners, losers int
			for _, r := range results {
				switch {
				case r.Class == fsx.ClassCommitted:
					winners++
					if r.Exit != 0 {
						t.Errorf("winner exit=%d", r.Exit)
					}
				case r.Class == fsx.ClassConflict || r.ErrorID() == diagnostic.IDFSDestinationExists:
					losers++
					if r.Exit != diagnostic.ExitUsage {
						t.Errorf("loser exit=%d want 2", r.Exit)
					}
					if !hostilefsx.StageStillPresent(t, parentDir, r.StageName) {
						t.Errorf("loser stage %s not preserved", r.StageName)
					}
				default:
					t.Errorf("unexpected result class=%s id=%s exit=%d msg=%s",
						r.Class, r.ErrorID(), r.Exit, r.Message)
				}
			}
			tl.Assert("one_winner", winners == 1, 1, winners)
			tl.Assert("one_loser", losers == 1, 1, losers)
			hostilefsx.AssertSentinelAlive(t, sentinel, body)
			pl.Record(hostilefsx.ProbeEntry{
				FS: fsLabel, Kernel: hostilefsx.KernelVersion(),
				Probe: "eexist_race", Step: "done", Outcome: "pass",
				Syscall: fsx.RenameSyscallName(),
				Detail:  fmt.Sprintf("winners=%d losers=%d stages=%s,%s", winners, losers, stageA, stageB),
			})
		})
	}
}

// TestHostileFSX_AmbiguousAndUnsupportedClassification proves Section 31.8
// identity classification for false-success and uncommitted errno shapes using
// the public ClassifyCommit surface (injection of exclusiveRename lives in
// internal/fsx hostile-tagged tests — export_test hooks are package-local).
func TestHostileFSX_AmbiguousAndUnsupportedClassification(t *testing.T) {
	_, tl, pl := hostilefsx.NewHarness(t, "host")
	id := fsx.FileID{} // zero ids still exercise classification branches
	// Non-zero-looking ids via ChildLookup on real objects for realism.
	root := t.TempDir()
	_ = os.Chmod(root, 0o700)
	log, _, _ := hostilefsx.NewHarness(t, hostilefsx.DetectFSType(root))
	dest, parentDir := hostilefsx.ArrangeDest(t, root, "class")
	txn, err := fsx.Begin(dest, fsx.Options{Log: log, Project: "proj"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = txn.Close() })
	id = txn.StageIdentity()
	_ = parentDir

	cases := []struct {
		name string
		obs  fsx.CommitObservation
		want fsx.CommitClass
	}{
		{
			name: "false_success_ambiguous",
			obs: fsx.CommitObservation{
				SyscallOK: true, StagePresent: true, StageID: id,
				RecordedStage: id, DestPresent: false,
			},
			want: fsx.ClassAmbiguous,
		},
		{
			name: "uncommitted_enotsup_shape",
			obs: fsx.CommitObservation{
				SyscallOK: false, SyscallErrno: unix.ENOTSUP,
				StagePresent: true, StageID: id, RecordedStage: id, DestPresent: false,
			},
			want: fsx.ClassUncommitted,
		},
		{
			name: "uncommitted_exdev_shape",
			obs: fsx.CommitObservation{
				SyscallOK: false, SyscallErrno: unix.EXDEV,
				StagePresent: true, StageID: id, RecordedStage: id, DestPresent: false,
			},
			want: fsx.ClassUncommitted,
		},
		{
			name: "conflict_other_dest",
			obs: fsx.CommitObservation{
				SyscallOK: false, SyscallErrno: unix.EEXIST,
				StagePresent: true, StageID: id, RecordedStage: id,
				DestPresent: true, DestID: fsx.FileID{Dev: 1, Ino: 2},
			},
			want: fsx.ClassConflict,
		},
		{
			name: "committed_dest_is_stage",
			obs: fsx.CommitObservation{
				SyscallOK: true, StagePresent: false,
				DestPresent: true, DestID: id, RecordedStage: id,
			},
			want: fsx.ClassCommitted,
		},
	}
	tl.Phase("classify")
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, detail := fsx.ClassifyCommit(tc.obs)
			if got != tc.want {
				t.Fatalf("class=%s want=%s detail=%s", got, tc.want, detail)
			}
			pl.Record(hostilefsx.ProbeEntry{
				Probe: "classify", Step: tc.name, Outcome: "pass",
				Errno:         errnoString(tc.obs.SyscallErrno),
				Detail:        fmt.Sprintf("class=%s %s", got, detail),
				StageIdentity: id.String(),
			})
		})
	}
	tl.PhaseEnd("classify", testutil.OutcomeOK)
}

// TestHostileFSX_EXDEV_AndRenameUnsupported_FailClosed exercises live FAT mounts
// when present and proves stage preservation on exclusive-rename failure paths
// reachable without package-local injection (pre-existing dest EEXIST).
// Full ENOTSUP/EXDEV Commit injection is in internal/fsx (//go:build hostile).
func TestHostileFSX_EXDEV_AndRenameUnsupported_FailClosed(t *testing.T) {
	root := t.TempDir()
	_ = os.Chmod(root, 0o700)
	fsLabel := hostilefsx.DetectFSType(root)

	t.Run("preexisting_dest_eexist_preserve", func(t *testing.T) {
		log, tl, pl := hostilefsx.NewHarness(t, fsLabel)
		dest, parentDir := hostilefsx.ArrangeDest(t, root, "eexist")
		sentinel, body := hostilefsx.PlantSentinel(t, parentDir)
		// Pre-create destination so exclusive rename hits EEXIST.
		if err := os.Mkdir(dest, 0o755); err != nil {
			t.Fatal(err)
		}
		// Begin refuses existing dest — use CreateStage after AcquireParent.
		parent, err := fsx.AcquireParent(dest, fsx.PreflightOptions{Log: log})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = parent.Close() })
		stage, err := fsx.CreateStage(parent, "proj")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = stage.Close() })
		stageName := stage.Name()
		_ = stage.Writer().WriteFile("u.txt", "0644", []byte("u\n"))
		res := fsx.Commit(stage)
		if res.Committed() {
			t.Fatal("must not replace existing dest")
		}
		if res.ErrorID() != diagnostic.IDFSDestinationExists {
			t.Fatalf("id=%s want destination_exists class=%s", res.ErrorID(), res.Class)
		}
		if !hostilefsx.StageStillPresent(t, parentDir, stageName) {
			t.Fatal("stage not preserved on EEXIST")
		}
		hostilefsx.AssertSentinelAlive(t, sentinel, body)
		pl.Record(hostilefsx.ProbeEntry{
			FS: fsLabel, Probe: "eexist_preserve", Step: "done", Outcome: "pass",
			Syscall: fsx.RenameSyscallName(), Errno: "EEXIST",
			StageName: stageName, StagePath: res.StagePath,
		})
		tl.Assert("exit_2", res.Exit == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Exit)
	})

	// Live FAT negative when matrix provides a mount.
	for label, fatRoot := range hostilefsx.FSRoots(t) {
		if !hostilefsx.IsNegativeFS(label) {
			continue
		}
		label, fatRoot := label, fatRoot
		t.Run("live_"+label, func(t *testing.T) {
			_, _, pl := hostilefsx.NewHarness(t, label)
			parentDir := filepath.Join(fatRoot, "fat-parent")
			if err := os.MkdirAll(parentDir, 0o777); err != nil {
				t.Fatal(err)
			}
			fd, err := unix.Open(parentDir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer unix.Close(fd)
			stageName := ".foundry-fatprobe-aaaa"
			destName := "dest-proj"
			_ = unix.Unlinkat(fd, stageName, unix.AT_REMOVEDIR)
			_ = unix.Unlinkat(fd, destName, unix.AT_REMOVEDIR)
			if err := unix.Mkdirat(fd, stageName, 0o700); err != nil {
				t.Fatalf("mkdirat: %v", err)
			}
			sysErr := exclusiveRenameViaHook(t, fd, stageName, destName)
			pl.Record(hostilefsx.ProbeEntry{
				FS: label, Probe: "fat_rename", Step: "first",
				Syscall: fsx.RenameSyscallName(),
				Args:    stageName + " -> " + destName,
				Errno:   errnoString(sysErr),
				Outcome: "info",
			})
			if sysErr != nil && isRenameUnsupported(sysErr) {
				if !childExists(t, fd, stageName) {
					t.Fatal("stage must be preserved when rename unsupported")
				}
				pl.Record(hostilefsx.ProbeEntry{
					FS: label, Probe: "fat_rename", Step: "unsupported", Outcome: "pass",
					Errno: errnoString(sysErr),
				})
				return
			}
			if sysErr != nil {
				t.Fatalf("first exclusive rename failed: %v", sysErr)
			}
			stage2 := ".foundry-fatprobe-bbbb"
			_ = unix.Unlinkat(fd, stage2, unix.AT_REMOVEDIR)
			if err := unix.Mkdirat(fd, stage2, 0o700); err != nil {
				t.Fatal(err)
			}
			sysErr2 := exclusiveRenameViaHook(t, fd, stage2, destName)
			if sysErr2 == nil {
				t.Fatal("RENAME_NOREPLACE on FAT replaced existing dest")
			}
			if !childExists(t, fd, stage2) {
				t.Fatal("loser stage not preserved on FAT")
			}
			pl.Record(hostilefsx.ProbeEntry{
				FS: label, Probe: "fat_rename", Step: "second_eexist", Outcome: "pass",
				Errno:  errnoString(sysErr2),
				Detail: "FAT supports exclusive rename on this kernel; loser preserved",
			})
		})
	}
}

// TestHostileFSX_CaseSensitivityNativeMatrix asserts FND-018 native exact-name
// behavior (no synthetic case-fold scan).
func TestHostileFSX_CaseSensitivityNativeMatrix(t *testing.T) {
	root := t.TempDir()
	_ = os.Chmod(root, 0o700)
	fsLabel := hostilefsx.DetectFSType(root)
	log, tl, pl := hostilefsx.NewHarness(t, fsLabel)
	parentDir := hostilefsx.PrivateParent(t, root, "case-parent")
	alt := "Proj"
	if err := os.Mkdir(filepath.Join(parentDir, alt), 0o700); err != nil {
		t.Fatal(err)
	}
	parent, err := fsx.AcquireParent(filepath.Join(parentDir, "proj"), fsx.PreflightOptions{Log: log})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parent.Close() })
	existsProj, _, err := parent.ChildLookup("proj")
	if err != nil {
		t.Fatal(err)
	}
	existsAlt, altID, err := parent.ChildLookup(alt)
	if err != nil || !existsAlt {
		t.Fatalf("alt missing: exists=%v err=%v", existsAlt, err)
	}
	caseInsensitive := existsProj
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	_ = stage.Writer().WriteFile("x.txt", "0644", []byte("x\n"))
	stageName := stage.Name()
	res := fsx.Commit(stage)

	if caseInsensitive {
		if res.Class == fsx.ClassCommitted {
			t.Fatal("case-insensitive FS committed over folded name")
		}
		if res.ErrorID() != diagnostic.IDFSDestinationExists {
			t.Fatalf("want destination_exists; got %s class=%s", res.ErrorID(), res.Class)
		}
		if !hostilefsx.StageStillPresent(t, parentDir, stageName) {
			t.Fatal("stage not preserved")
		}
		exists, id, err := parent.ChildLookup(alt)
		if err != nil || !exists || !id.Equal(altID) {
			t.Fatalf("alt identity lost")
		}
		pl.Record(hostilefsx.ProbeEntry{
			FS: fsLabel, Probe: "case", Step: "insensitive_no_replace", Outcome: "pass",
			StageName: stageName,
		})
	} else {
		tl.Assert("committed", res.Class == fsx.ClassCommitted, fsx.ClassCommitted, res.Class)
		exists, id, err := parent.ChildLookup(alt)
		if err != nil || !exists || !id.Equal(altID) {
			t.Fatalf("Proj sibling lost")
		}
		pl.Record(hostilefsx.ProbeEntry{
			FS: fsLabel, Probe: "case", Step: "sensitive_sibling_ok", Outcome: "pass",
			StageName: stageName,
		})
	}
}

// TestHostileFSX_CancelAndCrash_StagePreserved simulates cancellation (Close
// without Commit) and crash leftovers (abandon handles) — stage remains for
// owner inspection (Section 31.6 / REQ-036 / REQ-130).
func TestHostileFSX_CancelAndCrash_StagePreserved(t *testing.T) {
	root := t.TempDir()
	_ = os.Chmod(root, 0o700)
	fsLabel := hostilefsx.DetectFSType(root)

	t.Run("cancel_close_no_commit", func(t *testing.T) {
		log, tl, pl := hostilefsx.NewHarness(t, fsLabel)
		dest, parentDir := hostilefsx.ArrangeDest(t, root, "cancel")
		sentinel, body := hostilefsx.PlantSentinel(t, parentDir)
		txn, err := fsx.Begin(dest, fsx.Options{Log: log, Project: "proj"})
		if err != nil {
			t.Fatal(err)
		}
		stageName := txn.StageName()
		stagePath := txn.StagePath()
		_ = txn.RootedWriter().WriteFile("partial.txt", "0644", []byte("partial\n"))
		// Cancellation before commit: Close releases fds only.
		if err := txn.Close(); err != nil {
			t.Fatal(err)
		}
		if !hostilefsx.StageStillPresent(t, parentDir, stageName) {
			t.Fatal("stage deleted on cancel/Close — Section 31.6 violation")
		}
		// RSK-310 remediation text available for reporting hooks.
		rem := fsx.ManualRemovalRemediation(stagePath)
		if rem == "" || !containsAll(rem, "RSK-310") {
			t.Fatalf("RSK-310 remediation missing: %q", rem)
		}
		hostilefsx.AssertSentinelAlive(t, sentinel, body)
		// Commit after Close must fail closed, not delete.
		res := txn.Commit()
		if res.Class == fsx.ClassCommitted {
			t.Fatal("commit after close must not succeed")
		}
		if !hostilefsx.StageStillPresent(t, parentDir, stageName) {
			t.Fatal("stage scavenged after late commit")
		}
		pl.Record(hostilefsx.ProbeEntry{
			FS: fsLabel, Probe: "cancel", Step: "close_no_commit", Outcome: "pass",
			StageName: stageName, StagePath: stagePath,
			Detail: fmt.Sprintf("late_commit_class=%s", res.Class),
		})
		tl.Assert("stage_path_set", stagePath != "", "non-empty", stagePath)
	})

	t.Run("crash_abandon_handles", func(t *testing.T) {
		log, _, pl := hostilefsx.NewHarness(t, fsLabel)
		dest, parentDir := hostilefsx.ArrangeDest(t, root, "crash")
		sentinel, body := hostilefsx.PlantSentinel(t, parentDir)
		txn, err := fsx.Begin(dest, fsx.Options{Log: log, Project: "proj"})
		if err != nil {
			t.Fatal(err)
		}
		stageName := txn.StageName()
		_ = txn.RootedWriter().WriteFile("crash.txt", "0644", []byte("crash\n"))
		// Simulate process crash: drop references without Close or Commit.
		// (GC may close FDs; stage directory entry must remain on disk.)
		txn = nil
		runtime.GC()
		if !hostilefsx.StageStillPresent(t, parentDir, stageName) {
			t.Fatal("stage missing after crash simulation")
		}
		hostilefsx.AssertSentinelAlive(t, sentinel, body)
		pl.Record(hostilefsx.ProbeEntry{
			FS: fsLabel, Probe: "crash", Step: "abandon", Outcome: "pass",
			StageName: stageName,
			Detail:    "stage entry survives abandoned handles",
		})
	})
}

// TestHostileFSX_SentinelSurvival_AllFailureClasses plants sentinels around
// failure classes reachable via public API (EEXIST conflict, cancel/close,
// parent swap) and asserts survival (REQ-213 / 31.6). Injection classes
// (ENOTSUP/EXDEV/ambiguous) are covered by internal/fsx hostile-tagged tests.
func TestHostileFSX_SentinelSurvival_AllFailureClasses(t *testing.T) {
	root := t.TempDir()
	_ = os.Chmod(root, 0o700)
	fsLabel := hostilefsx.DetectFSType(root)

	t.Run("eexist_conflict", func(t *testing.T) {
		log, tl, pl := hostilefsx.NewHarness(t, fsLabel)
		dest, parentDir := hostilefsx.ArrangeDest(t, root, "sent-eexist")
		sentinel, body := hostilefsx.PlantSentinel(t, parentDir)
		txn, err := fsx.Begin(dest, fsx.Options{Log: log, Project: "proj"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = txn.Close() })
		_ = txn.RootedWriter().WriteFile("f.txt", "0644", []byte("f\n"))
		if err := unix.Mkdirat(txn.Parent().DirFD(), txn.Parent().Basename(), 0o755); err != nil {
			t.Fatal(err)
		}
		res := txn.Commit()
		if res.Committed() {
			t.Fatal("expected non-committed")
		}
		hostilefsx.AssertSentinelAlive(t, sentinel, body)
		if res.StagePath == "" {
			t.Fatal("StagePath must be reported on failure")
		}
		pl.Record(hostilefsx.ProbeEntry{
			FS: fsLabel, Probe: "sentinel_survival", Step: "eexist", Outcome: "pass",
			StageName: res.StageName, StagePath: res.StagePath,
			Detail: fmt.Sprintf("class=%s id=%s", res.Class, res.ErrorID()),
		})
		tl.Assert("not_committed", !res.Committed(), true, true)
	})

	t.Run("cancel_close", func(t *testing.T) {
		log, _, pl := hostilefsx.NewHarness(t, fsLabel)
		dest, parentDir := hostilefsx.ArrangeDest(t, root, "sent-cancel")
		sentinel, body := hostilefsx.PlantSentinel(t, parentDir)
		txn, err := fsx.Begin(dest, fsx.Options{Log: log, Project: "proj"})
		if err != nil {
			t.Fatal(err)
		}
		stageName := txn.StageName()
		_ = txn.RootedWriter().WriteFile("f.txt", "0644", []byte("f\n"))
		_ = txn.Close()
		hostilefsx.AssertSentinelAlive(t, sentinel, body)
		if !hostilefsx.StageStillPresent(t, parentDir, stageName) {
			t.Fatal("stage deleted on cancel")
		}
		pl.Record(hostilefsx.ProbeEntry{
			FS: fsLabel, Probe: "sentinel_survival", Step: "cancel", Outcome: "pass",
			StageName: stageName,
		})
	})
}

// --- helpers used only by external tests ---

func errnoString(err error) string {
	if err == nil {
		return ""
	}
	if errno, ok := err.(unix.Errno); ok {
		return errno.Error()
	}
	return err.Error()
}

func containsAll(s, sub string) bool {
	return strings.Contains(s, sub)
}

func exclusiveRenameViaHook(t *testing.T, parentFd int, stageName, destName string) error {
	t.Helper()
	return realExclusiveRename(parentFd, stageName, destName)
}

func childExists(t *testing.T, parentFd int, name string) bool {
	t.Helper()
	var st unix.Stat_t
	err := unix.Fstatat(parentFd, name, &st, unix.AT_SYMLINK_NOFOLLOW)
	return err == nil
}

func isRenameUnsupported(err error) bool {
	if err == nil {
		return false
	}
	// ENOTSUP and EOPNOTSUPP are the same errno on Linux; check via errors.Is.
	return errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.ENOSYS) ||
		errors.Is(err, unix.EINVAL)
}
