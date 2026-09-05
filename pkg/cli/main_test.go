package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/markdown-linter/pkg/cli"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func command(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestCustomerPackControlsIdentitySeverityAndSuppression(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "guide.md"), "# Customer guide\n")
	config := filepath.Join(root, "rules.yaml")
	pack := `version: 1
rules:
  - id: customer.purpose
    check: markdown.required-heading
    severity: warning
    options:
      heading: Purpose
      level: 2
`
	write(t, config, pack)
	args := []string{"--root", root, "--rules", config, "--format", "json", root}
	code, out, stderr := command(args...)
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	var diagnostics []interfaces.Diagnostic
	if err := json.Unmarshal([]byte(out), &diagnostics); err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].RuleID != "customer.purpose" || diagnostics[0].Severity != interfaces.SeverityWarning || diagnostics[0].Line != 1 {
		t.Fatalf("diagnostics: %+v", diagnostics)
	}
	// Repeated runs must produce byte-for-byte identical machine output.
	_, again, _ := command(args...)
	if out != again {
		t.Fatalf("nondeterministic output")
	}
	write(t, config, strings.Replace(pack, "warning", "error", 1))
	if code, _, _ := command(args...); code != 1 {
		t.Fatalf("error exit=%d", code)
	}
	write(t, config, pack+`suppressions:
  - rule: customer.purpose
    path: guide.md
    reason: Intentional tutorial exception
`)
	if code, out, stderr := command(args...); code != 0 || strings.TrimSpace(out) != "[]" {
		t.Fatalf("code=%d out=%s err=%s", code, out, stderr)
	}
}

func TestExplicitPackDoesNotAddDefaults(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "guide.md"), "# Guide\n\n[bad](missing.md)\n")
	config := filepath.Join(root, "rules.yaml")
	write(t, config, "version: 1\nrules: []\n")
	if code, out, stderr := command("--root", root, "--rules", config, root); code != 0 || out != "" || stderr != "" {
		t.Fatalf("%d %s %s", code, out, stderr)
	}
}

func TestInvalidUnselectedCheckFailsBeforeLinting(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "rules.yaml")
	write(t, config, `version: 1
rules:
  - id: good
    check: markdown.heading-order
  - id: bad
    check: customer.unknown
`)
	code, _, stderr := command("--rules", config, "--only", "good", root)
	if code != 2 || !strings.Contains(stderr, "unknown check") {
		t.Fatalf("%d %s", code, stderr)
	}
}

func TestInputAndLinkRootBoundaries(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside.md")
	write(t, outside, "# Outside\n")
	write(t, filepath.Join(root, "guide.md"), "# Guide\n\n[private](../outside.md#outside)\n")
	if code, _, stderr := command("--root", root, outside); code != 2 || !strings.Contains(stderr, "escapes document root") {
		t.Fatalf("input: %d %s", code, stderr)
	}
	if code, out, stderr := command("--root", root, root); code != 1 || !strings.Contains(out, "outside document root") {
		t.Fatalf("link: %d %s %s", code, out, stderr)
	}
	if err := os.Symlink(outside, filepath.Join(root, "alias.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if code, _, stderr := command("--root", root, filepath.Join(root, "alias.md")); code != 2 {
		t.Fatalf("symlink input: %d %s", code, stderr)
	}
	write(t, filepath.Join(root, "guide.md"), "# Guide\n\n[private](alias.md#outside)\n")
	if code, out, stderr := command("--root", root, filepath.Join(root, "guide.md")); code != 1 || !strings.Contains(out, "outside document root") {
		t.Fatalf("symlink link: %d %s %s", code, out, stderr)
	}
}
