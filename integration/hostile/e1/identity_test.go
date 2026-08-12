package e1

import "testing"

func TestClassifyCommit_Matrix(t *testing.T) {
	recorded := FileID{Dev: 1, Ino: 100}

	cases := []struct {
		name  string
		obs   CommitObservation
		want  CommitClass
		errID string // expected MapUncommitted when uncommitted; optional
	}{
		{
			name: "syscall_ok_dest_matches",
			obs: CommitObservation{
				SyscallOK: true, RecordedStage: recorded,
				DestPresent: true, DestID: recorded,
			},
			want: ClassCommitted,
		},
		{
			name: "syscall_ok_dest_mismatch_ambiguous",
			obs: CommitObservation{
				SyscallOK: true, RecordedStage: recorded,
				DestPresent: true, DestID: FileID{Dev: 1, Ino: 999},
			},
			want: ClassAmbiguous,
		},
		{
			name: "fail_stage_present_dest_absent_uncommitted",
			obs: CommitObservation{
				SyscallOK: false, RecordedStage: recorded,
				StagePresent: true, StageID: recorded,
				DestPresent: false,
			},
			want: ClassUncommitted,
		},
		{
			name: "fail_false_negative_committed",
			obs: CommitObservation{
				SyscallOK: false, RecordedStage: recorded,
				StagePresent: false,
				DestPresent:  true, DestID: recorded,
			},
			want: ClassCommitted,
		},
		{
			name: "fail_dest_other_identity_conflict",
			obs: CommitObservation{
				SyscallOK: false, RecordedStage: recorded,
				StagePresent: true, StageID: recorded,
				DestPresent: true, DestID: FileID{Dev: 1, Ino: 777},
			},
			want: ClassConflict,
		},
		{
			name: "fail_both_absent_ambiguous",
			obs: CommitObservation{
				SyscallOK: false, RecordedStage: recorded,
				StagePresent: false, DestPresent: false,
			},
			want: ClassAmbiguous,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, detail := ClassifyCommit(tc.obs)
			if got != tc.want {
				t.Fatalf("class=%s want=%s detail=%s", got, tc.want, detail)
			}
			t.Logf("pass class=%s detail=%s", got, detail)
		})
	}
}

func TestMapUncommittedError_RenameUnsupported(t *testing.T) {
	if id := MapUncommittedError(errExist); id != ErrDestinationExists {
		t.Fatalf("EEXIST -> %s", id)
	}
}

func TestMapUncommittedError_ENOTSUP_EINVAL(t *testing.T) {
	// Platform errnos exercised in errno_unix.go.
	for _, err := range renameUnsupportedSamples() {
		if id := MapUncommittedError(err); id != ErrRenameUnsupported {
			t.Fatalf("%v -> %s want %s", err, id, ErrRenameUnsupported)
		}
	}
}
