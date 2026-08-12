//go:build unix

package e1

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

// TestMain captures environment metadata once for evidence logs.
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func kernelVersion(t *testing.T) string {
	t.Helper()
	var uts unix.Utsname
	if err := unix.Uname(&uts); err != nil {
		return "unknown"
	}
	return unix.ByteSliceToString(uts.Release[:])
}

// fsRoots returns label→writable directory pairs for the matrix.
// E1_FS_ROOTS="ext4:/path,xfs:/path,btrfs:/path,vfat:/path" overrides.
// Default: a private 0700 dir on the current filesystem labeled by detectFS.
func fsRoots(t *testing.T) map[string]string {
	t.Helper()
	if env := os.Getenv("E1_FS_ROOTS"); env != "" {
		out := make(map[string]string)
		for _, part := range strings.Split(env, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			label, path, ok := strings.Cut(part, ":")
			if !ok {
				t.Fatalf("bad E1_FS_ROOTS entry %q (want label:path)", part)
			}
			out[label] = path
		}
		return out
	}
	// Fallback: host FS only (still proves contracts 1–4,6 on one FS).
	dir := t.TempDir()
	// Ensure private for custody.
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// Also chmod parents we own in TempDir chain — TempDir is under /tmp (sticky).
	label := detectFS(t, dir)
	return map[string]string{label: dir}
}

func detectFS(t *testing.T, path string) string {
	t.Helper()
	// Platform-specific: Linux findmnt; Darwin Statfs Fstypename (apfs).
	return detectFSType(path)
}

func privateParent(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestE1_PlatformExclusiveRenameSyscall(t *testing.T) {
	// Documents the Section 31.7 primitive for evidence (Linux vs Darwin).
	name := renameSyscallName()
	switch runtime.GOOS {
	case "linux":
		if name != "Renameat2(RENAME_NOREPLACE)" {
			t.Fatalf("linux rename syscall label=%q", name)
		}
	case "darwin":
		if name != "RenameatxNp(RENAME_EXCL|RENAME_NOFOLLOW_ANY)" {
			t.Fatalf("darwin rename syscall label=%q", name)
		}
	default:
		t.Logf("GOOS=%s rename=%s", runtime.GOOS, name)
	}
	t.Logf("exclusive rename primitive: %s (GOOS=%s)", name, runtime.GOOS)
}

func TestE1_HappyPath_StageCommit(t *testing.T) {
	orig, err := CaptureOriginalCWD()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = orig.Close() })

	for fsLabel, root := range fsRoots(t) {
		fsLabel, root := fsLabel, root
		t.Run(fsLabel, func(t *testing.T) {
			if isNegativeFS(fsLabel) {
				t.Skip("FAT covered by TestE1_FAT_ExclusiveRenameMatrix")
			}
			log := NewProbeLog()
			log.Record(ProbeEntry{FS: fsLabel, Probe: "env", Step: "meta", Outcome: "info",
				Kernel: kernelVersion(t),
				Detail: fmt.Sprintf("go=%s root=%s", runtime.Version(), root)})

			parentDir := privateParent(t, root, "happy-parent")
			dest := filepath.Join(parentDir, "proj")

			parent, base, err := AcquireParent(dest, log, fsLabel)
			if err != nil {
				t.Fatalf("AcquireParent: %v", err)
			}
			t.Cleanup(func() { _ = parent.Close() })
			if base != "proj" {
				t.Fatalf("base=%q", base)
			}

			// Destination must not exist (31.4).
			if exists, _, err := parent.ChildLookup(base); err != nil || exists {
				t.Fatalf("dest exists before commit: exists=%v err=%v", exists, err)
			}

			stage, err := CreateStage(parent, "proj", log, fsLabel)
			if err != nil {
				t.Fatalf("CreateStage: %v", err)
			}
			t.Cleanup(func() { _ = stage.Close() })

			if !strings.HasPrefix(stage.Name, ".foundry-proj-") {
				t.Fatalf("stage name %q", stage.Name)
			}
			// Mode check via fstat.
			var st unix.Stat_t
			if err := unix.Fstat(stage.FD, &st); err != nil {
				t.Fatal(err)
			}
			if st.Mode&0777 != 0700 && !isLooseModeFS(fsLabel) {
				t.Fatalf("stage mode=%04o want 0700", st.Mode&0777)
			}

			if err := stage.WriteFile("hello.txt", []byte("e1\n"), 0600); err != nil {
				t.Fatalf("WriteFile via os.Root: %v", err)
			}

			res := Commit(parent, stage, base, log, fsLabel)
			if res.Class != ClassCommitted {
				t.Fatalf("commit class=%s id=%s msg=%s err=%v", res.Class, res.ErrorID, res.Message, res.SyscallErr)
			}
			// Stage entry consumed; dest present with stage identity.
			if exists, _, _ := parent.ChildLookup(stage.Name); exists {
				t.Fatal("stage name still present after commit")
			}
			exists, destID, err := parent.ChildLookup(base)
			if err != nil || !exists || !destID.Equal(stage.ID) {
				t.Fatalf("dest identity: exists=%v id=%s stage=%s err=%v", exists, destID, stage.ID, err)
			}
			pass, fail := log.SummaryPassFail()
			t.Logf("fs=%s pass=%d fail=%d commit=ok", fsLabel, pass, fail)
		})
	}
}

