package markdownlint_test

import (
	"context"
	engine "github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	rules "github.com/portpowered/markdown-linter/pkg/rules"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLinter_GoldenOutput_MatchesExpectedRuleOutput(t *testing.T) {
	tests := []struct {
		name       string
		inputPath  string
		goldenPath string
		rule       interfaces.Rule
	}{
		{
			name:       "local links valid",
			inputPath:  "testdata/golden/link-valid.md",
			goldenPath: "testdata/golden/link-valid.golden",
			rule:       rules.NewLocalLinkRule(),
		},
		{
			name:       "local links invalid",
			inputPath:  "testdata/golden/link-invalid.md",
			goldenPath: "testdata/golden/link-invalid.golden",
			rule:       rules.NewLocalLinkRule(),
		},
		{
			name:       "ordered list valid",
			inputPath:  "testdata/golden/ordered-list-valid.md",
			goldenPath: "testdata/golden/ordered-list-valid.golden",
			rule:       rules.NewOrderedListRule(),
		},
		{
			name:       "ordered list invalid",
			inputPath:  "testdata/golden/ordered-list-invalid.md",
			goldenPath: "testdata/golden/ordered-list-invalid.golden",
			rule:       rules.NewOrderedListRule(),
		},
		{
			name:       "heading order valid",
			inputPath:  "testdata/golden/heading-order-valid.md",
			goldenPath: "testdata/golden/heading-order-valid.golden",
			rule:       rules.NewHeadingOrderRule(),
		},
		{
			name:       "heading order invalid",
			inputPath:  "testdata/golden/heading-order-invalid.md",
			goldenPath: "testdata/golden/heading-order-invalid.golden",
			rule:       rules.NewHeadingOrderRule(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := engine.New(engine.WithRules(tt.rule))
			inputPath := fixturePath(tt.inputPath)
			goldenPath := fixturePath(tt.goldenPath)
			fixtureRoot := fixturePath("testdata")
			violations, err := l.RunFile(context.Background(), inputPath)
			if err != nil {
				t.Fatalf("RunFile returned error: %v", err)
			}

			got := renderGoldenViolations(violations, fixtureRoot)
			wantBytes, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden file: %v", err)
			}
			want := string(wantBytes)

			if got != want {
				t.Fatalf("golden output mismatch for %s\nwant:\n%s\ngot:\n%s", tt.inputPath, want, got)
			}
		})
	}
}

func renderGoldenViolations(violations []interfaces.Violation, fixtureRoot string) string {
	fixturePrefix := filepath.ToSlash(fixtureRoot)
	var builder strings.Builder
	for _, violation := range violations {
		line := strings.ReplaceAll(violation.String(), "\\", "/")
		if strings.HasPrefix(line, fixturePrefix) {
			line = "testdata" + strings.TrimPrefix(line, fixturePrefix)
		}
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	return builder.String()
}

func fixturePath(rel string) string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("unable to resolve test fixture path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", rel))
}
