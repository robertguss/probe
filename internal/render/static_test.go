package render_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/render"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestStaticMultiFileInventory covers several static files with byte-identical
// outputs, modes, digests, and step logs (P1.5.a acceptance).
func TestStaticMultiFileInventory(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	cat := fixtureCatalog()
	jobs := []render.StaticJob{
		{
			Path:   "LICENSE",
			Mode:   "0644",
			Source: "core/files/LICENSE",
			Owner:  "core",
		},
		{
			Path:   "AGENTS.md",
			Mode:   "0644",
			Source: "core/files/AGENTS.md",
			Owner:  "core",
		},
		{
			Path:   "docs/releasing.md",
			Mode:   "0644",
			Source: "profiles/distribution/files/releasing.md",
			Owner:  "profile:distribution",
		},
	}
	log.Fixture("jobs", fmt.Sprintf("n=%d", len(jobs)))
	log.Inputs(map[string]string{
		"mechanism": "static",
		"files":     fmt.Sprintf("%d", len(jobs)),
	})
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	buf := render.NewMemoryWriter()
	inv, err := render.RenderStaticAll(cat, jobs, buf)
	if err != nil {
		log.Fail("render_static_all", err.Error())
	}
	log.Step("inventory", testutil.OutcomeOK, fmt.Sprintf("n=%d paths=%v", inv.Len(), inv.Paths()))
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	log.Assert("entry_count", inv.Len() == 3, 3, inv.Len())
	log.Assert("writer_count", buf.Len() == 3, 3, buf.Len())

	// Sorted by path: AGENTS.md, LICENSE, docs/releasing.md
	entries := inv.Entries()
	wantOrder := []string{"AGENTS.md", "LICENSE", "docs/releasing.md"}
	for i, want := range wantOrder {
		e := entries[i]
		log.Assert("path_"+want, e.Path == want, want, e.Path)
		log.Assert("mech_"+want, e.Mechanism == render.MechanismStatic,
			render.MechanismStatic, e.Mechanism)
		log.Assert("mode_"+want, e.Mode == "0644", "0644", e.Mode)

		src, rerr := cat.Read(e.Source)
		if rerr != nil {
			log.Fail("read_source_"+want, rerr.Error())
		}
		out, ok := inv.Content(e.Path)
		log.Assert("content_present_"+want, ok, true, ok)
		log.Assert("byte_identical_"+want, bytes.Equal(src, out), true, bytes.Equal(src, out))

		srcDig := digestOf(src)
		log.Assert("source_digest_"+want, e.SourceDigest == srcDig, srcDig, e.SourceDigest)
		log.Assert("content_digest_"+want, e.ContentDigest == srcDig, srcDig, e.ContentDigest)
		log.Assert("digest_eq_"+want, e.SourceDigest == e.ContentDigest,
			e.SourceDigest, e.ContentDigest)

		wContent, wMode, wok := buf.Get(e.Path)
		log.Assert("writer_present_"+want, wok, true, wok)
		log.Assert("writer_mode_"+want, wMode == "0644", "0644", wMode)
		log.Assert("writer_bytes_"+want, bytes.Equal(src, wContent), true, bytes.Equal(src, wContent))

		log.Step("file", testutil.OutcomeOK, fmt.Sprintf(
			"path=%s source=%s mode=%s digest=%s outcome=ok",
			e.Path, e.Source, e.Mode, e.ContentDigest,
		))
		log.NotePath(e.Path)
		log.NoteID(string(e.ContentDigest))
	}
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestStaticModeMatrix applies 0644 and 0755 modes without rewriting content.
func TestStaticModeMatrix(t *testing.T) {
	log := testutil.New(t)
	log.Phase("mode_matrix")
	cat := fixtureCatalog()

	cases := []struct {
		path   string
		mode   string
		source string
	}{
		{path: "LICENSE", mode: "0644", source: "core/files/LICENSE"},
		{path: "scripts/run.sh", mode: "0755", source: "core/files/scripts/run.sh"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.mode+"_"+tc.path, func(t *testing.T) {
			t.Parallel()
			sub := testutil.New(t)
			sub.Phase("render")
			sub.Inputs(map[string]string{
				"path":   tc.path,
				"mode":   tc.mode,
				"source": tc.source,
			})
			inv, err := render.RenderStatic(cat, render.StaticJob{
				Path:   tc.path,
				Mode:   tc.mode,
				Source: tc.source,
			}, nil)
			if err != nil {
				sub.Fail("render", err.Error())
			}
			e, ok := inv.EntryByPath(tc.path)
			sub.Assert("found", ok, true, ok)
			sub.Assert("mode_applied", e.Mode == tc.mode, tc.mode, e.Mode)
			sub.Assert("mechanism", e.Mechanism == render.MechanismStatic,
				render.MechanismStatic, e.Mechanism)
			src, _ := cat.Read(tc.source)
			out, _ := inv.Content(tc.path)
			sub.Assert("byte_identical", bytes.Equal(src, out), true, bytes.Equal(src, out))
			sub.Assert("digest_eq", e.SourceDigest == e.ContentDigest,
				e.SourceDigest, e.ContentDigest)
			sub.Step("mode", testutil.OutcomeOK, fmt.Sprintf(
				"path=%s source=%s mode=%s digest=%s outcome=ok",
				e.Path, e.Source, e.Mode, e.ContentDigest,
			))
			sub.PhaseEnd("render", testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("mode_matrix", testutil.OutcomeOK)
}

// TestStaticRefusePathEscape refuses .. and absolute paths at the render
// planning layer with fs.unsafe_path.
func TestStaticRefusePathEscape(t *testing.T) {
	log := testutil.New(t)
	log.Phase("escape")
	cat := fixtureCatalog()

	cases := []struct {
		name string
		job  render.StaticJob
	}{
		{
			name: "dotdot_segment",
			job:  render.StaticJob{Path: "foo/../../etc/passwd", Mode: "0644", Source: "core/files/LICENSE"},
		},
		{
			name: "leading_dotdot",
			job:  render.StaticJob{Path: "../escape", Mode: "0644", Source: "core/files/LICENSE"},
		},
		{
			name: "absolute_unix",
			job:  render.StaticJob{Path: "/etc/passwd", Mode: "0644", Source: "core/files/LICENSE"},
		},
		{
			name: "absolute_drive",
			job:  render.StaticJob{Path: "C:/Windows/system32", Mode: "0644", Source: "core/files/LICENSE"},
		},
		{
			name: "source_dotdot",
			job:  render.StaticJob{Path: "LICENSE", Mode: "0644", Source: "../secret"},
		},
		{
			name: "source_absolute",
			job:  render.StaticJob{Path: "LICENSE", Mode: "0644", Source: "/etc/passwd"},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sub := testutil.New(t)
			sub.Phase("refuse")
			sub.Inputs(map[string]string{
				"path":   tc.job.Path,
				"source": tc.job.Source,
				"case":   tc.name,
			})
			inv, err := render.RenderStatic(cat, tc.job, nil)
			sub.Assert("nil_inventory", inv == nil, true, inv == nil)
			if err == nil {
				sub.Fail("expected_error", "want fs.unsafe_path, got nil")
			}
			fe := asFoundry(t, err)
			sub.Assert("id", fe.ID() == diagnostic.IDFSUnsafePath,
				diagnostic.IDFSUnsafePath, fe.ID())
			sub.Step("refuse", testutil.OutcomeOK, fmt.Sprintf(
				"path=%s source=%s id=%s outcome=refused",
				tc.job.Path, tc.job.Source, fe.ID(),
			))
			sub.NotePath(tc.job.Path)
			sub.PhaseEnd("refuse", testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("escape", testutil.OutcomeOK)
}

// TestStaticDigestEquality asserts source digest == output digest for static.
func TestStaticDigestEquality(t *testing.T) {
	log := testutil.New(t)
	log.Phase("digest_eq")
	cat := fixtureCatalog()
	srcPath := "core/files/AGENTS.md"
	src, err := cat.Read(srcPath)
	if err != nil {
		log.Fail("read", err.Error())
	}
	wantDig := digestOf(src)

	inv, err := render.RenderStatic(cat, render.StaticJob{
		Path:   "AGENTS.md",
		Mode:   "0644",
		Source: srcPath,
	}, nil)
	if err != nil {
		log.Fail("render", err.Error())
	}
	e := inv.Entries()[0]
	log.Assert("source_digest", e.SourceDigest == wantDig, wantDig, e.SourceDigest)
	log.Assert("content_digest", e.ContentDigest == wantDig, wantDig, e.ContentDigest)
	log.Assert("equal", e.SourceDigest == e.ContentDigest, e.SourceDigest, e.ContentDigest)
	out, _ := inv.Content("AGENTS.md")
	log.Assert("bytes", bytes.Equal(src, out), true, bytes.Equal(src, out))
	log.Step("digest", testutil.OutcomeOK, fmt.Sprintf(
		"path=%s source=%s mode=%s digest=%s outcome=ok",
		e.Path, e.Source, e.Mode, e.ContentDigest,
	))
	log.PhaseEnd("digest_eq", testutil.OutcomeOK)
}

// TestStaticMissingCatalogSource returns stable catalog.invalid.
func TestStaticMissingCatalogSource(t *testing.T) {
	log := testutil.New(t)
	log.Phase("missing_source")
	cat := fixtureCatalog()
	inv, err := render.RenderStatic(cat, render.StaticJob{
		Path:   "missing.txt",
		Mode:   "0644",
		Source: "core/files/does-not-exist.txt",
	}, nil)
	log.Assert("nil_inventory", inv == nil, true, inv == nil)
	if err == nil {
		log.Fail("expected_error", "want catalog.invalid")
	}
	fe := asFoundry(t, err)
	log.Assert("id", fe.ID() == diagnostic.IDCatalogInvalid,
		diagnostic.IDCatalogInvalid, fe.ID())
	log.Step("missing", testutil.OutcomeOK, fmt.Sprintf(
		"path=missing.txt source=core/files/does-not-exist.txt id=%s outcome=fail",
		fe.ID(),
	))
	log.NotePath("core/files/does-not-exist.txt")
	log.PhaseEnd("missing_source", testutil.OutcomeOK)
}

// TestStaticDeterminism ensures identical inputs produce equal inventories
// under repeated invocation (pair with go test -count=2).
func TestStaticDeterminism(t *testing.T) {
	log := testutil.New(t)
	log.Phase("determinism")
	cat := fixtureCatalog()
	jobs := []render.StaticJob{
		{Path: "LICENSE", Mode: "0644", Source: "core/files/LICENSE", Owner: "core"},
		{Path: "AGENTS.md", Mode: "0644", Source: "core/files/AGENTS.md", Owner: "core"},
		{Path: "scripts/run.sh", Mode: "0755", Source: "core/files/scripts/run.sh", Owner: "core"},
	}

	a, err := render.RenderStaticAll(cat, jobs, render.NewMemoryWriter())
	if err != nil {
		log.Fail("render_a", err.Error())
	}
	b, err := render.RenderStaticAll(cat, jobs, render.NewMemoryWriter())
	if err != nil {
		log.Fail("render_b", err.Error())
	}
	log.Assert("equal", a.Equal(b), true, a.Equal(b))
	for i, e := range a.Entries() {
		log.Assert("path_"+e.Path, e.Path == b.Entries()[i].Path, e.Path, b.Entries()[i].Path)
		log.Assert("digest_"+e.Path, e.ContentDigest == b.Entries()[i].ContentDigest,
			e.ContentDigest, b.Entries()[i].ContentDigest)
		log.Step("file", testutil.OutcomeOK, fmt.Sprintf(
			"path=%s source=%s mode=%s digest=%s outcome=ok",
			e.Path, e.Source, e.Mode, e.ContentDigest,
		))
	}
	log.PhaseEnd("determinism", testutil.OutcomeOK)
}

// TestStaticEmbeddedDistributionFile renders the real catalog static file.
func TestStaticEmbeddedDistributionFile(t *testing.T) {
	log := testutil.New(t)
	log.Phase("embedded")
	cat := mustLoadCatalog(t)
	src := "profiles/distribution/files/releasing.md"
	log.Fixture("source", src)

	raw, err := cat.Read(src)
	if err != nil {
		log.Fail("catalog_read", err.Error())
	}
	log.Step("source_bytes", testutil.OutcomeOK, fmt.Sprintf("n=%d digest=%s", len(raw), digestOf(raw)))

	inv, err := render.RenderStatic(cat, render.StaticJob{
		Path:   "docs/releasing.md",
		Mode:   "0644",
		Source: src,
		Owner:  "profile:distribution",
	}, render.NewMemoryWriter())
	if err != nil {
		log.Fail("render", err.Error())
	}
	e, ok := inv.EntryByPath("docs/releasing.md")
	log.Assert("found", ok, true, ok)
	log.Assert("mechanism", e.Mechanism == render.MechanismStatic, render.MechanismStatic, e.Mechanism)
	log.Assert("owner", e.Owner == "profile:distribution", "profile:distribution", e.Owner)
	out, _ := inv.Content(e.Path)
	log.Assert("byte_identical", bytes.Equal(raw, out), true, bytes.Equal(raw, out))
	log.Assert("digest_eq", e.SourceDigest == e.ContentDigest, e.SourceDigest, e.ContentDigest)
	log.Step("file", testutil.OutcomeOK, fmt.Sprintf(
		"path=%s source=%s mode=%s digest=%s outcome=ok",
		e.Path, e.Source, e.Mode, e.ContentDigest,
	))
	log.PhaseEnd("embedded", testutil.OutcomeOK)
}

// TestStaticInvalidMode rejects modes outside the admitted set.
func TestStaticInvalidMode(t *testing.T) {
	log := testutil.New(t)
	log.Phase("invalid_mode")
	cat := fixtureCatalog()
	_, err := render.RenderStatic(cat, render.StaticJob{
		Path:   "LICENSE",
		Mode:   "0777",
		Source: "core/files/LICENSE",
	}, nil)
	if err == nil {
		log.Fail("expected_error", "want render.failed for mode 0777")
	}
	fe := asFoundry(t, err)
	log.Assert("id", fe.ID() == diagnostic.IDRenderFailed,
		diagnostic.IDRenderFailed, fe.ID())
	log.Step("mode", testutil.OutcomeOK, fmt.Sprintf(
		"path=LICENSE mode=0777 id=%s outcome=fail", fe.ID(),
	))
	log.PhaseEnd("invalid_mode", testutil.OutcomeOK)
}

// TestStaticNilWriterInventoryOnly still returns pure content without a writer.
func TestStaticNilWriterInventoryOnly(t *testing.T) {
	log := testutil.New(t)
	log.Phase("nil_writer")
	cat := fixtureCatalog()
	inv, err := render.RenderStatic(cat, render.StaticJob{
		Path:   "LICENSE",
		Mode:   "0644",
		Source: "core/files/LICENSE",
	}, nil)
	if err != nil {
		log.Fail("render", err.Error())
	}
	out, ok := inv.Content("LICENSE")
	log.Assert("present", ok, true, ok)
	src, _ := cat.Read("core/files/LICENSE")
	log.Assert("bytes", bytes.Equal(src, out), true, bytes.Equal(src, out))
	log.Step("file", testutil.OutcomeOK, fmt.Sprintf(
		"path=LICENSE source=core/files/LICENSE mode=0644 digest=%s outcome=ok",
		inv.Entries()[0].ContentDigest,
	))
	log.PhaseEnd("nil_writer", testutil.OutcomeOK)
}

// TestStaticDuplicateJobPath fails closed on duplicate destinations.
func TestStaticDuplicateJobPath(t *testing.T) {
	log := testutil.New(t)
	log.Phase("dup_path")
	cat := fixtureCatalog()
	_, err := render.RenderStaticAll(cat, []render.StaticJob{
		{Path: "LICENSE", Mode: "0644", Source: "core/files/LICENSE"},
		{Path: "LICENSE", Mode: "0644", Source: "core/files/AGENTS.md"},
	}, nil)
	if err == nil {
		log.Fail("expected_error", "want render.failed on duplicate path")
	}
	fe := asFoundry(t, err)
	log.Assert("id", fe.ID() == diagnostic.IDRenderFailed,
		diagnostic.IDRenderFailed, fe.ID())
	log.Step("dup", testutil.OutcomeOK, fmt.Sprintf(
		"path=LICENSE id=%s outcome=fail", fe.ID(),
	))
	log.PhaseEnd("dup_path", testutil.OutcomeOK)
}

// TestMechanismValid locks the three product mechanisms (REQ-096).
func TestMechanismValid(t *testing.T) {
	log := testutil.New(t)
	log.Phase("mechanism_enum")
	for _, m := range []render.Mechanism{
		render.MechanismStatic,
		render.MechanismTemplate,
		render.MechanismGomod,
	} {
		log.Assert("valid_"+string(m), m.Valid(), true, m.Valid())
	}
	log.Assert("invalid", !render.Mechanism("token").Valid(), false, render.Mechanism("token").Valid())
	log.PhaseEnd("mechanism_enum", testutil.OutcomeOK)
}