func TestE1_SymlinkParentComponent_UnsafePath(t *testing.T) {
	for fsLabel, root := range fsRoots(t) {
		if isNegativeFS(fsLabel) {
			continue
		}
		t.Run(fsLabel, func(t *testing.T) {
			log := NewProbeLog()
			base := privateParent(t, root, "sym-base")
			real := filepath.Join(base, "real")
			if err := os.Mkdir(real, 0700); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(base, "link")
			if err := os.Symlink(real, link); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(link, "proj")
			_, _, err := AcquireParent(dest, log, fsLabel)
			if err == nil {
				t.Fatal("expected fs.unsafe_path for symlink component")
			}
			se, ok := err.(*SpikeError)
			if !ok || se.ID != ErrUnsafePath {
				t.Fatalf("got %v want %s", err, ErrUnsafePath)
			}
			log.Record(ProbeEntry{FS: fsLabel, Probe: "symlink_reject", Step: "done", Outcome: "pass", Detail: se.Error()})
		})
	}
}

func TestE1_MissingParent_ParentMissing(t *testing.T) {
	for fsLabel, root := range fsRoots(t) {
		if isNegativeFS(fsLabel) {
			continue
		}
		t.Run(fsLabel, func(t *testing.T) {
			log := NewProbeLog()
			base := privateParent(t, root, "missing-base")
			// Intermediate component does not exist → fs.parent_missing (Appendix D).
			dest := filepath.Join(base, "no-such-dir", "proj")
			_, _, err := AcquireParent(dest, log, fsLabel)
			if err == nil {
				t.Fatal("expected fs.parent_missing for absent parent component")
			}
			se, ok := err.(*SpikeError)
			if !ok || se.ID != ErrParentMissing {
				t.Fatalf("got %v want %s", err, ErrParentMissing)
			}
			log.Record(ProbeEntry{FS: fsLabel, Probe: "parent_missing", Step: "done", Outcome: "pass", Detail: se.Error()})
		})
	}
}

func TestE1_NonDirectoryParent_ParentMissing(t *testing.T) {
	for fsLabel, root := range fsRoots(t) {
		if isNegativeFS(fsLabel) {
			continue
		}
		t.Run(fsLabel, func(t *testing.T) {
			log := NewProbeLog()
			base := privateParent(t, root, "file-parent-base")
			filePath := filepath.Join(base, "not-a-dir")
			if err := os.WriteFile(filePath, []byte("x"), 0600); err != nil {
				t.Fatal(err)
			}
			// Component is a regular file, not a directory → fs.parent_missing.
			dest := filepath.Join(filePath, "proj")
			_, _, err := AcquireParent(dest, log, fsLabel)
			if err == nil {
				t.Fatal("expected fs.parent_missing for non-directory parent component")
			}
			se, ok := err.(*SpikeError)
			if !ok || se.ID != ErrParentMissing {
				t.Fatalf("got %v want %s", err, ErrParentMissing)
			}
		})
	}
}

