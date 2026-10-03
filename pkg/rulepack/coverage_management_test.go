package rulepack

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type brokenOutput struct{}

func (brokenOutput) Write([]byte) (int, error) { return 0, errors.New("output closed") }

func TestManagementCommands(t *testing.T) {
	r := NewRegistry()
	if err := RegisterStock(r); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	config := filepath.Join(root, "rules.yaml")
	if err := os.WriteFile(config, []byte("version: 1\nextends: ["+DefaultPresetName()+"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		args     []string
		code     int
		contains string
	}{
		{[]string{"rules", "list"}, 0, "["},
		{[]string{"rules", "list", "--format", "json"}, 0, "guidance"},
		{[]string{"rules", "describe", r.Catalog()[0].ID}, 0, "id"},
		{[]string{"presets", "list"}, 0, DefaultPresetName()},
		{[]string{"presets", "describe", DefaultPresetName()}, 0, "Hash"},
		{[]string{"presets", "export", DefaultPresetName()}, 0, "version:"},
		{[]string{"config", "schema"}, 0, "properties"},
		{[]string{"config", "validate", "--root", root, "--rules", config}, 0, "Configuration valid"},
		{[]string{"config", "explain", "--root", root, "--rules", config, "--path", "guide.md"}, 0, "Applies"},
		{[]string{"rules"}, 2, "requires an action"},
		{[]string{"rules", "unknown"}, 2, "unknown"},
		{[]string{"rules", "describe"}, 2, "requires"},
		{[]string{"rules", "describe", "missing"}, 2, "unknown"},
		{[]string{"rules", "list", "--format", "yaml"}, 2, "unknown format"},
		{[]string{"rules", "list", "--bad"}, 2, "flag"},
		{[]string{"presets", "unknown"}, 2, "unknown"},
		{[]string{"presets", "describe"}, 2, "requires"},
		{[]string{"presets", "describe", "missing"}, 2, "unknown"},
		{[]string{"init", "--preset", "missing"}, 2, "unknown"},
		{[]string{"init", "--output", root}, 2, ""},
		{[]string{"config", "unknown"}, 2, "unknown"},
		{[]string{"config", "validate", "--rules", filepath.Join(root, "absent")}, 2, ""},
		{[]string{"config", "explain", "--root", root, "--rules", config, "--path", "../escape"}, 2, "root-relative"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			var out, stderr bytes.Buffer
			handled, code := Manage(tc.args, ".lint.yaml", r, &out, &stderr)
			if !handled || code != tc.code || !strings.Contains(out.String()+stderr.String(), tc.contains) {
				t.Fatalf("handled=%v code=%d output=%s stderr=%s", handled, code, out.String(), stderr.String())
			}
		})
	}
	if handled, _ := Manage(nil, ".lint.yaml", r, &bytes.Buffer{}, &bytes.Buffer{}); handled {
		t.Fatal("handled empty command")
	}
	if handled, _ := Manage([]string{"file.md"}, ".lint.yaml", r, &bytes.Buffer{}, &bytes.Buffer{}); handled {
		t.Fatal("handled lint input")
	}
	for _, args := range [][]string{{"rules", "list", "--format", "json"}, {"presets", "export", DefaultPresetName()}, {"config", "schema"}} {
		if _, code := Manage(args, ".lint.yaml", r, brokenOutput{}, &bytes.Buffer{}); code != 2 {
			t.Fatalf("%v output failure code=%d", args, code)
		}
	}
	schema, err := json.Marshal(r.ConfigurationSchema())
	if err != nil || !bytes.Contains(schema, []byte("additionalProperties")) {
		t.Fatalf("schema=%s err=%v", schema, err)
	}
}

func TestStrictBaselineFailures(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "baseline.json")
	for _, source := range []string{"bad", `{"version":2,"findings":[]}`, `{"version":1,"findings":["nothex"]}`, `{"version":1,"findings":["ab"]}`, `{"version":1,"findings":[],"unknown":true}`, `{"version":1,"findings":[]} {}`} {
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := ApplyBaseline(path, root, nil); err == nil {
			t.Fatalf("accepted %s", source)
		}
	}
	if _, _, err := ApplyBaseline(filepath.Join(root, "absent"), root, nil); err == nil {
		t.Fatal("missing baseline accepted")
	}
	if err := WriteBaseline(root, root, nil, false); err == nil {
		t.Fatal("directory baseline accepted")
	}
	translated, err := BaselineArgs([]string{"baseline", "create", "--overwrite", "guide.md"})
	if err != nil || len(translated) != 4 || translated[0] != "--baseline-write" || translated[2] != "--baseline-overwrite" {
		t.Fatalf("%v %v", translated, err)
	}
}

func TestScopeValidationAndMatching(t *testing.T) {
	for _, pattern := range []string{"", "../escape", "/root", `docs\file`, "docs/../file", "**/file", "docs/**/file", "["} {
		if err := validatePattern(pattern); err == nil {
			t.Fatalf("accepted scope %q", pattern)
		}
	}
	for _, tc := range []struct {
		pattern, file string
		want          bool
	}{{"docs/**", "docs", true}, {"docs/**", "docs/nested/file", true}, {"docs/**", "other/file", false}, {"*.md", "guide.md", true}, {"*.md", "nested/guide.md", false}} {
		if got := matches(tc.pattern, tc.file); got != tc.want {
			t.Fatalf("%+v got=%v", tc, got)
		}
	}
	if anyMatch([]string{"a.md"}, "b.md") {
		t.Fatal("unexpected match")
	}
	if _, err := relativePath(t.TempDir(), filepath.Join(t.TempDir(), "file.md")); err == nil {
		t.Fatal("outside relative path accepted")
	}
}
