//go:build unix

package fsx

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// Default file/directory modes from the plan inventory / Section 15.6.
const (
	// DefaultFileMode is the v1.0 plan inventory mode for regular files.
	DefaultFileMode = "0644"
	// DefaultDirMode is the v1.0 plan inventory mode for directories.
	DefaultDirMode = "0755"
)

// RootedWriter writes exclusively through a stage's descriptor-bound os.Root
// (Section 31.5 / REQ-125/184). It forbids absolute paths and ".." escapes,
// verifies stage identity before mutation, and applies exact modes via fchmod
// on open descriptors so the process umask cannot widen or narrow planned
// modes.
//
// RootedWriter never exposes a raw host path for mutation and never deletes
// the stage or any object under the destination parent.
//
// Method set intentionally omits Remove / RemoveAll / unlink. The signature
// of WriteFile matches render.Writer for structural wiring in generate
// without importing render (Section 42.3 layering).
type RootedWriter struct {
	stage *Stage
	log   StepLogger
}

// WriteFile creates path relative to the stage root with the planned mode
// string (e.g. "0644"), writes data, and fchmods the open descriptor to the
// exact mode. Parent directories are created at DefaultDirMode (0755) with
// fchmod when missing. Duplicate exclusive creates fail closed.
//
// path must be destination-relative: no absolute form, no ".." components.
func (w *RootedWriter) WriteFile(relPath, mode string, data []byte) error {
	if w == nil || w.stage == nil || w.stage.root == nil {
		return diagnostic.New(
			diagnostic.IDRenderFailed,
			"rooted writer is closed or nil",
			diagnostic.PathLocation(relPath),
		)
	}
	if err := w.stage.VerifyIdentity(); err != nil {
		return err
	}
	clean, err := validateRootedRelPath(relPath)
	if err != nil {
		return err
	}
	perm, err := parseMode(mode)
	if err != nil {
		return diagnostic.Wrap(
			diagnostic.IDRenderFailed,
			fmt.Sprintf("invalid planned mode %q for %s", mode, clean),
			diagnostic.PathLocation(clean),
			err,
		)
	}

	if err := w.ensureParentDirs(clean); err != nil {
		return err
	}

	// Exclusive create; initial mode is umask-affected — fchmod corrects it.
	f, err := w.stage.root.OpenFile(clean, os.O_CREATE|os.O_WRONLY|os.O_EXCL, perm)
	if err != nil {
		logStep(w.log, "rooted_writer", "open_create", "fail",
			fmt.Sprintf("path=%q mode=%s errno=%s", clean, mode, errnoString(err)))
		return diagnostic.Wrap(
			diagnostic.IDRenderFailed,
			fmt.Sprintf("cannot create staged file %q", clean),
			diagnostic.PathLocation(clean),
			err,
		)
	}
	defer f.Close()

	// Exact mode via fchmod on the open descriptor (File.Chmod → fchmod).
	if err := f.Chmod(perm); err != nil {
		logStep(w.log, "rooted_writer", "fchmod", "fail",
			fmt.Sprintf("path=%q mode=%s errno=%s", clean, mode, errnoString(err)))
		return diagnostic.Wrap(
			diagnostic.IDRenderFailed,
			fmt.Sprintf("cannot fchmod staged file %q to %s", clean, mode),
			diagnostic.PathLocation(clean),
			err,
		)
	}

	if _, err := f.Write(data); err != nil {
		logStep(w.log, "rooted_writer", "write", "fail",
			fmt.Sprintf("path=%q errno=%s", clean, errnoString(err)))
		return diagnostic.Wrap(
			diagnostic.IDRenderFailed,
			fmt.Sprintf("cannot write staged file %q", clean),
			diagnostic.PathLocation(clean),
			err,
		)
	}

	logStep(w.log, "rooted_writer", "write_file", "pass",
		fmt.Sprintf("path=%q mode=%s bytes=%d stage=%s", clean, formatMode(perm), len(data), w.stage.id))
	return nil
}

