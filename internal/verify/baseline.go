package verify

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// FileMeta is one non-.git path under the stage for ConformanceBaseline
// (Section 35.3 / FND-006 / REQ-126).
type FileMeta struct {
	// Rel is slash-separated path relative to the stage root.
	Rel string
	// Type is "file", "dir", "symlink", or "other".
	Type string
	// Mode is permission bits as 4-digit octal (e.g. "0644"); empty for symlink.
	Mode string
	// Size is content length for regular files; 0 otherwise.
	Size int64
	// Digest is SHA-256 hex of file bytes for regular files; empty otherwise.
	Digest string
}

// Diff describes one non-.git conformance divergence.
type Diff struct {
	Rel    string
	Reason string // "missing", "extra", "type", "mode", "bytes"
	Before string
	After  string
}

// ConformanceBaseline is the frozen post-tidy tree (Section 43 contract):
// paths, types, modes, byte digests, plus an aggregate digest.
//
// Immutable after Freeze. Compare / Conform detect any later drift as
// verify.unplanned_mutation.
type ConformanceBaseline struct {
	// Entries keyed by Rel.
	Entries map[string]FileMeta
	// Keys is sorted Rel order.
	Keys []string
	// AggregateDigest is SHA-256 over the canonical serialization of all
	// entries (path|type|mode|size|digest lines). Empty when frozen with no
	// entries is still a valid 64-hex digest of the empty materialization.
	AggregateDigest string
}

// SnapshotNonGit walks root and records every path except `.git` and its
// descendants. Symlinks are recorded as type symlink without following.
// Regular file digests are SHA-256 of content.
func SnapshotNonGit(root string) (ConformanceBaseline, error) {
	out := ConformanceBaseline{Entries: make(map[string]FileMeta)}
	if root == "" {
		return out, fmt.Errorf("verify: empty stage root for snapshot")
	}
	root = filepath.Clean(root)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if relSlash == ".git" || strings.HasPrefix(relSlash, ".git/") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		meta := FileMeta{Rel: relSlash}
		mode := info.Mode()
		switch {
		case mode.IsRegular():
			meta.Type = "file"
			meta.Mode = formatMode(mode.Perm())
			meta.Size = info.Size()
			sum, err := fileSHA256(path)
			if err != nil {
				return err
			}
			meta.Digest = sum
		case mode.IsDir():
			meta.Type = "dir"
			meta.Mode = formatMode(mode.Perm())
		case mode&os.ModeSymlink != 0:
			meta.Type = "symlink"
		default:
			meta.Type = "other"
			meta.Mode = formatMode(mode.Perm())
		}
		out.Entries[relSlash] = meta
		return nil
	})
	if err != nil {
		return ConformanceBaseline{}, err
	}
	keys := make([]string, 0, len(out.Entries))
	for k := range out.Entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out.Keys = keys
	out.AggregateDigest = aggregateDigest(out)
	return out, nil
}

// Freeze is an alias for SnapshotNonGit used after tidy mutation-set success
// to name the post-tidy approved tree (REQ-126).
func Freeze(root string) (ConformanceBaseline, error) {
	return SnapshotNonGit(root)
}

// Compare returns divergences of after relative to baseline (before).
// Empty slice means path/type/mode/byte equality for all non-.git entries.
func Compare(baseline, after ConformanceBaseline) []Diff {
	var diffs []Diff
	seen := make(map[string]struct{}, len(after.Entries))
	for _, k := range baseline.Keys {
		b := baseline.Entries[k]
		a, ok := after.Entries[k]
		if !ok {
			diffs = append(diffs, Diff{Rel: k, Reason: "missing", Before: b.summary(), After: ""})
			continue
		}
		seen[k] = struct{}{}
		if b.Type != a.Type {
			diffs = append(diffs, Diff{Rel: k, Reason: "type", Before: b.Type, After: a.Type})
			continue
		}
		if b.Mode != a.Mode {
			diffs = append(diffs, Diff{Rel: k, Reason: "mode", Before: b.Mode, After: a.Mode})
		}
		if b.Type == "file" && (b.Size != a.Size || b.Digest != a.Digest) {
			diffs = append(diffs, Diff{
				Rel: k, Reason: "bytes",
				Before: fmt.Sprintf("size=%d digest=%s", b.Size, b.Digest),
				After:  fmt.Sprintf("size=%d digest=%s", a.Size, a.Digest),
			})
		}
	}
	for _, k := range after.Keys {
		if _, ok := seen[k]; ok {
			continue
		}
		if _, inBefore := baseline.Entries[k]; inBefore {
			continue
		}
		a := after.Entries[k]
		diffs = append(diffs, Diff{Rel: k, Reason: "extra", Before: "", After: a.summary()})
	}
	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].Rel != diffs[j].Rel {
			return diffs[i].Rel < diffs[j].Rel
		}
		return diffs[i].Reason < diffs[j].Reason
	})
	return diffs
}

