package catalog

import (
	"fmt"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// Lock is the parsed catalog/versions.toml single lock manifest (Section 33.4).
// Values are immutable after ParseLock returns.
type Lock struct {
	Schema    int
	LockedAt  string
	Evidence  string
	Toolchain LockToolchain
	Modules   []LockModule
	Tools     []LockTool
	Actions   []LockAction
	// entries is the sorted list of consumption IDs (stable).
	entries []string
}

// LockToolchain pins the Go language/toolchain (Section 12 / 33.4).
type LockToolchain struct {
	Go              string
	GoTag           string
	GoCommit        string
	GoPrimarySource string
	GoAnnounce      string
	GoCVE           string
}

// LockModule is one [[modules]] pin.
type LockModule struct {
	ID            string
	Path          string
	Version       string
	Scope         []string
	Section       string
	PrimarySource string
	OriginHash    string
	Package       string
	Note          string
}

// LockTool is one [[tools]] pin.
type LockTool struct {
	ID            string
	Name          string
	Module        string
	Version       string
	Release       string
	Scope         []string
	Section       string
	PrimarySource string
	OriginHash    string
	OriginRef     string
	Note          string
}

// LockAction is one [[actions]] pin (full commit SHA + tag).
type LockAction struct {
	ID            string
	Uses          string
	Tag           string
	SHA           string
	Scope         []string
	PrimarySource string
	Note          string
}

// wireLock is the TOML decode target (strict unknown fields rejected).
type wireLock struct {
	Schema    int               `toml:"schema"`
	LockedAt  string            `toml:"locked_at"`
	Evidence  string            `toml:"evidence"`
	Toolchain wireLockToolchain `toml:"toolchain"`
	Modules   []wireLockModule  `toml:"modules"`
	Tools     []wireLockTool    `toml:"tools"`
	Actions   []wireLockAction  `toml:"actions"`
}

type wireLockToolchain struct {
	Go              string `toml:"go"`
	GoTag           string `toml:"go_tag"`
	GoCommit        string `toml:"go_commit"`
	GoPrimarySource string `toml:"go_primary_source"`
	GoAnnounce      string `toml:"go_announce"`
	GoCVE           string `toml:"go_cve"`
}

type wireLockModule struct {
	ID            string   `toml:"id"`
	Path          string   `toml:"path"`
	Version       string   `toml:"version"`
	Scope         []string `toml:"scope"`
	Section       string   `toml:"section"`
	PrimarySource string   `toml:"primary_source"`
	OriginHash    string   `toml:"origin_hash"`
	Package       string   `toml:"package"`
	Note          string   `toml:"note"`
}

type wireLockTool struct {
	ID            string   `toml:"id"`
	Name          string   `toml:"name"`
	Module        string   `toml:"module"`
	Version       string   `toml:"version"`
	Release       string   `toml:"release"`
	Scope         []string `toml:"scope"`
	Section       string   `toml:"section"`
	PrimarySource string   `toml:"primary_source"`
	OriginHash    string   `toml:"origin_hash"`
	OriginRef     string   `toml:"origin_ref"`
	Note          string   `toml:"note"`
}

type wireLockAction struct {
	ID            string   `toml:"id"`
	Uses          string   `toml:"uses"`
	Tag           string   `toml:"tag"`
	SHA           string   `toml:"sha"`
	Scope         []string `toml:"scope"`
	PrimarySource string   `toml:"primary_source"`
	Note          string   `toml:"note"`
}

// ParseLock decodes versions.toml bytes. Unknown fields are rejected.
// Schema must be 1. Every module/tool/action must have a non-empty id.
func ParseLock(data []byte) (*Lock, error) {
	var wire wireLock
	md, err := toml.Decode(string(data), &wire)
	if err != nil {
		return nil, diagnostic.Wrap(
			diagnostic.IDCatalogInvalid,
			"versions.toml parse error",
			diagnostic.PathLocation("versions.toml"),
			err,
		)
	}
	if undec := md.Undecoded(); len(undec) > 0 {
		keys := make([]string, len(undec))
		for i, k := range undec {
			keys[i] = k.String()
		}
		sort.Strings(keys)
		return nil, diagnostic.Newf(
			diagnostic.IDCatalogInvalid,
			diagnostic.PathLocation("versions.toml"),
			"versions.toml unknown field(s): %s",
			strings.Join(keys, ", "),
		)
	}
	if wire.Schema != 1 {
		return nil, diagnostic.Newf(
			diagnostic.IDCatalogInvalid,
			diagnostic.PathLocation("versions.toml"),
			"versions.toml schema must be 1, got %d",
			wire.Schema,
		)
	}
	if wire.Toolchain.Go == "" {
		return nil, diagnostic.New(
			diagnostic.IDCatalogInvalid,
			"versions.toml toolchain.go pin is required",
			diagnostic.PathLocation("versions.toml"),
		)
	}

	lock := &Lock{
		Schema:   wire.Schema,
		LockedAt: wire.LockedAt,
		Evidence: wire.Evidence,
		Toolchain: LockToolchain{
			Go:              wire.Toolchain.Go,
			GoTag:           wire.Toolchain.GoTag,
			GoCommit:        wire.Toolchain.GoCommit,
			GoPrimarySource: wire.Toolchain.GoPrimarySource,
			GoAnnounce:      wire.Toolchain.GoAnnounce,
			GoCVE:           wire.Toolchain.GoCVE,
		},
	}

	seen := map[string]struct{}{}
	add := func(id string) error {
		if id == "" {
			return diagnostic.New(
				diagnostic.IDCatalogInvalid,
				"versions.toml entry missing id",
				diagnostic.PathLocation("versions.toml"),
			)
		}
		key := id
		if _, dup := seen[key]; dup {
			return diagnostic.Newf(
				diagnostic.IDCatalogInvalid,
				diagnostic.PathLocation("versions.toml"),
				"versions.toml duplicate lock entry id %q",
				id,
			)
		}
		seen[key] = struct{}{}
		lock.entries = append(lock.entries, key)
		return nil
	}

	// Toolchain is a lock entry consumed by generated go.mod / Foundry go.mod.
	if err := add("toolchain.go"); err != nil {
		return nil, err
	}

	for _, m := range wire.Modules {
		if err := add("module:" + m.ID); err != nil {
			return nil, err
		}
		if m.Path == "" || m.Version == "" {
			return nil, diagnostic.Newf(
				diagnostic.IDCatalogInvalid,
				diagnostic.PathLocation("versions.toml"),
				"module %q requires path and version",
				m.ID,
			)
		}
		lock.Modules = append(lock.Modules, LockModule{
			ID:            m.ID,
			Path:          m.Path,
			Version:       m.Version,
			Scope:         append([]string(nil), m.Scope...),
			Section:       m.Section,
			PrimarySource: m.PrimarySource,
			OriginHash:    m.OriginHash,
			Package:       m.Package,
			Note:          m.Note,
		})
	}
	for _, t := range wire.Tools {
		if err := add("tool:" + t.ID); err != nil {
			return nil, err
		}
		if t.Version == "" {
			return nil, diagnostic.Newf(
				diagnostic.IDCatalogInvalid,
				diagnostic.PathLocation("versions.toml"),
				"tool %q requires version",
				t.ID,
			)
		}
		lock.Tools = append(lock.Tools, LockTool{
			ID:            t.ID,
			Name:          t.Name,
			Module:        t.Module,
			Version:       t.Version,
			Release:       t.Release,
			Scope:         append([]string(nil), t.Scope...),
			Section:       t.Section,
			PrimarySource: t.PrimarySource,
			OriginHash:    t.OriginHash,
			OriginRef:     t.OriginRef,
			Note:          t.Note,
		})
	}
	for _, a := range wire.Actions {
		if err := add("action:" + a.ID); err != nil {
			return nil, err
		}
		if a.SHA == "" || a.Tag == "" || a.Uses == "" {
			return nil, diagnostic.Newf(
				diagnostic.IDCatalogInvalid,
				diagnostic.PathLocation("versions.toml"),
				"action %q requires uses, tag, and sha",
				a.ID,
			)
		}
		if len(a.SHA) != 40 {
			return nil, diagnostic.Newf(
				diagnostic.IDCatalogInvalid,
				diagnostic.PathLocation("versions.toml"),
				"action %q sha must be full 40-char commit SHA, got len=%d",
				a.ID, len(a.SHA),
			)
		}
		lock.Actions = append(lock.Actions, LockAction{
			ID:            a.ID,
			Uses:          a.Uses,
			Tag:           a.Tag,
			SHA:           a.SHA,
			Scope:         append([]string(nil), a.Scope...),
			PrimarySource: a.PrimarySource,
			Note:          a.Note,
		})
	}

	sort.Strings(lock.entries)
	return lock, nil
}

// EntryIDs returns a copy of the sorted lock entry consumption IDs.
// Format: "toolchain.go", "module:<id>", "tool:<id>", "action:<id>".
func (l *Lock) EntryIDs() []string {
	if l == nil {
		return nil
	}
	out := make([]string, len(l.entries))
	copy(out, l.entries)
	return out
}

// ModuleByID returns the module pin or false.
func (l *Lock) ModuleByID(id string) (LockModule, bool) {
	if l == nil {
		return LockModule{}, false
	}
	for _, m := range l.Modules {
		if m.ID == id {
			return m, true
		}
	}
	return LockModule{}, false
}

// ValidateConsumption enforces Section 33.4 / REQ-090: every lock entry is
// consumed exactly once, and every consumed ID exists in the lock.
//
// consumed maps entry ID → count. Unit manifests contribute module pins via
// LockConsumptionFromManifests; callers merge foundry/toolchain/tool/action
// consumers for full exactly-once checks. Load matches dependency path+version
// to lock pins without requiring complete consumption.
func ValidateConsumption(lock *Lock, consumed map[string]int) error {
	if lock == nil {
		return diagnostic.New(
			diagnostic.IDCatalogInvalid,
			"lock is nil",
			diagnostic.PathLocation("versions.toml"),
		)
	}
	known := map[string]struct{}{}
	for _, id := range lock.entries {
		known[id] = struct{}{}
	}

	var unused []string
	var notOnce []string
	for _, id := range lock.entries {
		n := consumed[id]
		if n == 0 {
			unused = append(unused, id)
		} else if n != 1 {
			notOnce = append(notOnce, fmt.Sprintf("%s(count=%d)", id, n))
		}
	}
	var unknown []string
	for id := range consumed {
		if _, ok := known[id]; !ok {
			unknown = append(unknown, id)
		}
	}
	sort.Strings(unknown)

	if len(unused) == 0 && len(notOnce) == 0 && len(unknown) == 0 {
		return nil
	}

	var parts []string
	if len(unused) > 0 {
		parts = append(parts, "unused lock entr"+pluralEntries(len(unused))+": "+strings.Join(unused, ", "))
	}
	if len(notOnce) > 0 {
		parts = append(parts, "lock entr"+pluralEntries(len(notOnce))+" not consumed exactly once: "+strings.Join(notOnce, ", "))
	}
	if len(unknown) > 0 {
		parts = append(parts, "consumed id(s) missing from lock: "+strings.Join(unknown, ", "))
	}
	return diagnostic.New(
		diagnostic.IDCatalogInvalid,
		strings.Join(parts, "; "),
		diagnostic.PathLocation("versions.toml"),
	)
}

func pluralEntries(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