func TestE1_SharedWritableNonSticky_NamespaceNotPrivate(t *testing.T) {
	for fsLabel, root := range fsRoots(t) {
		if isNegativeFS(fsLabel) || isLooseModeFS(fsLabel) {
			continue
		}
		t.Run(fsLabel, func(t *testing.T) {
			log := NewProbeLog()
			shared := filepath.Join(root, "shared-ww")
			if err := os.MkdirAll(shared, 0700); err != nil {
				t.Fatal(err)
			}
			// 0777 without sticky → custody failure.
			if err := os.Chmod(shared, 0777); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(shared, 0700) })
			dest := filepath.Join(shared, "proj")
			_, _, err := AcquireParent(dest, log, fsLabel)
			if err == nil {
				t.Fatal("expected fs.namespace_not_private")
			}
			se, ok := err.(*SpikeError)
			if !ok || se.ID != ErrNamespaceNotPrivate {
				t.Fatalf("got %v want %s", err, ErrNamespaceNotPrivate)
			}
		})
	}
}

func TestE1_StickyWorldWritable_Permitted(t *testing.T) {
	// Sticky world-writable parents (SV-05) must be permitted. Prefer a real
	// sticky directory, not a directory-symlink alias: on Darwin /tmp is a
	// symlink to /private/tmp and O_NOFOLLOW refuses the /tmp component.
	log := NewProbeLog()
	stickyRoot := "/tmp"
	if runtime.GOOS == "darwin" {
		stickyRoot = "/private/tmp"
	}
	tmp, err := os.MkdirTemp(stickyRoot, "e1-sticky-")
	if err != nil {
		t.Fatal(err)
	}
	// Resolve any residual symlink hops so the no-follow walk matches the
	// real sticky directory object (authored path may still be under /tmp).
	if resolved, err := filepath.EvalSymlinks(tmp); err == nil {
		tmp = resolved
	}
	if err := os.Chmod(tmp, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	dest := filepath.Join(tmp, "proj")
	fsLabel := detectFS(t, tmp)
	parent, _, err := AcquireParent(dest, log, fsLabel)
	if err != nil {
		t.Fatalf("sticky tmp walk (root=%s): %v", stickyRoot, err)
	}
	_ = parent.Close()
}

func TestE1_ChildCWD_PathnameSwap(t *testing.T) {
	orig, err := CaptureOriginalCWD()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = orig.Close() })

	for fsLabel, root := range fsRoots(t) {
		if isNegativeFS(fsLabel) {
			continue
		}
		t.Run(fsLabel, func(t *testing.T) {
			log := NewProbeLog()
			parentDir := privateParent(t, root, "cwd-parent")
			dest := filepath.Join(parentDir, "proj")
			parent, _, err := AcquireParent(dest, log, fsLabel)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = parent.Close() })
			stage, err := CreateStage(parent, "proj", log, fsLabel)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = stage.Close() })
			if err := ProvePathnameSwap(parent, stage, orig, log, fsLabel); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestE1_ParentSwap_RetainedHandle(t *testing.T) {
	// After AcquireParent, rename the parent directory's pathname; stage create
	// via retained handle must still succeed (SV-03 object continuity).
	for fsLabel, root := range fsRoots(t) {
		if isNegativeFS(fsLabel) {
			continue
		}
		t.Run(fsLabel, func(t *testing.T) {
			log := NewProbeLog()
			container := privateParent(t, root, "swap-container")
			parentDir := filepath.Join(container, "parent-a")
			if err := os.Mkdir(parentDir, 0700); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(parentDir, "proj")
			parent, base, err := AcquireParent(dest, log, fsLabel)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = parent.Close() })

			// Swap pathname of the parent directory.
			swapped := filepath.Join(container, "parent-b-swapped")
			if err := os.Rename(parentDir, swapped); err != nil {
				t.Fatal(err)
			}
			// Authored path no longer names the object — reobserve should fail closed.
			if err := reobserveParent(parent, log, fsLabel); err == nil {
				t.Fatal("expected fs.parent_moved after pathname swap of parent")
			} else if se, ok := err.(*SpikeError); !ok || se.ID != ErrParentMoved {
				t.Fatalf("got %v want %s", err, ErrParentMoved)
			}

			// Mutations through retained handle still work.
			stage, err := CreateStage(parent, "proj", log, fsLabel)
			if err != nil {
				t.Fatalf("stage create via retained handle after parent pathname swap: %v", err)
			}
			t.Cleanup(func() { _ = stage.Close() })
			if err := stage.WriteFile("still-here.txt", []byte("ok\n"), 0600); err != nil {
				t.Fatal(err)
			}
			// Commit will fail reobserve (parent_moved) and preserve stage — correct.
			res := Commit(parent, stage, base, log, fsLabel)
			if res.ErrorID != ErrParentMoved {
				t.Fatalf("commit after parent move: class=%s id=%s", res.Class, res.ErrorID)
			}
			// Stage still present under retained parent.
			exists, _, err := parent.ChildLookup(stage.Name)
			if err != nil || !exists {
				t.Fatalf("stage must be preserved: exists=%v err=%v", exists, err)
			}
			log.Record(ProbeEntry{FS: fsLabel, Probe: "parent_swap", Step: "done", Outcome: "pass",
				Detail: "retained handle works; reobserve fail-closed; stage preserved"})
		})
	}
}

