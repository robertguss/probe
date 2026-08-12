package render

import (
	"bytes"
	"fmt"
	"go/format"
	"path"
	"strconv"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// Approved template delimiters (Section 26.2 / REQ-097).
const (
	TemplateLeftDelim  = "[["
	TemplateRightDelim = "]]"
)

// TemplateData is the frozen typed data bag for restricted complete-file
// templates (Section 26.2 / REQ-097).
//
// Fields come from the validated specification, resolution facts
// (e.g. DistributionEnabled), and generation metadata (FoundryVersion for the
// single AGENTS.md provenance comment — REQ-161). No maps: every field is a
// scalar so iteration order cannot leak nondeterminism. Values are treated as
// immutable inputs.
//
// Catalog templates reference Name, Binary, Module, Description, Archetype,
// Visibility, FoundryVersion, and (optionally) DistributionEnabled.
type TemplateData struct {
	// Name is the project name (spec.name).
	Name string
	// Binary is the binary name (defaults to name when omitted in the spec).
	Binary string
	// Module is the module path (spec.module).
	Module string
	// Description is the single-line project description.
	Description string
	// Archetype is "cli" or "tui".
	Archetype string
	// Visibility is "private" or "public".
	Visibility string
	// FoundryVersion is the Foundry release that produced the tree (REQ-161).
	// Used only in the single AGENTS.md comment line; never parsed later.
	FoundryVersion string
	// DistributionEnabled is true when the distribution profile is selected
	// (Section 20 / Core template conditional; frozen boolean fact).
	DistributionEnabled bool
}

// TemplateJob is one restricted complete-file template render request.
type TemplateJob struct {
	// Path is the destination-relative output path.
	Path string
	// Mode is the planned file mode ("0644", "0755", …).
	Mode string
	// Source is the catalog-relative source path (source id / template id).
	Source string
	// Owner is an optional stable owner label (core, archetype:<id>, …).
	Owner string
	// Data is the frozen typed template data for this render.
	Data TemplateData
}

// restrictedFuncMap is the sole FuncMap for product templates (REQ-097).
// Built-in text/template operators (eq, and, len, …) remain available as
// language features; only join and quote are admitted as custom functions.
func restrictedFuncMap() template.FuncMap {
	return template.FuncMap{
		"join":  templateJoin,
		"quote": templateQuote,
	}
}

// templateJoin joins string parts with sep.
//
// Usage:
//
//	[[join ", " .Name .Binary]]          → "minimal-cli, minimal-cli"
//	[[join "/" "cmd" .Binary "main.go"]] → "cmd/minimal-cli/main.go"
//
// When the first argument after sep is a []string (and no further args), the
// slice is joined with sep (strings.Join semantics).
func templateJoin(sep string, parts ...any) (string, error) {
	if len(parts) == 1 {
		if elems, ok := parts[0].([]string); ok {
			return strings.Join(elems, sep), nil
		}
	}
	out := make([]string, 0, len(parts))
	for i, p := range parts {
		switch v := p.(type) {
		case string:
			out = append(out, v)
		case []string:
			// The sole-[]string case is handled above (len(parts)==1 early return).
			// Reaching here means []string was mixed with other args — forbidden.
			return "", fmt.Errorf("join: []string permitted only as the sole part after sep (arg %d)", i)
		default:
			return "", fmt.Errorf("join: arg %d: want string or []string, got %T", i, p)
		}
	}
	return strings.Join(out, sep), nil
}

// templateQuote returns a double-quoted Go string literal for s.
// Usage: [[quote .Name]]
func templateQuote(s string) string {
	return strconv.Quote(s)
}

// RenderTemplate renders one complete catalog template into a pure buffer and
// returns a single-entry Inventory (mechanism=template).
//
// Pipeline (P1.5.b / REQ-097):
//  1. Refuse unsafe output / source paths
//  2. Validate mode
//  3. Read source bytes from the catalog (missing → catalog.invalid)
//  4. Parse with [[ / ]], missingkey=error, FuncMap={join,quote}
//  5. Reject nested defines, partials, and template includes (fail closed)
//  6. Execute; unknown identifiers / disallowed actions → render.failed
//  7. Normalize LF; format .go sources with go/format
//  8. Digest source + content; optionally write through w
//
// When w is nil, content is retained only in the returned Inventory.
func RenderTemplate(cat CatalogReader, job TemplateJob, w Writer) (*Inventory, error) {
	entry, content, err := renderTemplateOne(cat, job)
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

// RenderTemplateAll renders multiple template jobs into one sorted Inventory.
// Fail-closed: the first error aborts with no partial inventory returned.
// When w is non-nil, each successful file is written before the next job.
func RenderTemplateAll(cat CatalogReader, jobs []TemplateJob, w Writer) (*Inventory, error) {
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
		entry, data, err := renderTemplateOne(cat, job)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[entry.Path]; dup {
			return nil, diagnostic.Newf(
				diagnostic.IDRenderFailed,
				diagnostic.PathLocation(entry.Path),
				"template render job %d duplicates output path %q",
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

// ValidateTemplateSource checks that source is a valid restricted complete-file
// template under the product contract (Section 26.2 / REQ-097): delimiters
// [[ / ]], missingkey=error, FuncMap join+quote only, no nested defines,
// partials, or forbidden functions. Does not execute the template.
//
// Used for hostile-template catalog validation and parse-only gates.
// Fail-closed: returns render.failed; never panics.
func ValidateTemplateSource(name string, source []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = templateError(name, fmt.Sprintf("template panic recovered: %v", r))
		}
	}()
	_, err = parseRestrictedTemplate(name, source)
	return err
}

// ExecuteTemplate renders source template bytes with data under the restricted
// contract, without catalog IO or inventory. name is used only as the template
// name in diagnostics (typically the catalog source path). outPath, when
// non-empty and ending in ".go", triggers go/format.
//
// Fail-closed: parse/execute/restriction failures return render.failed.
// Never panics on invalid input (recovered as render.failed).
func ExecuteTemplate(name string, source []byte, data TemplateData, outPath string) (content []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = templateError(name, fmt.Sprintf("template panic recovered: %v", r))
			content = nil
		}
	}()

	tpl, perr := parseRestrictedTemplate(name, source)
	if perr != nil {
		return nil, perr
	}

	var buf bytes.Buffer
	if eerr := tpl.Execute(&buf, data); eerr != nil {
		if name == "" {
			name = "<template>"
		}
		return nil, templateError(name, fmt.Sprintf("template execute failed: %v", eerr))
	}

	out := normalizeLF(buf.Bytes())
	// Always return a non-nil slice on success (empty template → empty file).
	if out == nil {
		out = []byte{}
	}

	// Go sources must parse and format (Section 26.3).
	if isGoSourcePath(outPath) {
		formatted, ferr := format.Source(out)
		if ferr != nil {
			if name == "" {
				name = "<template>"
			}
			return nil, templateError(name, fmt.Sprintf(
				"rendered Go source is not go/format-clean for %q: %v", outPath, ferr,
			))
		}
		out = normalizeLF(formatted)
		if out == nil {
			out = []byte{}
		}
	}

	return out, nil
}

// parseRestrictedTemplate parses source under the product template contract
// and rejects nested defines / partials / forbidden functions.
func parseRestrictedTemplate(name string, source []byte) (*template.Template, error) {
	if name == "" {
		name = "<template>"
	}
	// Parse under restricted delimiters, missingkey=error, join/quote only.
	tpl, perr := template.New(name).
		Delims(TemplateLeftDelim, TemplateRightDelim).
		Option("missingkey=error").
		Funcs(restrictedFuncMap()).
		Parse(string(source))
	if perr != nil {
		return nil, templateError(name, fmt.Sprintf("template parse failed: %v", perr))
	}

	// Nested {{define}} / {{block}} create associated templates beyond the root.
	if associated := tpl.Templates(); len(associated) > 1 {
		names := make([]string, 0, len(associated))
		for _, t := range associated {
			if t.Name() != name {
				names = append(names, t.Name())
			}
		}
		return nil, templateError(name, fmt.Sprintf(
			"nested template definitions are not allowed (found: %s); complete-file templates only (REQ-097)",
			strings.Join(names, ", "),
		))
	}

	// Reject {{template}} includes / partials by walking the parse tree.
	if tpl.Tree != nil && tpl.Tree.Root != nil {
		if ferr := rejectForbiddenNodes(name, tpl.Tree.Root); ferr != nil {
			return nil, ferr
		}
	}
	return tpl, nil
}

func renderTemplateOne(cat CatalogReader, job TemplateJob) (Entry, []byte, error) {
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
			"template render mode is required for path %q",
			outPath,
		)
	}
	if _, ok := knownModes[mode]; !ok {
		return Entry{}, nil, diagnostic.Newf(
			diagnostic.IDRenderFailed,
			diagnostic.PathLocation(outPath),
			"template render mode %q is not admitted (allowed: 0644, 0755) for path %q",
			mode, outPath,
		)
	}

	src := strings.TrimSpace(job.Source)
	if src == "" {
		return Entry{}, nil, diagnostic.Newf(
			diagnostic.IDRenderFailed,
			diagnostic.PathLocation(outPath),
			"template render source is required for path %q",
			outPath,
		)
	}
	if errMsg := sourcePathUnsafe(src); errMsg != "" {
		return Entry{}, nil, unsafePathError(src, "catalog source "+errMsg)
	}

	raw, err := cat.Read(src)
	if err != nil {
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

	// Defensive copy of source bytes for digests and parse independence.
	srcBytes := make([]byte, len(raw))
	copy(srcBytes, raw)
	srcDigest := ContentDigest(srcBytes)

	content, err := ExecuteTemplate(src, srcBytes, job.Data, outPath)
	if err != nil {
		return Entry{}, nil, err
	}

	outDigest := ContentDigest(content)
	entry := Entry{
		Path:          outPath,
		Mode:          mode,
		Mechanism:     MechanismTemplate,
		Source:        src,
		Owner:         strings.TrimSpace(job.Owner),
		SourceDigest:  srcDigest,
		ContentDigest: outDigest,
	}
	return entry, content, nil
}

