//go:build tools

// Package tools retains Section 44.1 BOM module pins in go.mod until product
// packages import them for real work. go mod tidy considers //go:build tools.
//
// Remove individual blank imports as diagnostic/spec/cli/render adopt each
// module. Do not import this package from product code.
package tools

import (
	_ "github.com/BurntSushi/toml"
	// cobra is imported by internal/cli (product); keep tools pin only if needed
	// for go mod tidy with tools build tag alone — re-export for BOM completeness.
	_ "github.com/spf13/cobra"
	_ "golang.org/x/mod/modfile"
	_ "golang.org/x/mod/module"
)
