package spec

import (
	"fmt"
	"log/slog"
	"path"
	"sort"
	"strconv"

	"golang.org/x/mod/module"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// fieldDiag is one collected field diagnostic before source-order sort.
type fieldDiag struct {
	field string
	line  int
	col   int
	order int // contract order index for stable sort when position missing
	err   *diagnostic.FoundryError
}

// Validate applies Section 14.3–14.5 field and cross-field rules to raw and
// returns an immutable ValidatedSpecification with defaults applied.
//
// Defaults are applied only when structural validation fully succeeds
// (no field or cross-field errors). Schema physical position is irrelevant
// (FND-013): only the value is checked.
//
// Independent per-field errors are aggregated in source order (Section 14.5).
// Schema version failure (unsupported value) is reported alone for that stage
// before per-field collection.
//
// Profile ID existence is not checked here (resolve.unknown_profile in resolve).
// Duplicate profile IDs fail with spec.duplicate_profile.
func Validate(raw *RawSpecification) (*ValidatedSpecification, error) {
	if raw == nil {
		return nil, diagnostic.New(
			diagnostic.IDSpecInvalidField,
			"raw specification is nil",
			diagnostic.Location{},
		)
	}

	// Stage 1: schema version (Section 14.2 / 14.5) — reported alone.
	if err := validateSchema(raw); err != nil {
		return nil, err
	}

	// Stage 2: per-field rules (aggregate independent errors).
	fields := validateFields(raw)

	// Stage 3: cross-field constraints (appended to field diags).
	validateCrossField(raw, &fields)

	if len(fields.diags) > 0 {
		return nil, fields.emitError(raw)
	}

	// Stage 4: defaults only after full structural validation.
	return applyDefaults(raw, &fields), nil
}

// validateSchema checks the schema version is supported (Section 14.2).
// Returns an error if schema is present but unsupported; nil otherwise.
func validateSchema(raw *RawSpecification) error {
	if raw.Schema != nil && *raw.Schema != SupportedSchema {
		loc := raw.Position("schema")
		if loc.IsZero() {
			loc = diagnostic.SpecLocation(raw.File, 0, 0)
		}
		err := diagnostic.Newf(
			diagnostic.IDSpecUnsupportedSchema,
			loc,
			"schema = %d is not supported; supported set is {%d}",
			*raw.Schema, SupportedSchema,
		)
		logFieldReject("schema", "unsupported_schema", loc, err)
		return err
	}
	return nil
}

// fieldState holds per-field parsed values and validity flags accumulated
// during validateFields and consumed by validateCrossField and applyDefaults.
type fieldState struct {
	diags []fieldDiag
	order int

	name        string
	nameOK      bool
	modulePath  string
	moduleOK    bool
	description string
	archetype   string
	destination string
	destBase    string
	destOK      bool
	binary      string
	binarySet   bool
	visibility  string
	visSet      bool
	profiles    []string
	gitInit     bool
	gitInitSet  bool
	gitBranch   string
	gitBrSet    bool
}

func (f *fieldState) nextOrder() int {
	f.order++
	return f.order
}

// validateFields applies Section 14.3 per-field rules, collecting all
// independent diagnostics for source-order reporting.
func validateFields(raw *RawSpecification) fieldState {
	var f fieldState

	// schema required
	if raw.Schema == nil {
		f.diags = append(f.diags, missingField(raw, "schema", f.nextOrder()))
	}

	// name
	if raw.Name == nil {
		f.diags = append(f.diags, missingField(raw, "name", f.nextOrder()))
	} else {
		f.name = *raw.Name
		if !validName(f.name) {
			f.diags = append(f.diags, fieldReject(raw, "name", f.nextOrder(),
				diagnostic.IDSpecInvalidField, nameRuleMessage("name", f.name)))
		} else {
			f.nameOK = true
		}
	}

	// module (CheckPath + no /vN; final-segment cross-check later)
	if raw.Module == nil {
		f.diags = append(f.diags, missingField(raw, "module", f.nextOrder()))
	} else {
		f.modulePath = *raw.Module
		if err := module.CheckPath(f.modulePath); err != nil {
			f.diags = append(f.diags, fieldReject(raw, "module", f.nextOrder(),
				diagnostic.IDSpecInvalidField,
				fmt.Sprintf("module path %q is invalid: %v", f.modulePath, err)))
		} else if hasSemanticImportVersionSuffix(f.modulePath) {
			f.diags = append(f.diags, fieldReject(raw, "module", f.nextOrder(),
				diagnostic.IDSpecInvalidField,
				fmt.Sprintf("module path %q has a semantic import-version suffix (%q); applications must not use /vN",
					f.modulePath, path.Base(f.modulePath))))
		} else {
			f.moduleOK = true
		}
	}

	// description
	if raw.Description == nil {
		f.diags = append(f.diags, missingField(raw, "description", f.nextOrder()))
	} else {
		trimmed, msg := validateDescription(*raw.Description)
		if msg != "" {
			f.diags = append(f.diags, fieldReject(raw, "description", f.nextOrder(),
				diagnostic.IDSpecInvalidField, msg))
		} else {
			f.description = trimmed
		}
	}

	// archetype
	if raw.Archetype == nil {
		f.diags = append(f.diags, missingField(raw, "archetype", f.nextOrder()))
	} else {
		f.archetype = *raw.Archetype
		if !validArchetype(f.archetype) {
			f.diags = append(f.diags, fieldReject(raw, "archetype", f.nextOrder(),
				diagnostic.IDSpecInvalidField,
				fmt.Sprintf("archetype must be exactly \"cli\" or \"tui\" (got %q)", f.archetype)))
		}
	}

	// destination (lexical only; basename cross-check later)
	if raw.Destination == nil {
		f.diags = append(f.diags, missingField(raw, "destination", f.nextOrder()))
	} else {
		f.destination = *raw.Destination
		base, msg := destinationLexicalOK(f.destination)
		if msg != "" {
			f.diags = append(f.diags, fieldReject(raw, "destination", f.nextOrder(),
				diagnostic.IDSpecInvalidField, msg))
		} else {
			f.destBase = base
			f.destOK = true
		}
	}

	// binary (optional; same character rules as name)
	if raw.Binary != nil {
		f.binarySet = true
		f.binary = *raw.Binary
		if !validName(f.binary) {
			f.diags = append(f.diags, fieldReject(raw, "binary", f.nextOrder(),
				diagnostic.IDSpecInvalidField, nameRuleMessage("binary", f.binary)))
		}
	}

	// visibility (optional)
	if raw.Visibility != nil {
		f.visSet = true
		f.visibility = *raw.Visibility
		if !validVisibility(f.visibility) {
			f.diags = append(f.diags, fieldReject(raw, "visibility", f.nextOrder(),
				diagnostic.IDSpecInvalidField,
				fmt.Sprintf("visibility must be \"private\" or \"public\" (got %q)", f.visibility)))
		}
	}

	// profiles (optional; duplicates here; existence in resolve)
	if raw.ProfilesSet {
		f.profiles = append([]string(nil), raw.Profiles...)
		// Duplicate IDs → spec.duplicate_profile naming both indexes.
		seen := make(map[string]int, len(f.profiles))
		for i, id := range f.profiles {
			if prev, ok := seen[id]; ok {
				loc := raw.Position("profiles")
				if loc.IsZero() {
					loc = diagnostic.SpecLocation(raw.File, 0, 0)
				}
				msg := fmt.Sprintf("profiles lists duplicate ID %q at indexes %d and %d", id, prev, i)
				fe := diagnostic.New(diagnostic.IDSpecDuplicateProfile, msg, loc)
				f.diags = append(f.diags, fieldDiag{
					field: "profiles",
					line:  loc.Line,
					col:   loc.Column,
					order: f.nextOrder(),
					err:   fe,
				})
				// One duplicate is enough; still collect other field errors.
				break
			}
			seen[id] = i
		}
	}

	// git.init (optional bool — any bool is valid)
	if raw.GitInit != nil {
		f.gitInitSet = true
		f.gitInit = *raw.GitInit
	}

	// git.initial_branch (optional; same character rules as name)
	if raw.GitInitialBranch != nil {
		f.gitBrSet = true
		f.gitBranch = *raw.GitInitialBranch
		if !validName(f.gitBranch) {
			f.diags = append(f.diags, fieldReject(raw, "git.initial_branch", f.nextOrder(),
				diagnostic.IDSpecInvalidField, nameRuleMessage("git.initial_branch", f.gitBranch)))
		}
	}

	return f
}

// validateCrossField applies Section 14.4 cross-field constraints, appending
// any diagnostics to f.diags.
func validateCrossField(raw *RawSpecification, f *fieldState) {
	if f.nameOK && f.moduleOK {
		seg := moduleFinalSegment(f.modulePath)
		if seg != f.name {
			f.diags = append(f.diags, fieldReject(raw, "module", f.nextOrder(),
				diagnostic.IDSpecInvalidField,
				fmt.Sprintf("module final path segment %q must equal name %q", seg, f.name)))
		}
	}
	if f.nameOK && f.destOK {
		if f.destBase != f.name {
			f.diags = append(f.diags, fieldReject(raw, "destination", f.nextOrder(),
				diagnostic.IDSpecInvalidField,
				fmt.Sprintf("destination basename %q must equal name %q", f.destBase, f.name)))
		}
	}
}

// emitError sorts diagnostics by source order and returns the aggregated
// ValidationError.
func (f *fieldState) emitError(raw *RawSpecification) error {
	sortDiags(f.diags)
	errs := make([]*diagnostic.FoundryError, len(f.diags))
	for i, d := range f.diags {
		errs[i] = d.err
		logFieldReject(d.field, string(d.err.ID()), diagnostic.Location{
			File: raw.File, Line: d.line, Column: d.col,
		}, d.err)
	}
	return &ValidationError{Errs: errs}
}

// applyDefaults fills optional fields with their defaults and returns the
// immutable ValidatedSpecification. Called only after all validation passes.
func applyDefaults(raw *RawSpecification, f *fieldState) *ValidatedSpecification {
	binary := f.binary
	if !f.binarySet {
		binary = f.name
	}
	visibility := f.visibility
	if !f.visSet {
		visibility = DefaultVisibility
	}
	gitInit := f.gitInit
	if !f.gitInitSet {
		gitInit = DefaultGitInit
	}
	gitBranch := f.gitBranch
	if !f.gitBrSet {
		gitBranch = DefaultGitInitialBranch
	}
	// Always store a non-nil profiles slice (default []).
	// append(nil, empty...) yields nil — allocate explicitly.
	profCopy := make([]string, len(f.profiles))
	copy(profCopy, f.profiles)

	return &ValidatedSpecification{
		file:             raw.File,
		schema:           SupportedSchema,
		name:             f.name,
		module:           f.modulePath,
		description:      f.description,
		archetype:        f.archetype,
		destination:      f.destination,
		binary:           binary,
		visibility:       visibility,
		profiles:         profCopy,
		gitInit:          gitInit,
		gitInitialBranch: gitBranch,
	}
}

func missingField(raw *RawSpecification, field string, order int) fieldDiag {
	loc := diagnostic.SpecLocation(raw.File, 0, 0)
	fe := diagnostic.Newf(
		diagnostic.IDSpecInvalidField,
		loc,
		"required field %q is missing",
		field,
	)
	return fieldDiag{field: field, line: 0, col: 0, order: order, err: fe}
}

func fieldReject(raw *RawSpecification, field string, order int, id diagnostic.Identifier, msg string) fieldDiag {
	loc := raw.Position(field)
	if loc.IsZero() {
		loc = diagnostic.SpecLocation(raw.File, 0, 0)
	}
	fe := diagnostic.New(id, msg, loc)
	return fieldDiag{
		field: field,
		line:  loc.Line,
		col:   loc.Column,
		order: order,
		err:   fe,
	}
}

func sortDiags(diags []fieldDiag) {
	sort.SliceStable(diags, func(i, j int) bool {
		a, b := diags[i], diags[j]
		// Present source positions (line > 0) sort before missing (line 0),
		// then by line, column, then contract order for stability.
		aPos := a.line > 0
		bPos := b.line > 0
		if aPos != bPos {
			return aPos // positioned first
		}
		if a.line != b.line {
			return a.line < b.line
		}
		if a.col != b.col {
			return a.col < b.col
		}
		return a.order < b.order
	})
}

func logFieldReject(field, ruleID string, loc diagnostic.Location, err *diagnostic.FoundryError) {
	if err == nil {
		return
	}
	slog.Debug("spec.field_reject",
		"field", field,
		"rule_id", ruleID,
		"error_id", string(err.ID()),
		"line", loc.Line,
		"column", loc.Column,
		"message", err.Message(),
	)
}

// FormatValidationFailure returns a one-line summary for tests and step logs:
// error count + ids + first field position.
func FormatValidationFailure(err error) string {
	if err == nil {
		return "ok"
	}
	errs := CollectFoundryErrors(err)
	if len(errs) == 0 {
		return err.Error()
	}
	ids := make([]string, len(errs))
	for i, fe := range errs {
		ids[i] = string(fe.ID())
	}
	first := errs[0]
	loc := first.Location()
	return "count=" + strconv.Itoa(len(errs)) +
		" ids=" + joinComma(ids) +
		" first_line=" + strconv.Itoa(loc.Line) +
		" first_col=" + strconv.Itoa(loc.Column)
}

func joinComma(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for i := 1; i < len(ss); i++ {
		out += "," + ss[i]
	}
	return out
}