// Mkdir creates a single directory relative to the stage root with the planned
// mode (e.g. "0755"), applying fchmod on the created directory for exact mode.
// Parents must already exist (use MkdirAll for nested paths).
func (w *RootedWriter) Mkdir(relPath, mode string) error {
	if w == nil || w.stage == nil || w.stage.root == nil {
		return diagnostic.New(
			diagnostic.IDRenderFailed,
			"rooted writer is closed or nil",
			diagnostic.PathLocation(relPath),
		)
	}
	if err := w.stage.VerifyIdentity(); err != nil {
		return err
	}
	clean, err := validateRootedRelPath(relPath)
	if err != nil {
		return err
	}
	perm, err := parseMode(mode)
	if err != nil {
		return diagnostic.Wrap(
			diagnostic.IDRenderFailed,
			fmt.Sprintf("invalid planned mode %q for dir %s", mode, clean),
			diagnostic.PathLocation(clean),
			err,
		)
	}

	if err := w.stage.root.Mkdir(clean, perm); err != nil {
		logStep(w.log, "rooted_writer", "mkdir", "fail",
			fmt.Sprintf("path=%q mode=%s errno=%s", clean, mode, errnoString(err)))
		return diagnostic.Wrap(
			diagnostic.IDRenderFailed,
			fmt.Sprintf("cannot mkdir staged dir %q", clean),
			diagnostic.PathLocation(clean),
			err,
		)
	}
	// fchmod via Root.Chmod after create (descriptor-relative under the root).
	if err := w.stage.root.Chmod(clean, perm); err != nil {
		logStep(w.log, "rooted_writer", "dir_fchmod", "fail",
			fmt.Sprintf("path=%q mode=%s errno=%s", clean, mode, errnoString(err)))
		return diagnostic.Wrap(
			diagnostic.IDRenderFailed,
			fmt.Sprintf("cannot fchmod staged dir %q to %s", clean, mode),
			diagnostic.PathLocation(clean),
			err,
		)
	}
	logStep(w.log, "rooted_writer", "mkdir", "pass",
		fmt.Sprintf("path=%q mode=%s stage=%s", clean, formatMode(perm), w.stage.id))
	return nil
}

// MkdirAll creates every missing directory component of relPath with the
// planned directory mode (exact via fchmod). Existing components are left
// unchanged.
func (w *RootedWriter) MkdirAll(relPath, mode string) error {
	if w == nil || w.stage == nil || w.stage.root == nil {
		return diagnostic.New(
			diagnostic.IDRenderFailed,
			"rooted writer is closed or nil",
			diagnostic.PathLocation(relPath),
		)
	}
	if err := w.stage.VerifyIdentity(); err != nil {
		return err
	}
	clean, err := validateRootedRelPath(relPath)
	if err != nil {
		return err
	}
	perm, err := parseMode(mode)
	if err != nil {
		return diagnostic.Wrap(
			diagnostic.IDRenderFailed,
			fmt.Sprintf("invalid planned mode %q for dir %s", mode, clean),
			diagnostic.PathLocation(clean),
			err,
		)
	}

	// Create component-by-component so each new dir gets exact fchmod.
	parts := splitRel(clean)
	built := ""
	for _, p := range parts {
		if built == "" {
			built = p
		} else {
			built = path.Join(built, p)
		}
		st, statErr := w.stage.root.Lstat(built)
		if statErr == nil {
			if !st.IsDir() {
				return diagnostic.New(
					diagnostic.IDRenderFailed,
					fmt.Sprintf("staged path %q exists and is not a directory", built),
					diagnostic.PathLocation(built),
				)
			}
			continue
		}
		if !os.IsNotExist(statErr) {
			return diagnostic.Wrap(
				diagnostic.IDRenderFailed,
				fmt.Sprintf("cannot lstat staged path %q", built),
				diagnostic.PathLocation(built),
				statErr,
			)
		}
		if err := w.stage.root.Mkdir(built, perm); err != nil {
			// Race: another creator — re-stat.
			if os.IsExist(err) {
				continue
			}
			return diagnostic.Wrap(
				diagnostic.IDRenderFailed,
				fmt.Sprintf("cannot mkdir staged dir %q", built),
				diagnostic.PathLocation(built),
				err,
			)
		}
		if err := w.stage.root.Chmod(built, perm); err != nil {
			return diagnostic.Wrap(
				diagnostic.IDRenderFailed,
				fmt.Sprintf("cannot fchmod staged dir %q to %s", built, mode),
				diagnostic.PathLocation(built),
				err,
			)
		}
		logStep(w.log, "rooted_writer", "mkdir_component", "pass",
			fmt.Sprintf("path=%q mode=%s", built, formatMode(perm)))
	}
	return nil
}

