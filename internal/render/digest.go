package render

import (
	"crypto/sha256"
	"encoding/hex"
)

// DigestHex is the lowercase hex encoding of a content SHA-256 digest.
type DigestHex string

// String returns the hex digest (or empty).
func (d DigestHex) String() string { return string(d) }

// ContentDigest returns the SHA-256 hex digest of b (REQ-090 family for content).
// Static copy guarantees source digest == output digest for identical bytes.
func ContentDigest(b []byte) DigestHex {
	sum := sha256.Sum256(b)
	return DigestHex(hex.EncodeToString(sum[:]))
}