// rejectForbiddenNodes walks the parse tree and fails closed on partials,
// includes, and nested template invocations (REQ-097).
//
// Typed-nil parse nodes (for example a nil *parse.ListNode stored in an
// interface when an if has no else branch) are treated as absent: a plain
// `n == nil` check is not enough for those values.
func rejectForbiddenNodes(tmplName string, n parse.Node) error {
	if n == nil || parseNodeIsNil(n) {
		return nil
	}
	switch n := n.(type) {
	case *parse.ListNode:
		for _, c := range n.Nodes {
			if err := rejectForbiddenNodes(tmplName, c); err != nil {
				return err
			}
		}
	case *parse.ActionNode:
		return rejectForbiddenNodes(tmplName, n.Pipe)
	case *parse.IfNode:
		if err := rejectForbiddenNodes(tmplName, n.Pipe); err != nil {
			return err
		}
		if err := rejectForbiddenNodes(tmplName, n.List); err != nil {
			return err
		}
		return rejectForbiddenNodes(tmplName, n.ElseList)
	case *parse.RangeNode:
		if err := rejectForbiddenNodes(tmplName, n.Pipe); err != nil {
			return err
		}
		if err := rejectForbiddenNodes(tmplName, n.List); err != nil {
			return err
		}
		return rejectForbiddenNodes(tmplName, n.ElseList)
	case *parse.WithNode:
		if err := rejectForbiddenNodes(tmplName, n.Pipe); err != nil {
			return err
		}
		if err := rejectForbiddenNodes(tmplName, n.List); err != nil {
			return err
		}
		return rejectForbiddenNodes(tmplName, n.ElseList)
	case *parse.TemplateNode:
		// {{template "name" pipeline}} — partials / includes (forbidden).
		return templateError(tmplName, fmt.Sprintf(
			"template include/partial %q is not allowed; complete-file templates only (REQ-097)",
			n.Name,
		))
	case *parse.PipeNode:
		for _, c := range n.Cmds {
			if err := rejectForbiddenNodes(tmplName, c); err != nil {
				return err
			}
		}
	case *parse.CommandNode:
		// Reject identifiers outside the admitted custom set that look like
		// exec/reflection escapes. Built-ins (eq, and, len, …) are language
		// features and remain; undefined names fail at execute. We hard-reject
		// call (reflection-ish) and any explicit env/fs/time/rand/exec names.
		if len(n.Args) > 0 {
			if id, ok := n.Args[0].(*parse.IdentifierNode); ok {
				if reason := forbiddenFuncReason(id.Ident); reason != "" {
					return templateError(tmplName, reason)
				}
			}
		}
		for _, arg := range n.Args {
			if err := rejectForbiddenNodes(tmplName, arg); err != nil {
				return err
			}
		}
	case *parse.ChainNode:
		return rejectForbiddenNodes(tmplName, n.Node)
	case *parse.BranchNode:
		// Embedded by If/Range/With; also reachable if type-asserted broadly.
		if err := rejectForbiddenNodes(tmplName, n.Pipe); err != nil {
			return err
		}
		if err := rejectForbiddenNodes(tmplName, n.List); err != nil {
			return err
		}
		return rejectForbiddenNodes(tmplName, n.ElseList)
	}
	return nil
}