func (w *RootedWriter) ensureParentDirs(filePath string) error {
	dir := path.Dir(filePath)
	if dir == "." || dir == "" {
		return nil
	}
	return w.MkdirAll(dir, DefaultDirMode)
}

// validateRootedRelPath rejects absolute paths, empty names, and ".." escapes
// before they reach os.Root (which also rejects them). Returns a cleaned
// relative path using forward slashes for os.Root.
func validateRootedRelPath(relPath string) (string, error) {
	if relPath == "" {
		return "", errUnsafePath("<empty>", "rooted path must be non-empty")
	}
	if strings.Contains(relPath, "\x00") {
		return "", errUnsafePath(relPath, "rooted path must not contain NUL")
	}
	// Normalize to slash form for component checks; os.Root on Unix accepts
	// either separator but clean relative form is required.
	slash := path.Clean("/" + strings.ReplaceAll(relPath, "\\", "/"))
	// path.Clean("/a/../b") → "/b"; path.Clean("/../x") → "/../x" then we detect.
	if !strings.HasPrefix(slash, "/") {
		return "", errUnsafePath(relPath, "rooted path failed clean")
	}
	trimmed := strings.TrimPrefix(slash, "/")
	if trimmed == "" || trimmed == "." {
		return "", errUnsafePath(relPath, "rooted path must not refer to the stage root itself as a file")
	}
	// Absolute form (caller passed "/abs" or similar).
	if strings.HasPrefix(relPath, "/") || path.IsAbs(relPath) {
		return "", errUnsafePath(relPath, "rooted path must be relative (absolute form rejected)")
	}
	// Windows-style absolute.
	if len(relPath) >= 2 && relPath[1] == ':' {
		return "", errUnsafePath(relPath, "rooted path must be relative (drive form rejected)")
	}
	for _, c := range strings.Split(trimmed, "/") {
		if c == ".." {
			return "", errUnsafePath(relPath, `rooted path must not contain ".." components`)
		}
		if c == "" {
			return "", errUnsafePath(relPath, "rooted path has empty component")
		}
	}
	// Original had ".." before Clean collapsed it.
	for _, c := range strings.Split(strings.ReplaceAll(relPath, "\\", "/"), "/") {
		if c == ".." {
			return "", errUnsafePath(relPath, `rooted path must not contain ".." components`)
		}
	}
	return trimmed, nil
}

func splitRel(rel string) []string {
	rel = path.Clean(rel)
	if rel == "." || rel == "" {
		return nil
	}
	return strings.Split(rel, "/")
}

func parseMode(mode string) (os.FileMode, error) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return 0, fmt.Errorf("empty mode")
	}
	// Accept "0644", "644", "0o644".
	s := mode
	if strings.HasPrefix(s, "0o") || strings.HasPrefix(s, "0O") {
		s = s[2:]
	}
	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, err
	}
	if v > 0o777 {
		return 0, fmt.Errorf("mode %s out of permission range", mode)
	}
	return os.FileMode(v), nil
}

func formatMode(m os.FileMode) string {
	return fmt.Sprintf("%04o", m.Perm())
}