// Conform re-snapshots root and compares against baseline. On drift returns
// a *diagnostic.FoundryError with id verify.unplanned_mutation naming paths.
func Conform(baseline ConformanceBaseline, root string) ([]Diff, error) {
	after, err := SnapshotNonGit(root)
	if err != nil {
		return nil, diagnostic.Wrapf(
			diagnostic.IDVerifyFailed,
			diagnostic.StepLocation(CheckFinalConformance),
			err,
			"failed to snapshot stage for conformance: %v", err,
		)
	}
	diffs := Compare(baseline, after)
	if len(diffs) == 0 {
		return nil, nil
	}
	return diffs, unplannedMutationError(CheckFinalConformance, diffs, baseline.AggregateDigest, after.AggregateDigest)
}

// ConformAfterStep is Conform with a step-specific location (post-tool recheck).
func ConformAfterStep(baseline ConformanceBaseline, root, stepID string) ([]Diff, error) {
	after, err := SnapshotNonGit(root)
	if err != nil {
		return nil, diagnostic.Wrapf(
			diagnostic.IDVerifyFailed,
			diagnostic.StepLocation(stepID),
			err,
			"failed to snapshot stage after %s: %v", stepID, err,
		)
	}
	diffs := Compare(baseline, after)
	if len(diffs) == 0 {
		return nil, nil
	}
	return diffs, unplannedMutationError(stepID, diffs, baseline.AggregateDigest, after.AggregateDigest)
}

func unplannedMutationError(stepID string, diffs []Diff, beforeDig, afterDig string) error {
	paths := make([]string, 0, len(diffs))
	for _, d := range diffs {
		paths = append(paths, d.Rel+"("+d.Reason+")")
	}
	detail := formatDiffs(diffs)
	return diagnostic.Newf(
		diagnostic.IDVerifyUnplannedMutation,
		diagnostic.StepLocation(stepID),
		"staged tree deviates from frozen baseline after %s: %s (baseline=%s after=%s)",
		stepID, detail, shortDigest(beforeDig), shortDigest(afterDig),
	).WithRemediation(
		"A verification step mutated the staged tree unexpectedly. " +
			"Inspect the named divergent paths in the preserved stage and correct the tool or catalog " +
			"so verification is read-only (FND-006). Paths: " + strings.Join(paths, ", "),
	)
}

func formatDiffs(diffs []Diff) string {
	if len(diffs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(diffs))
	for _, d := range diffs {
		parts = append(parts, fmt.Sprintf("%s:%s", d.Rel, d.Reason))
	}
	return strings.Join(parts, ", ")
}

func (m FileMeta) summary() string {
	return fmt.Sprintf("type=%s mode=%s size=%d digest=%s", m.Type, m.Mode, m.Size, m.Digest)
}

func formatMode(p os.FileMode) string {
	return fmt.Sprintf("%04o", p&0o7777)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func aggregateDigest(b ConformanceBaseline) string {
	h := sha256.New()
	for _, k := range b.Keys {
		m := b.Entries[k]
		_, _ = fmt.Fprintf(h, "%s|%s|%s|%d|%s\n", m.Rel, m.Type, m.Mode, m.Size, m.Digest)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func shortDigest(d string) string {
	if len(d) <= 12 {
		return d
	}
	return d[:12]
}

// PathSet returns the set of relative paths in the baseline/snapshot.
func (b ConformanceBaseline) PathSet() map[string]struct{} {
	out := make(map[string]struct{}, len(b.Entries))
	for k := range b.Entries {
		out[k] = struct{}{}
	}
	return out
}