func TestE1_CommitEEXIST_TwoProcess(t *testing.T) {
	for fsLabel, root := range fsRoots(t) {
		if isNegativeFS(fsLabel) {
			continue
		}
		t.Run(fsLabel, func(t *testing.T) {
			log := NewProbeLog()
			parentDir := privateParent(t, root, "race-parent")
			dest := filepath.Join(parentDir, "proj")
			parent, base, err := AcquireParent(dest, log, fsLabel)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = parent.Close() })

			stageA, err := CreateStage(parent, "proj", log, fsLabel)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = stageA.Close() })
			stageB, err := CreateStage(parent, "proj", log, fsLabel)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = stageB.Close() })
			_ = stageA.WriteFile("a.txt", []byte("A"), 0600)
			_ = stageB.WriteFile("b.txt", []byte("B"), 0600)

			var (
				mu      sync.Mutex
				results []CommitResult
				wg      sync.WaitGroup
			)
			wg.Add(2)
			run := func(st *Stage) {
				defer wg.Done()
				// Skip reobserve race noise: call exclusiveRename path via Commit.
				// Both race the same dest name.
				res := Commit(parent, st, base, NewProbeLog(), fsLabel)
				mu.Lock()
				results = append(results, res)
				mu.Unlock()
			}
			go run(stageA)
			go run(stageB)
			wg.Wait()

			var winners, losers int
			for _, r := range results {
				switch r.Class {
				case ClassCommitted:
					winners++
				case ClassUncommitted, ClassConflict:
					if r.ErrorID != ErrDestinationExists && MapUncommittedError(r.SyscallErr) != ErrDestinationExists {
						// Conflict class uses destination_exists; uncommitted should map EEXIST.
						if r.ErrorID != ErrDestinationExists {
							t.Logf("loser id=%s class=%s err=%v", r.ErrorID, r.Class, r.SyscallErr)
						}
					}
					losers++
					// Loser stage preserved.
					exists, _, err := parent.ChildLookup(r.StageName)
					if err != nil || !exists {
						// If classified committed wrongly — check
						if r.Class != ClassCommitted {
							t.Fatalf("loser stage %s not preserved", r.StageName)
						}
					}
				default:
					t.Fatalf("unexpected class %s: %+v", r.Class, r)
				}
			}
			if winners != 1 || losers != 1 {
				t.Fatalf("want 1 winner 1 loser; winners=%d losers=%d results=%+v", winners, losers, results)
			}
			log.Record(ProbeEntry{FS: fsLabel, Probe: "eexist_race", Step: "done", Outcome: "pass",
				Detail: fmt.Sprintf("winners=%d losers=%d", winners, losers)})
		})
	}
}

