package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// RateLimit is optional catalog pacing metadata.
type RateLimit struct {
	RPS      float64 `yaml:"rps,omitempty" json:"rps,omitempty"`
	RetryMax int     `yaml:"retry_max,omitempty" json:"retry_max,omitempty"`
	Notes    string  `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// CatalogEndpoint is one promoted endpoint in api.yaml.
type CatalogEndpoint struct {
	ID         string `yaml:"id" json:"id"`
	Method     string `yaml:"method" json:"method"`
	Path       string `yaml:"path" json:"path"`
	Auth       string `yaml:"auth,omitempty" json:"auth,omitempty"`
	Fixture    string `yaml:"fixture" json:"fixture"`
	LastStatus int    `yaml:"last_status,omitempty" json:"last_status,omitempty"`
	PromotedAt string `yaml:"promoted_at,omitempty" json:"promoted_at,omitempty"`
}

// CatalogAPI is the on-disk api.yaml model.
type CatalogAPI struct {
	Name      string            `yaml:"name" json:"name"`
	Base      string            `yaml:"base,omitempty" json:"base,omitempty"`
	RateLimit *RateLimit        `yaml:"rate_limit,omitempty" json:"rate_limit,omitempty"`
	Endpoints []CatalogEndpoint `yaml:"endpoints,omitempty" json:"endpoints,omitempty"`
}

type promoteInput struct {
	API      string
	Request  string
	Endpoint string
	DryRun   bool
}

var apiNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

func (e *Engine) requireCatalogRoot(command string) (CatalogPaths, Result, bool) {
	cat, err := e.resolveCatalog()
	if err != nil {
		return CatalogPaths{}, e.fail(command, ExitTransport, "transport", err.Error(), "", nil), false
	}
	if cat.Root == "" {
		return CatalogPaths{}, e.usageError(command, "catalog root not set", "PROBE_CATALOG=/path/to/catalog probe catalog list --json"), false
	}
	return cat, Result{}, true
}

func (e *Engine) loadCatalogAPI(paths APIPaths) (CatalogAPI, error) {
	b, err := os.ReadFile(paths.APIYAML)
	if err != nil {
		return CatalogAPI{}, err
	}
	var api CatalogAPI
	if err := yaml.Unmarshal(b, &api); err != nil {
		return CatalogAPI{}, fmt.Errorf("parse api.yaml: %w", err)
	}
	return api, nil
}

func (e *Engine) saveCatalogAPI(paths APIPaths, api CatalogAPI) error {
	if err := os.MkdirAll(paths.Fixtures, 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(&api)
	if err != nil {
		return err
	}
	return writeFileAtomic(paths.APIYAML, b, 0o644)
}

type catalogListData struct {
	APIs []string `json:"apis"`
	Root string   `json:"root"`
}

type catalogShowEndpointData struct {
	API       string          `json:"api"`
	Endpoint  CatalogEndpoint `json:"endpoint"`
	Base      string          `json:"base"`
	RateLimit *RateLimit      `json:"rate_limit,omitempty"`
}

type catalogPathData struct {
	API  string `json:"api"`
	Path string `json:"path"`
}

type promoteData struct {
	API      string          `json:"api"`
	Endpoint CatalogEndpoint `json:"endpoint"`
	Request  string          `json:"request"`
	DryRun   bool            `json:"dry_run"`
	Path     string          `json:"path"`
}

type catalogFixture struct {
	Request  savedRequestFile  `json:"request"`
	Response savedResponseFile `json:"response"`
}

func (e *Engine) catalogList(_ context.Context) Result {
	cat, res, ok := e.requireCatalogRoot("catalog list")
	if !ok {
		return res
	}
	entries, err := os.ReadDir(cat.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return e.ok("catalog list", catalogListData{APIs: []string{}, Root: cat.Root})
		}
		return e.fail("catalog list", ExitTransport, "transport", err.Error(), "", nil)
	}
	apis := make([]string, 0)
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		apiPath, err := catalogAPI(cat, ent.Name())
		if err != nil {
			continue
		}
		if _, err := os.Stat(apiPath.APIYAML); err == nil {
			apis = append(apis, ent.Name())
		}
	}
	return e.ok("catalog list", catalogListData{APIs: apis, Root: cat.Root})
}

func (e *Engine) catalogShow(_ context.Context, name, endpoint string) Result {
	example := "probe catalog show canvas --json"
	name = strings.TrimSpace(name)
	if name == "" {
		return e.usageError("catalog show", "missing api name", example)
	}
	cat, res, ok := e.requireCatalogRoot("catalog show")
	if !ok {
		return res
	}
	paths, err := catalogAPI(cat, name)
	if err != nil {
		return e.usageError("catalog show", "invalid api name", example)
	}
	api, err := e.loadCatalogAPI(paths)
	if err != nil {
		if os.IsNotExist(err) {
			return e.fail("catalog show", ExitUsage, "not_found", fmt.Sprintf("api %q not in catalog", name), example, []string{example})
		}
		return e.fail("catalog show", ExitTransport, "transport", err.Error(), "", nil)
	}
	if endpoint != "" {
		for _, ep := range api.Endpoints {
			if ep.ID == endpoint {
				return e.ok("catalog show", catalogShowEndpointData{API: api.Name, Endpoint: ep, Base: api.Base, RateLimit: api.RateLimit})
			}
		}
		return e.fail("catalog show", ExitUsage, "not_found", fmt.Sprintf("endpoint %q not found", endpoint), "probe catalog show "+name+" --json", nil)
	}
	return e.ok("catalog show", api)
}

func (e *Engine) catalogPath(_ context.Context, name string) Result {
	example := "probe catalog path canvas --json"
	name = strings.TrimSpace(name)
	if name == "" {
		return e.usageError("catalog path", "missing api name", example)
	}
	cat, res, ok := e.requireCatalogRoot("catalog path")
	if !ok {
		return res
	}
	paths, err := catalogAPI(cat, name)
	if err != nil {
		return e.usageError("catalog path", "invalid api name", example)
	}
	return e.ok("catalog path", catalogPathData{API: name, Path: paths.Root})
}

func (e *Engine) promote(_ context.Context, in promoteInput) Result {
	example := "probe promote canvas --endpoint get-courses --json"
	apiName := strings.TrimSpace(in.API)
	if apiName == "" {
		return e.usageError("promote", "missing api name", example)
	}
	if !apiNameRe.MatchString(apiName) {
		return e.usageError("promote", "api name must be a slug like canvas or stripe", example)
	}

	sp, res, ok := e.requireSpike("promote")
	if !ok {
		return res
	}
	cat, res, ok := e.requireCatalogRoot("promote")
	if !ok {
		return res
	}

	store := e.spikeStore(sp)
	policy := NewRedactionPolicy()
	var req savedRequestFile
	var resp savedResponseFile
	var err error
	reqID := strings.TrimSpace(in.Request)
	if reqID == "" {
		req, resp, err = store.Load(LastQuery(), policy)
		if err != nil {
			if os.IsNotExist(err) {
				return e.usageError("promote", "no saved request to promote", "probe hit GET https://example.com --save demo --json")
			}
			return e.fail("promote", ExitTransport, "transport", err.Error(), "", nil)
		}
		reqID = req.ID
	} else {
		req, resp, err = store.Load(ByID(reqID), policy)
		if err != nil {
			if os.IsNotExist(err) {
				return e.fail("promote", ExitUsage, "not_found", fmt.Sprintf("request %q not found", reqID), "probe last --json", nil)
			}
			return e.fail("promote", ExitUsage, "not_found", fmt.Sprintf("request %q not found", reqID), "probe last --json", nil)
		}
		reqID = req.ID
	}
	if req.Auth != "" {
		if profiles, perr := e.loadAuthProfiles(sp); perr == nil {
			if p, ok := profiles[req.Auth]; ok {
				req.Headers, req.URL = policyForAuth(p).redactForPersist(req.Headers, req.URL)
			}
		}
	}

	u, err := url.Parse(req.URL)
	if err != nil {
		return e.fail("promote", ExitTransport, "transport", "invalid saved URL: "+err.Error(), "", nil)
	}
	origin := u.Scheme + "://" + u.Host
	pathOnly := u.EscapedPath()
	if pathOnly == "" {
		pathOnly = "/"
	}

	epID := strings.TrimSpace(in.Endpoint)
	if epID == "" {
		epID = endpointSlug(req.Method, pathOnly)
	}

	paths, err := catalogAPI(cat, apiName)
	if err != nil {
		return e.usageError("promote", "api name must be a slug like canvas or stripe", example)
	}
	var api CatalogAPI
	if _, err := os.Stat(paths.APIYAML); err == nil {
		api, err = e.loadCatalogAPI(paths)
		if err != nil {
			return e.fail("promote", ExitTransport, "transport", err.Error(), "", nil)
		}
		if api.Base != "" && origin != "" && !strings.EqualFold(strings.TrimRight(api.Base, "/"), strings.TrimRight(origin, "/")) {
			return e.fail("promote", ExitUsage, "base_conflict",
				fmt.Sprintf("catalog base %q differs from request origin %q", api.Base, origin),
				"use a different api name or update the catalog manually",
				[]string{"probe catalog show " + apiName + " --json"})
		}
		for _, ep := range api.Endpoints {
			if ep.ID == epID {
				return e.fail("promote", ExitUsage, "fixture_exists",
					fmt.Sprintf("endpoint %q already exists (no overwrite in v1)", epID),
					"choose --endpoint <new-id> or a new api name",
					[]string{"probe promote " + apiName + " --endpoint " + epID + "-2 --json"})
			}
		}
	} else if !os.IsNotExist(err) {
		return e.fail("promote", ExitTransport, "transport", err.Error(), "", nil)
	} else {
		api = CatalogAPI{Name: apiName, Base: origin}
	}
	if api.Base == "" {
		api.Base = origin
	}
	if api.Name == "" {
		api.Name = apiName
	}

	fixtureName := epID + ".json"
	fixtureRel := filepath.Join("fixtures", fixtureName)
	ep := CatalogEndpoint{
		ID:         epID,
		Method:     strings.ToUpper(req.Method),
		Path:       pathOnly,
		Auth:       req.Auth,
		Fixture:    fixtureRel,
		LastStatus: resp.Status,
		PromotedAt: e.now().UTC().Format(time.RFC3339),
	}

	data := promoteData{
		API:      apiName,
		Endpoint: ep,
		Request:  reqID,
		DryRun:   in.DryRun,
		Path:     paths.Root,
	}
	if in.DryRun {
		return e.ok("promote", data)
	}

	if err := os.MkdirAll(paths.Fixtures, 0o755); err != nil {
		return e.fail("promote", ExitTransport, "transport", err.Error(), "", nil)
	}
	fixture := catalogFixture{Request: req, Response: resp}
	fb, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		return e.fail("promote", ExitTransport, "transport", err.Error(), "", nil)
	}
	fb = append(fb, '\n')
	fixturePath := filepath.Join(paths.Fixtures, fixtureName)
	if err := writeFileAtomic(fixturePath, fb, 0o644); err != nil {
		return e.fail("promote", ExitTransport, "transport", err.Error(), "", nil)
	}

	api.Endpoints = append(api.Endpoints, ep)
	if err := e.saveCatalogAPI(paths, api); err != nil {
		_ = os.Remove(fixturePath)
		return e.fail("promote", ExitTransport, "transport", err.Error(), "", nil)
	}

	if notes, err := os.ReadFile(sp.Notes); err == nil && len(bytesTrimSpace(notes)) > 0 {
		_ = appendCatalogNotes(paths.Notes, apiName, reqID, notes)
	}

	return e.ok("promote", data)
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func appendCatalogNotes(path, api, reqID string, spikeNotes []byte) error {
	header := fmt.Sprintf("\n## from spike (%s / %s)\n", api, reqID)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(header); err != nil {
		return err
	}
	_, err = f.Write(spikeNotes)
	return err
}

func endpointSlug(method, path string) string {
	method = strings.ToLower(strings.TrimSpace(method))
	path = strings.Trim(path, "/")
	if path == "" {
		path = "root"
	}
	path = strings.ReplaceAll(path, "/", "-")
	var b strings.Builder
	for _, r := range path {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "endpoint"
	}
	return method + "-" + strings.ToLower(slug)
}

func (e *Engine) loadCatalogDefaults(apiName string) (base string, rps float64) {
	apiName = strings.TrimSpace(apiName)
	if apiName == "" {
		return "", 0
	}
	cat, err := e.resolveCatalog()
	if err != nil || cat.Root == "" {
		return "", 0
	}
	paths, err := catalogAPI(cat, apiName)
	if err != nil {
		return "", 0
	}
	api, err := e.loadCatalogAPI(paths)
	if err != nil {
		return "", 0
	}
	if api.RateLimit != nil {
		rps = api.RateLimit.RPS
	}
	return api.Base, rps
}
