package toolrun_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

func TestCapBytes_DefaultMatchesPlan(t *testing.T) {
	log := testutil.New(t)
	log.Assert("default_cap", toolrun.DefaultOutputCapBytes == plan.OutputCapBytes,
		plan.OutputCapBytes, toolrun.DefaultOutputCapBytes)
	log.Assert("four_mib", toolrun.DefaultOutputCapBytes == 4<<20, 4<<20, toolrun.DefaultOutputCapBytes)
}

func TestCapBytes_ExplicitTruncation(t *testing.T) {
	log := testutil.New(t)
	in := []byte(strings.Repeat("a", 100))
	out, trunc := toolrun.CapBytes(in, 50)
	log.Assert("len", len(out) == 50, 50, len(out))
	log.Assert("truncated", trunc, true, trunc)
	log.Assert("prefix", string(out) == strings.Repeat("a", 50), "a*50", string(out[:min(8, len(out))])+"…")

	out2, trunc2 := toolrun.CapBytes(in, 200)
	log.Assert("no_trunc", !trunc2, false, trunc2)
	log.Assert("full", len(out2) == 100, 100, len(out2))
}

func TestCapBytes_ZeroUsesDefault(t *testing.T) {
	log := testutil.New(t)
	small := []byte("ok")
	out, trunc := toolrun.CapBytes(small, 0)
	log.Assert("small_ok", !trunc && string(out) == "ok", "ok", string(out))

	// Exactly at default boundary: no truncate.
	exact := make([]byte, toolrun.DefaultOutputCapBytes)
	out2, trunc2 := toolrun.CapBytes(exact, 0)
	log.Assert("exact_len", len(out2) == toolrun.DefaultOutputCapBytes, toolrun.DefaultOutputCapBytes, len(out2))
	log.Assert("exact_no_trunc", !trunc2, false, trunc2)

	// One past default: truncate.
	over := make([]byte, toolrun.DefaultOutputCapBytes+1)
	out3, trunc3 := toolrun.CapBytes(over, 0)
	log.Assert("over_len", len(out3) == toolrun.DefaultOutputCapBytes, toolrun.DefaultOutputCapBytes, len(out3))
	log.Assert("over_trunc", trunc3, true, trunc3)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
