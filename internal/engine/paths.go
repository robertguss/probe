package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var containNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// containPath joins root/name only when name is a single slug segment and the
// result is a proper child of root (not root itself, not an escape).
func containPath(root, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || filepath.IsAbs(name) {
		return "", fmt.Errorf("invalid path name %q", name)
	}
	if strings.ContainsRune(name, '/') || strings.ContainsRune(name, '\\') {
		return "", fmt.Errorf("invalid path name %q", name)
	}
	if !containNameRe.MatchString(name) {
		return "", fmt.Errorf("invalid path name %q", name)
	}
	joined := filepath.Join(root, name)
	rel, err := filepath.Rel(root, joined)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes root")
	}
	if rel != name {
		return "", fmt.Errorf("path escapes root")
	}
	return joined, nil
}

// SpikePaths is the on-disk layout under .probe/.
type SpikePaths struct {
	Root      string
	Config    string
	Session   string
	Requests  string
	Responses string
	Notes     string
	Log       string
}

// CatalogPaths is the catalog root under $PROBE_CATALOG.
type CatalogPaths struct {
	Root string
}

// APIPaths is one API directory inside the catalog.
type APIPaths struct {
	Root     string
	APIYAML  string
	Fixtures string
	Notes    string
}

func spikePathsFor(root string) SpikePaths {
	return SpikePaths{
		Root:      root,
		Config:    filepath.Join(root, "config.yaml"),
		Session:   filepath.Join(root, "session.json"),
		Requests:  filepath.Join(root, "requests"),
		Responses: filepath.Join(root, "responses"),
		Notes:     filepath.Join(root, "notes.md"),
		Log:       filepath.Join(root, "log.jsonl"),
	}
}

func (e *Engine) cwd() (string, error) {
	if e.opts.Getwd != nil {
		return e.opts.Getwd()
	}
	return os.Getwd()
}

func (e *Engine) resolveSpike(cwd string) SpikePaths {
	if e.opts.SpikeDir != "" {
		return spikePathsFor(e.opts.SpikeDir)
	}
	return spikePathsFor(filepath.Join(cwd, ".probe"))
}

func (e *Engine) openSpike() (SpikePaths, error) {
	cwd, err := e.cwd()
	if err != nil {
		return SpikePaths{}, err
	}
	return e.resolveSpike(cwd), nil
}

func (e *Engine) requireSpike(command string) (SpikePaths, Result, bool) {
	sp, err := e.openSpike()
	if err != nil {
		return SpikePaths{}, e.fail(command, ExitTransport, "transport", err.Error(), "", nil), false
	}
	if _, err := os.Stat(sp.Root); err != nil {
		return SpikePaths{}, e.usageError(command, "not a probe workspace (no .probe/); run init first", "probe init --json"), false
	}
	return sp, Result{}, true
}

func (e *Engine) resolveCatalog() (CatalogPaths, error) {
	if e.opts.CatalogDir != "" {
		return CatalogPaths{Root: e.opts.CatalogDir}, nil
	}
	if v := e.environ["PROBE_CATALOG"]; v != "" {
		return CatalogPaths{Root: v}, nil
	}
	if xdg := e.environ["XDG_DATA_HOME"]; xdg != "" {
		return CatalogPaths{Root: filepath.Join(xdg, "probe", "catalog")}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return CatalogPaths{}, err
	}
	return CatalogPaths{Root: filepath.Join(home, ".local", "share", "probe", "catalog")}, nil
}

func catalogAPI(cat CatalogPaths, api string) (APIPaths, error) {
	root, err := containPath(cat.Root, api)
	if err != nil {
		return APIPaths{}, err
	}
	return APIPaths{
		Root:     root,
		APIYAML:  filepath.Join(root, "api.yaml"),
		Fixtures: filepath.Join(root, "fixtures"),
		Notes:    filepath.Join(root, "notes.md"),
	}, nil
}