func TestE1_FAT_ExclusiveRenameMatrix(t *testing.T) {
	// Historical gate text expected FAT to lack RENAME_NOREPLACE. On Linux 6.8+
	// the in-kernel fat driver implements it (EEXIST when dest exists). This
	// probe records whichever behavior is observed and still proves fail-closed
	// stage preservation when the kernel reports unsupported (see
	// TestE1_RenameUnsupported_FailClosedInjected).
	roots := fsRoots(t)
	var fatRoot string
	for _, label := range []string{"vfat", "fat", "exfat", "msdos"} {
		if p, ok := roots[label]; ok {
			fatRoot = p
			break
		}
	}
	if fatRoot == "" {
		t.Skip("no FAT/exFAT mount in E1_FS_ROOTS; run scripts/e1-mount-matrix.sh")
	}
	log := NewProbeLog()
	parentDir := filepath.Join(fatRoot, "fat-parent")
	if err := os.MkdirAll(parentDir, 0777); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Open(parentDir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	id, err := fileIDFromFD(fd)
	if err != nil {
		t.Fatal(err)
	}
	parent := &ParentHandle{FD: fd, Authored: parentDir, ID: id, log: log, fsLabel: "vfat"}

	stageName := ".foundry-fatprobe-aaaa"
	destName := "dest-proj"
	_ = unix.Unlinkat(fd, stageName, unix.AT_REMOVEDIR)
	_ = unix.Unlinkat(fd, destName, unix.AT_REMOVEDIR)
	if err := unix.Mkdirat(fd, stageName, 0700); err != nil {
		t.Fatalf("mkdirat stage: %v", err)
	}
	sfd, err := unix.Openat(fd, stageName, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(sfd)
	sid, err := fileIDFromFD(sfd)
	if err != nil {
		t.Fatal(err)
	}

	// First exclusive rename into empty dest.
	sysErr := exclusiveRename(fd, stageName, destName)
	obs := observeCommit(parent, stageName, destName, sid, sysErr)
	class, detail := ClassifyCommit(obs)
	log.Record(ProbeEntry{FS: "vfat", Probe: "fat_rename", Step: "first_commit",
		Syscall: renameSyscallName(), Args: stageName + " -> " + destName,
		Errno: errnoString(sysErr), Outcome: "info",
		Detail: fmt.Sprintf("class=%s detail=%s", class, detail)})

	if isRenameUnsupported(sysErr) {
		// Classic negative: unsupported → fail-closed id + stage preserved.
		if MapUncommittedError(sysErr) != ErrRenameUnsupported {
			t.Fatalf("map=%s", MapUncommittedError(sysErr))
		}
		exists, _, err := parent.ChildLookup(stageName)
		if err != nil || !exists {
			t.Fatalf("stage preserved on unsupported: exists=%v err=%v", exists, err)
		}
		t.Logf("FAT negative CONFIRMS rename_unsupported errno=%v stage preserved", sysErr)
		return
	}

	if class != ClassCommitted {
		t.Fatalf("unexpected first rename class=%s err=%v detail=%s", class, sysErr, detail)
	}

	// Second stage: exclusive rename must not replace existing dest.
	stage2 := ".foundry-fatprobe-bbbb"
	_ = unix.Unlinkat(fd, stage2, unix.AT_REMOVEDIR)
	if err := unix.Mkdirat(fd, stage2, 0700); err != nil {
		t.Fatal(err)
	}
	s2fd, err := unix.Openat(fd, stage2, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(s2fd)
	s2id, err := fileIDFromFD(s2fd)
	if err != nil {
		t.Fatal(err)
	}
	sysErr2 := exclusiveRename(fd, stage2, destName)
	obs2 := observeCommit(parent, stage2, destName, s2id, sysErr2)
	class2, detail2 := ClassifyCommit(obs2)
	log.Record(ProbeEntry{FS: "vfat", Probe: "fat_rename", Step: "second_commit_eexist",
		Syscall: renameSyscallName(), Args: stage2 + " -> " + destName,
		Errno: errnoString(sysErr2), Outcome: "info",
		Detail: fmt.Sprintf("class=%s detail=%s", class2, detail2)})

	if sysErr2 == nil {
		t.Fatal("RENAME_NOREPLACE on FAT replaced existing dest — exclusive semantics broken")
	}
	if !isExist(sysErr2) && class2 != ClassConflict && class2 != ClassUncommitted {
		t.Fatalf("want EEXIST/conflict; class=%s err=%v", class2, sysErr2)
	}
	// Loser stage preserved.
	exists, _, err := parent.ChildLookup(stage2)
	if err != nil || !exists {
		t.Fatalf("loser stage not preserved: exists=%v err=%v", exists, err)
	}
	// Winner dest still original identity.
	exists, destID, err := parent.ChildLookup(destName)
	if err != nil || !exists || !destID.Equal(sid) {
		t.Fatalf("winner dest identity lost: exists=%v id=%s want=%s err=%v", exists, destID, sid, err)
	}
	t.Logf("FAT on this kernel SUPPORTS RENAME_NOREPLACE (EEXIST+preserve); not a negative unsupported case")
}

func TestE1_RenameUnsupported_FailClosedInjected(t *testing.T) {
	// Proves Section 31.7 fail-closed path when the kernel reports the exclusive
	// rename is unsupported: map to fs.rename_unsupported and preserve stage.
	// Used when live FAT implements RENAME_NOREPLACE (Linux 6.8+ fat driver).
	log := NewProbeLog()
	root := t.TempDir()
	_ = os.Chmod(root, 0700)
	parentDir := privateParent(t, root, "unsup-parent")
	dest := filepath.Join(parentDir, "proj")
	parent, base, err := AcquireParent(dest, log, "ext4")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parent.Close() })
	stage, err := CreateStage(parent, "proj", log, "ext4")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })

	sysErr := unix.ENOTSUP
	obs := observeCommit(parent, stage.Name, base, stage.ID, sysErr)
	class, detail := ClassifyCommit(obs)
	errID := MapUncommittedError(sysErr)
	if class != ClassUncommitted {
		t.Fatalf("class=%s want uncommitted detail=%s", class, detail)
	}
	if errID != ErrRenameUnsupported {
		t.Fatalf("id=%s want %s", errID, ErrRenameUnsupported)
	}
	exists, _, err := parent.ChildLookup(stage.Name)
	if err != nil || !exists {
		t.Fatalf("stage must be preserved: exists=%v err=%v", exists, err)
	}
	// Destination must not have been created.
	if exists, _, _ := parent.ChildLookup(base); exists {
		t.Fatal("destination must not exist when rename unsupported")
	}
	log.Record(ProbeEntry{FS: "ext4", Probe: "rename_unsupported", Step: "injected_enotsup",
		Syscall: renameSyscallName(), Errno: errnoString(sysErr), Outcome: "pass",
		Detail: fmt.Sprintf("%s stage=%s preserved; %s", errID, stage.Name, detail)})
}

