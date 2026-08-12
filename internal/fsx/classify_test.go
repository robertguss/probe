//go:build unix

package fsx_test

import (
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/fsx"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"golang.org/x/sys/unix"
)

// TestClassifyCommit_Section318Matrix covers the full Section 31.8 identity
// classification table (unit; no filesystem).
func TestClassifyCommit_Section318Matrix(t *testing.T) {
	recorded := fsx.FileID{Dev: 1, Ino: 100}
	log := testutil.New(t)

	cases := []struct {
		name string
		obs  fsx.CommitObservation
		want fsx.CommitClass
	}{
		{
			name: "syscall_ok_dest_matches",
			obs: fsx.CommitObservation{
				SyscallOK: true, RecordedStage: recorded,
				DestPresent: true, DestID: recorded,
			},
			want: fsx.ClassCommitted,
		},
		{
			name: "syscall_ok_dest_mismatch_ambiguous",
			obs: fsx.CommitObservation{
				SyscallOK: true, RecordedStage: recorded,
				DestPresent: true, DestID: fsx.FileID{Dev: 1, Ino: 999},
			},
			want: fsx.ClassAmbiguous,
		},
		{
			name: "syscall_ok_dest_absent_ambiguous",
			obs: fsx.CommitObservation{
				SyscallOK: true, RecordedStage: recorded,
				DestPresent: false,
			},
			want: fsx.ClassAmbiguous,
		},
		{
			name: "fail_stage_present_dest_absent_uncommitted",
			obs: fsx.CommitObservation{
				SyscallOK: false, RecordedStage: recorded,
				StagePresent: true, StageID: recorded,
				DestPresent: false,
			},
			want: fsx.ClassUncommitted,
		},
		{
			name: "fail_false_negative_committed",
			obs: fsx.CommitObservation{
				SyscallOK: false, RecordedStage: recorded,
				StagePresent: false,
				DestPresent:  true, DestID: recorded,
			},
			want: fsx.ClassCommitted,
		},
		{
			name: "fail_dest_other_identity_conflict",
			obs: fsx.CommitObservation{
				SyscallOK: false, RecordedStage: recorded,
				StagePresent: true, StageID: recorded,
				DestPresent: true, DestID: fsx.FileID{Dev: 1, Ino: 777},
			},
			want: fsx.ClassConflict,
		},
		{
			name: "fail_both_absent_ambiguous",
			obs: fsx.CommitObservation{
				SyscallOK: false, RecordedStage: recorded,
				StagePresent: false, DestPresent: false,
			},
			want: fsx.ClassAmbiguous,
		},
		{
			name: "fail_stage_wrong_id_dest_absent_ambiguous",
			obs: fsx.CommitObservation{
				SyscallOK: false, RecordedStage: recorded,
				StagePresent: true, StageID: fsx.FileID{Dev: 1, Ino: 50},
				DestPresent: false,
			},
			want: fsx.ClassAmbiguous,
		},
	}

	log.Phase("assert")
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, detail := fsx.ClassifyCommit(tc.obs)
			log.Assert(tc.name, got == tc.want, tc.want, got)
			if got != tc.want {
				t.Fatalf("class=%s want=%s detail=%s", got, tc.want, detail)
			}
			t.Logf("pass class=%s detail=%s", got, detail)
		})
	}
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestMapUncommittedError_Section319 covers errno → identifier for the
// uncommitted row of the Section 31.9 matrix.
func TestMapUncommittedError_Section319(t *testing.T) {
	log := testutil.New(t)
	log.Phase("assert")

	cases := []struct {
		name string
		err  error
		want string
	}{
		{"eexist", unix.EEXIST, string(diagnostic.IDFSDestinationExists)},
		{"enotsup", unix.ENOTSUP, string(diagnostic.IDFSRenameUnsupported)},
		{"eopnotsupp", unix.EOPNOTSUPP, string(diagnostic.IDFSRenameUnsupported)},
		{"einval", unix.EINVAL, string(diagnostic.IDFSRenameUnsupported)},
		{"exdev", unix.EXDEV, string(diagnostic.IDFSCommitFailed)},
		{"eio", unix.EIO, string(diagnostic.IDFSCommitFailed)},
		{"nil", nil, string(diagnostic.IDFSCommitFailed)},
	}
	for _, tc := range cases {
		id, msg := fsx.MapUncommittedErrorForTest(tc.err, ".foundry-proj-aaaa", "proj")
		log.Assert(tc.name, id == tc.want, tc.want, id)
		if id != tc.want {
			t.Fatalf("%s: id=%s want=%s msg=%s", tc.name, id, tc.want, msg)
		}
		if msg == "" {
			t.Fatalf("%s: empty message", tc.name)
		}
	}
	log.PhaseEnd("assert", testutil.OutcomeOK)
}
