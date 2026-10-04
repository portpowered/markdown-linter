// Package cli exposes the version-2 command with stock or customer check registries.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/markdown-linter/pkg/contract"
	"github.com/portpowered/markdown-linter/pkg/rulepack"
)

const (
	ExitOK          = 0
	ExitViolation   = 1
	ExitOperational = 2
)

var Version = "dev"

func Run(ctx context.Context, args []string, out, errOut io.Writer) int {
	registry := rulepack.NewRegistry()
	if err := rulepack.RegisterStock(registry); err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return ExitOperational
	}
	return RunWithRegistry(ctx, args, out, errOut, registry)
}

// RunWithRegistry executes one contract, including trusted customer document adapters.
func RunWithRegistry(ctx context.Context, args []string, out, errOut io.Writer, registry *rulepack.Registry) int {
	return contract.Run(ctx, args, os.Stdin, out, errOut, registry, Version)
}
