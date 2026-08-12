package gitinit

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileMeta is one non-.git path under the stage for conformance re-check
// after git init (Section 29.2 stage 16 / FND-006 / REQ-128).
type FileMeta struct {
	// Rel is slash-separated path relative to the stage root.
	Rel  string
	Type string // "file", "dir", "symlink", "other"
	Mode string // permission bits as 4-digit octal (e.g. "0644"); empty for non-file/dir
	// Size is content length for regular files; 0 otherwise.
	Size int64
	// SHA256 hex of file bytes for regular files; empty otherwise.
	// Computed lazily only when Compare needs byte equality — stored at snapshot.
	Digest string
}

// Snapshot is an ordered inventory of every non-.git path under a stage.
type Snapshot struct {
	// Entries keyed by Rel; order of Keys is sorted.
	Entries map[string]FileMeta
	Keys    []string
}

// SnapshotNonGit walks root and records every path except `.git` and its
// descendants. Symlinks are recorded as type symlink without following.
// Regular file digests are SHA-256 of content.
func SnapshotNonGit(root string) (Snapshot, error) {
	out := Snapshot{Entries: make(map[string]FileMeta)}
	if root == "" {
		return out, fmt.Errorf("gitinit: empty stage root for snapshot")
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
		// Skip .git entirely (and anything under it).
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
		return Snapshot{}, err
	}
	keys := make([]string, 0, len(out.Entries))
	for k := range out.Entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out.Keys = keys
	return out, nil
}

// Diff describes one non-.git conformance divergence.
type Diff struct {
	Rel    string
	Reason string // "missing", "extra", "type", "mode", "bytes"
	Before string
	After  string
}

// CompareNonGit returns divergences of after relative to before.
// Empty slice means path/type/mode/byte equality for all non-.git entries.
func CompareNonGit(before, after Snapshot) []Diff {
	var diffs []Diff
	seen := make(map[string]struct{}, len(after.Entries))
	for _, k := range before.Keys {
		b := before.Entries[k]
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
		if _, inBefore := before.Entries[k]; inBefore {
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

func (m FileMeta) summary() string {
	return fmt.Sprintf("type=%s mode=%s size=%d digest=%s", m.Type, m.Mode, m.Size, m.Digest)
}

func formatMode(p os.FileMode) string {
	return fmt.Sprintf("%04o", p&0o7777)
}

func fileSHA256(path string) (string, error) {
	// Local import to keep snapshot.go self-contained.
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return sha256Hex(b), nil
}
