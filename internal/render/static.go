package render

import (
	"fmt"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// CatalogReader resolves catalog source bytes by catalog-relative path.
// *catalog.Catalog satisfies this interface.
type CatalogReader interface {
	Read(path string) ([]byte, error)
}

// StaticJob is one static-copy render request (destination path + catalog source).
//
// Source is the catalog-relative source path (the source id recorded in the
// inventory). Mode is the planned mode string applied to the output entry
// (v1.0 catalog files use "0644"; "0755" is admitted for directory-like /
// executable mode matrix tests at the render layer).
type StaticJob struct {
	// Path is the destination-relative output path.
	Path string
	// Mode is the planned file mode ("0644", "0755", …).
	Mode string
	// Source is the catalog-relative source path (source id).
	Source string
	// Owner is an optional stable owner label (core, archetype:<id>, …).
	Owner string
}

// knownModes are modes the static path will apply without rewriting (REQ-094
// file 0644 / directory 0755; render records whatever catalog/plan declares).
var knownModes = map[string]struct{}{
	"0644": {},
	"0755": {},
}

// RenderStatic copies one catalog source unchanged into a pure buffer and
// returns a single-entry Inventory (mechanism=static).
//
// Pipeline (P1.5.a):
//  1. Refuse unsafe output paths (fs.unsafe_path)
//  2. Validate mode
//  3. Read source bytes from the catalog (missing → catalog.invalid)
//  4. Digest source; copy bytes; digest output (equal for static)
//  5. Optionally write through w (MemoryWriter in P1; RootedWriter in P2)
//
// When w is nil, content is retained only in the returned Inventory.
func RenderStatic(cat CatalogReader, job StaticJob, w Writer) (*Inventory, error) {
	entry, content, err := renderStaticOne(cat, job)
	if err != nil {
		return nil, err
	}
	if w != nil {
		if err := w.WriteFile(entry.Path, entry.Mode, content); err != nil {
			return nil, wrapWriteError(entry.Path, err)
		}
	}
	return newInventory([]Entry{entry}, map[string][]byte{entry.Path: content}), nil
}

// RenderStaticAll renders multiple static jobs into one sorted Inventory.
// Fail-closed: the first error aborts with no partial inventory returned.
// When w is non-nil, each successful file is written before the next job.
func RenderStaticAll(cat CatalogReader, jobs []StaticJob, w Writer) (*Inventory, error) {
	if cat == nil {
		return nil, diagnostic.New(
			diagnostic.IDCatalogInvalid,
			"catalog reader is nil",
			diagnostic.PathLocation("catalog"),
		)
	}
	entries := make([]Entry, 0, len(jobs))
	content := make(map[string][]byte, len(jobs))
	seen := make(map[string]struct{}, len(jobs))

	for i, job := range jobs {
		entry, data, err := renderStaticOne(cat, job)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[entry.Path]; dup {
			return nil, diagnostic.Newf(
				diagnostic.IDRenderFailed,
				diagnostic.PathLocation(entry.Path),
				"static render job %d duplicates output path %q",
				i, entry.Path,
			)
		}
		seen[entry.Path] = struct{}{}
		if w != nil {
			if err := w.WriteFile(entry.Path, entry.Mode, data); err != nil {
				return nil, wrapWriteError(entry.Path, err)
			}
		}
		entries = append(entries, entry)
		content[entry.Path] = data
	}
	return newInventory(entries, content), nil
}

func renderStaticOne(cat CatalogReader, job StaticJob) (Entry, []byte, error) {
	if cat == nil {
		return Entry{}, nil, diagnostic.New(
			diagnostic.IDCatalogInvalid,
			"catalog reader is nil",
			diagnostic.PathLocation("catalog"),
		)
	}

	outPath, err := safeOutputPath(job.Path)
	if err != nil {
		return Entry{}, nil, err
	}

	mode := strings.TrimSpace(job.Mode)
	if mode == "" {
		return Entry{}, nil, diagnostic.Newf(
			diagnostic.IDRenderFailed,
			diagnostic.PathLocation(outPath),
			"static render mode is required for path %q",
			outPath,
		)
	}
	if _, ok := knownModes[mode]; !ok {
		return Entry{}, nil, diagnostic.Newf(
			diagnostic.IDRenderFailed,
			diagnostic.PathLocation(outPath),
			"static render mode %q is not admitted (allowed: 0644, 0755) for path %q",
			mode, outPath,
		)
	}

	src := strings.TrimSpace(job.Source)
	if src == "" {
		return Entry{}, nil, diagnostic.Newf(
			diagnostic.IDRenderFailed,
			diagnostic.PathLocation(outPath),
			"static render source is required for path %q",
			outPath,
		)
	}
	// Source path must also refuse escape (catalog already validates; render
	// planning layer re-checks so unit tests can exercise fail-closed behavior
	// without a hostile catalog).
	if errMsg := sourcePathUnsafe(src); errMsg != "" {
		return Entry{}, nil, unsafePathError(src, "catalog source "+errMsg)
	}

	raw, err := cat.Read(src)
	if err != nil {
		// Preserve stable catalog.invalid (or other FoundryError) from the reader.
		if fe, ok := diagnostic.AsFoundryError(err); ok {
			return Entry{}, nil, fe
		}
		return Entry{}, nil, diagnostic.Wrap(
			diagnostic.IDCatalogInvalid,
			fmt.Sprintf("catalog source %q not readable", src),
			diagnostic.PathLocation(src),
			err,
		)
	}

	// Defensive copy: catalog may return shared buffers; static must not mutate.
	content := make([]byte, len(raw))
	copy(content, raw)

	srcDigest := ContentDigest(raw)
	outDigest := ContentDigest(content)
	// Static invariant: digests equal (byte-identical copy).
	if srcDigest != outDigest {
		return Entry{}, nil, diagnostic.Newf(
			diagnostic.IDRenderFailed,
			diagnostic.PathLocation(outPath),
			"static copy digest mismatch for %q: source=%s output=%s",
			outPath, srcDigest, outDigest,
		)
	}

	entry := Entry{
		Path:          outPath,
		Mode:          mode,
		Mechanism:     MechanismStatic,
		Source:        src,
		Owner:         strings.TrimSpace(job.Owner),
		SourceDigest:  srcDigest,
		ContentDigest: outDigest,
	}
	return entry, content, nil
}

// sourcePathUnsafe returns a non-empty reason when src is not a safe relative
// catalog source path.
func sourcePathUnsafe(src string) string {
	if src == "" {
		return "must be non-empty"
	}
	if strings.HasPrefix(src, "/") {
		return "must be relative (not absolute)"
	}
	if strings.Contains(src, "\\") {
		return "must use forward slashes only"
	}
	if strings.Contains(src, "\x00") {
		return "must not contain NUL"
	}
	for _, seg := range strings.Split(src, "/") {
		if seg == ".." {
			return `must not contain ".." segments`
		}
		if seg == "" {
			return "must not contain empty segments"
		}
	}
	return ""
}

func wrapWriteError(path string, err error) error {
	if fe, ok := diagnostic.AsFoundryError(err); ok {
		return fe
	}
	return diagnostic.Wrap(
		diagnostic.IDRenderFailed,
		fmt.Sprintf("write failed for %q", path),
		diagnostic.PathLocation(path),
		err,
	)
}