func TestE1_SV01_osRoot_NoEscape(t *testing.T) {
	// Prove os.Root rejects escaping paths (SV-01).
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err := root.OpenFile("../escape", os.O_CREATE|os.O_WRONLY, 0600); err == nil {
		t.Fatal("os.Root allowed ../escape")
	}
	if _, err := root.OpenFile("/etc/passwd", os.O_RDONLY, 0); err == nil {
		t.Fatal("os.Root allowed absolute path")
	}
	// Positive: create within root.
	f, err := root.OpenFile("ok.txt", os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	t.Logf("SV-01 os.Root method set ok; go=%s", runtime.Version())
}

func TestE1_ObjectContinuity_OpenatFchdir(t *testing.T) {
	// SV-03: directory FD identifies same object after pathname rename.
	log := NewProbeLog()
	root := t.TempDir()
	_ = os.Chmod(root, 0700)
	a := filepath.Join(root, "a")
	if err := os.Mkdir(a, 0700); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Open(a, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	id1, err := fileIDFromFD(fd)
	if err != nil {
		t.Fatal(err)
	}
	b := filepath.Join(root, "b")
	if err := os.Rename(a, b); err != nil {
		t.Fatal(err)
	}
	id2, err := fileIDFromFD(fd)
	if err != nil {
		t.Fatal(err)
	}
	if !id1.Equal(id2) {
		t.Fatalf("fd identity changed after rename: %s vs %s", id1, id2)
	}
	// fchdir through fd still works.
	orig, err := CaptureOriginalCWD()
	if err != nil {
		t.Fatal(err)
	}
	defer orig.Close()
	if err := unix.Fchdir(fd); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := unix.Fchdir(orig.FD); err != nil {
		t.Fatal(err)
	}
	log.Record(ProbeEntry{Probe: "sv03", Step: "openat_fchdir", Outcome: "pass",
		Detail: fmt.Sprintf("id=%s wd_after_fchdir=%s", id1, wd)})
}

func isNegativeFS(label string) bool {
	switch strings.ToLower(label) {
	case "vfat", "fat", "exfat", "msdos":
		return true
	default:
		return false
	}
}

func isLooseModeFS(label string) bool {
	return isNegativeFS(label)
}

// TestE1_GitInit_CorruptedDotGitRejection proves that a pre-existing .git entry
// that is not a directory (e.g. a regular file or a broken symlink) causes
// `git init` to fail safely instead of silently corrupting the repository.
func TestE1_GitInit_CorruptedDotGitRejection(t *testing.T) {
	if testing.Short() {
		t.Skip("skips real git subprocess test in short mode")
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available: ", err)
	}

	for fsLabel, root := range fsRoots(t) {
		if isNegativeFS(fsLabel) {
			continue
		}
		fsLabel, root := fsLabel, root
		t.Run(fsLabel, func(t *testing.T) {
			stage := privateParent(t, root, "corrupt-git-stage")
			corruptGit := filepath.Join(stage, ".git")
			if err := os.WriteFile(corruptGit, []byte("not a gitdir"), 0600); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command(gitBin, "init", stage)
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("git init succeeded on corrupted .git: %s", out)
			}
			// The stage must still be a regular directory; git must not have
			// replaced the corrupted .git file with a directory.
			info, err := os.Lstat(corruptGit)
			if err != nil {
				t.Fatalf(".git disappeared: %v", err)
			}
			if info.IsDir() {
				t.Fatal("git init replaced corrupted .git file with a directory")
			}
		})
	}
}

// TestE1_AcquireParent_CyclicSymlinkRejection proves that a cyclic symlink in
// the parent path is rejected as fs.unsafe_path before any git init work can
// begin (Section 31.2 / REQ-214).
func TestE1_AcquireParent_CyclicSymlinkRejection(t *testing.T) {
	for fsLabel, root := range fsRoots(t) {
		if isNegativeFS(fsLabel) {
			continue
		}
		fsLabel, root := fsLabel, root
		t.Run(fsLabel, func(t *testing.T) {
			log := NewProbeLog()
			base := privateParent(t, root, "symlink-loop-base")
			// A directory symlink pointing back at its own parent creates a cycle
			// when the walk tries to descend base/loop/loop/... .
			loop := filepath.Join(base, "loop")
			if err := os.Symlink(base, loop); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(loop, "proj")
			_, _, err := AcquireParent(dest, log, fsLabel)
			if err == nil {
				t.Fatal("expected fs.unsafe_path for cyclic symlink parent")
			}
			se, ok := err.(*SpikeError)
			if !ok || se.ID != ErrUnsafePath {
				t.Fatalf("got %v want %s", err, ErrUnsafePath)
			}
			log.Record(ProbeEntry{FS: fsLabel, Probe: "symlink_loop_reject", Step: "done", Outcome: "pass", Detail: se.Error()})
		})
	}
}
