package catalog

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io/fs"
	"path"
	"sort"
)

// DigestHex is the lowercase hex encoding of a catalog SHA-256 digest.
// Empty when unset.
type DigestHex string

// String returns the hex digest (or empty).
func (d DigestHex) String() string { return string(d) }

// DigestFS computes the deterministic catalog digest over all regular files
// in fsys (REQ-090, Section 24.1).
//
// Canonical form (stable under path separators and map iteration):
//
//	for each path in lexicographic order (forward-slash, cleaned):
//	  write uint64be(len(path)) || path_bytes || uint64be(len(content)) || content
//
// The digest is the SHA-256 of that concatenation, hex-encoded lowercase.
func DigestFS(fsys fs.FS) (DigestHex, error) {
	files, err := readAllFiles(fsys)
	if err != nil {
		return "", err
	}
	return DigestMap(files), nil
}

// DigestMap computes the digest from an in-memory path→content map.
// Paths are cleaned to forward-slash form before sorting.
func DigestMap(files map[string][]byte) DigestHex {
	paths := make([]string, 0, len(files))
	normalized := make(map[string][]byte, len(files))
	for p, b := range files {
		np := path.Clean("/" + p)
		if np == "/" {
			np = "."
		} else {
			np = np[1:] // strip leading /
		}
		// Prefer first write; callers should not pass duplicates.
		if _, exists := normalized[np]; !exists {
			normalized[np] = b
			paths = append(paths, np)
		}
	}
	sort.Strings(paths)

	h := sha256.New()
	var lenBuf [8]byte
	for _, p := range paths {
		content := normalized[p]
		binary.BigEndian.PutUint64(lenBuf[:], uint64(len(p)))
		_, _ = h.Write(lenBuf[:])
		_, _ = h.Write([]byte(p))
		binary.BigEndian.PutUint64(lenBuf[:], uint64(len(content)))
		_, _ = h.Write(lenBuf[:])
		_, _ = h.Write(content)
	}
	return DigestHex(hex.EncodeToString(h.Sum(nil)))
}