// parseNodeIsNil reports whether n is a typed-nil parse node pointer. A nil
// *parse.ListNode (common for IfNode.ElseList when there is no else) is a
// non-nil interface value and must not be range'd or field-dereferenced.
func parseNodeIsNil(n parse.Node) bool {
	if n == nil {
		return true
	}
	switch v := n.(type) {
	case *parse.ListNode:
		return v == nil
	case *parse.ActionNode:
		return v == nil
	case *parse.IfNode:
		return v == nil
	case *parse.RangeNode:
		return v == nil
	case *parse.WithNode:
		return v == nil
	case *parse.TemplateNode:
		return v == nil
	case *parse.PipeNode:
		return v == nil
	case *parse.CommandNode:
		return v == nil
	case *parse.ChainNode:
		return v == nil
	case *parse.BranchNode:
		return v == nil
	case *parse.TextNode:
		return v == nil
	case *parse.StringNode:
		return v == nil
	case *parse.IdentifierNode:
		return v == nil
	case *parse.FieldNode:
		return v == nil
	case *parse.VariableNode:
		return v == nil
	case *parse.DotNode:
		return v == nil
	case *parse.NilNode:
		return v == nil
	case *parse.NumberNode:
		return v == nil
	case *parse.BoolNode:
		return v == nil
	default:
		return false
	}
}

