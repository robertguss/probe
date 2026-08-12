package fsx

// DestinationClass classifies an existing destination for agent-actionable
// refusal logs (REQ-003 destination refusal matrix). All classes fail with
// the same identifier fs.destination_exists; the class is logged for
// remediation clarity.
type DestinationClass string

const (
	// ClassAbsent means no destination object exists (preflight success).
	ClassAbsent DestinationClass = "absent"
	// ClassRegularFile is an existing regular file at the destination basename.
	ClassRegularFile DestinationClass = "regular_file"
	// ClassEmptyDir is an existing empty directory.
	ClassEmptyDir DestinationClass = "empty_dir"
	// ClassNonEmptyDir is an existing non-empty directory (no .git entry).
	ClassNonEmptyDir DestinationClass = "non_empty_dir"
	// ClassSymlink is a symbolic link at the destination basename (any target).
	ClassSymlink DestinationClass = "symlink"
	// ClassGitRepo is an existing directory that contains a .git entry.
	ClassGitRepo DestinationClass = "git_repo"
	// ClassOther is any other existing object type (socket, fifo, device, …).
	ClassOther DestinationClass = "other"
)

// String returns the stable class token.
func (c DestinationClass) String() string { return string(c) }
