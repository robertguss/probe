//go:build !unix

package generate

import (
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/fsx"
)

func dupStageDescriptor(stage *fsx.Stage) (int, error) {
	_ = stage
	return -1, diagnostic.New(
		diagnostic.IDInternalBug,
		"descriptor-bound stage dup requires unix",
		diagnostic.Location{},
	)
}