// forbiddenFuncReason returns a non-empty message when ident is an explicitly
// banned function name (defense-in-depth beyond empty FuncMap).
func forbiddenFuncReason(ident string) string {
	switch ident {
	case "join", "quote":
		return "" // admitted custom functions
	case "call":
		return `function "call" is not allowed in restricted templates (REQ-097)`
	case "env", "environ", "expandenv":
		return fmt.Sprintf("function %q is not allowed (no environment access; REQ-097)", ident)
	case "readFile", "readDir", "file", "glob", "os":
		return fmt.Sprintf("function %q is not allowed (no filesystem access; REQ-097)", ident)
	case "now", "time", "date", "dateInZone":
		return fmt.Sprintf("function %q is not allowed (no time access; REQ-097)", ident)
	case "rand", "randInt", "uuid", "shuffle":
		return fmt.Sprintf("function %q is not allowed (no randomness; REQ-097)", ident)
	case "exec", "shell", "cmd", "system":
		return fmt.Sprintf("function %q is not allowed (no command execution; REQ-097)", ident)
	case "httpGet", "getHostByName", "network":
		return fmt.Sprintf("function %q is not allowed (no network access; REQ-097)", ident)
	default:
		// Unknown identifiers fail at execute if not built-in; no ban here.
		return ""
	}
}

// normalizeLF converts CRLF/CR to LF (REQ-126 complete files with LF endings).
func normalizeLF(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	// Fast path: no CR present.
	if !bytes.Contains(b, []byte{'\r'}) {
		return b
	}
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	b = bytes.ReplaceAll(b, []byte{'\r'}, []byte("\n"))
	return b
}

// isGoSourcePath reports whether p is a Go source file path (for go/format).
func isGoSourcePath(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}
	return strings.EqualFold(path.Ext(p), ".go")
}

func templateError(name, msg string) *diagnostic.FoundryError {
	loc := diagnostic.PathLocation(name)
	if name == "" {
		loc = diagnostic.PathLocation("<template>")
	}
	return diagnostic.New(diagnostic.IDRenderFailed, msg, loc).WithRemediation(
		"Fix the restricted complete-file template: use delimiters [[ and ]], " +
			"reference only frozen TemplateData fields, and use only the join/quote " +
			"functions. Partials, includes, nested defines, environment, filesystem, " +
			"time, randomness, network, and exec are prohibited (REQ-097 / Section 26.2).",
	)
}
