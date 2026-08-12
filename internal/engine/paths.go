package engine

import (
	"os"
	"path/filepath"
)

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

func (e *Engine) resolveSpike(cwd string) (SpikePaths, error) {
	if e.opts.SpikeDir != "" {
		return spikePathsFor(e.opts.SpikeDir), nil
	}
	return spikePathsFor(filepath.Join(cwd, ".probe")), nil
}

func (e *Engine) openSpike() (SpikePaths, error) {
	cwd, err := e.cwd()
	if err != nil {
		return SpikePaths{}, err
	}
	sp, err := e.resolveSpike(cwd)
	if err != nil {
		return SpikePaths{}, err
	}
	e.spike = sp
	return sp, nil
}

func (e *Engine) requireSpike() (SpikePaths, Result, bool) {
	sp, err := e.openSpike()
	if err != nil {
		return SpikePaths{}, e.fail("probe", ExitTransport, "transport", err.Error(), "", nil), false
	}
	if _, err := os.Stat(sp.Root); err != nil {
		return SpikePaths{}, e.usageError("probe", "not a probe workspace (no .probe/); run init first", "probe init --json"), false
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
	return CatalogPaths{}, nil
}

func catalogAPI(cat CatalogPaths, api string) APIPaths {
	root := filepath.Join(cat.Root, api)
	return APIPaths{
		Root:     root,
		APIYAML:  filepath.Join(root, "api.yaml"),
		Fixtures: filepath.Join(root, "fixtures"),
		Notes:    filepath.Join(root, "notes.md"),
	}
}
