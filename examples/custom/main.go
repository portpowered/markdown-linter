// A customer command registers new executable logic through the public API.
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/portpowered/markdown-linter/pkg/cli"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/portpowered/markdown-linter/pkg/rulepack"
	"gopkg.in/yaml.v3"
)

type requiredText struct {
	Text string `yaml:"text"`
}

func (requiredText) ID() string { return "customer.required-text" }
func (r requiredText) Analyze(ctx context.Context, pass *interfaces.Pass) {
	for _, doc := range pass.Documents {
		if ctx.Err() != nil {
			return
		}
		if !bytes.Contains(doc.Source, []byte(r.Text)) {
			pass.Report(interfaces.NewDiagnostic(doc.Path, 1, -1, -1, r.ID(), "missing required text: "+r.Text, interfaces.SeverityError))
		}
	}
}
func main() {
	registry := rulepack.NewRegistry()
	if err := rulepack.RegisterStock(registry); err != nil {
		panic(err)
	}
	if err := registry.Register("customer.required-text", func(node yaml.Node) (interfaces.Analyzer, error) {
		var check requiredText
		if err := rulepack.DecodeOptions(node, &check); err != nil {
			return nil, err
		}
		if check.Text == "" {
			return nil, fmt.Errorf("text is required")
		}
		return check, nil
	}); err != nil {
		panic(err)
	}
	os.Exit(cli.RunWithRegistry(context.Background(), os.Args[1:], os.Stdout, os.Stderr, registry))
}
