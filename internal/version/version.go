package version

import (
	"runtime"
	"runtime/debug"
)

// DefaultVersion is reported when build info has no stamped module version
// (local `go run` / unstamped builds). Matches plan.DefaultFoundryVersion.
const DefaultVersion = "0.1.0"

// Info is the stable version payload for `foundry version` (REQ-035/REQ-162).
// Field names are the JSON contract surface for --output json.
type Info struct {
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	Go            string `json:"go"`
	CatalogDigest string `json:"catalog_digest"`
}

// Read returns identity from runtime/debug.BuildInfo and runtime.Version.
// CatalogDigest is left empty; callers fill it from the loaded catalog.
func Read() Info {
	info := Info{
		Version: DefaultVersion,
		Go:      runtime.Version(),
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok || bi == nil {
		return info
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		info.Version = v
	}
	var revision, modified string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	if revision != "" {
		info.Commit = revision
		if modified == "true" {
			info.Commit += "-dirty"
		}
	}
	return info
}

// WithCatalog returns a copy of info with CatalogDigest set.
func (i Info) WithCatalog(digest string) Info {
	i.CatalogDigest = digest
	return i
}
