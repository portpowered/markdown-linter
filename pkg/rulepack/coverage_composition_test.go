package rulepack

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompositionRejectsDuplicateRulesAndInvalidSources(t *testing.T) {
	root := t.TempDir()
	putPack(t, root, "base.yaml", "version: 1\nrules: [{id: base, check: customer.check}]\n")
	for _, source := range []string{"version: 1\nextends: [base.yaml]\nrules: [{id: base, check: customer.check}]\n", "version: 1\nextends: [base.yaml]\noverrides: [{id: base, enabled: true}, {id: base, enabled: false}]\n", "version: 1\ninvalid: true\n", "version: 1\nextends: [missing.yaml]\n"} {
		path := putPack(t, root, "rules.yaml", source)
		if _, err := Load(path, root); err == nil {
			t.Fatalf("accepted %s", source)
		}
	}
	if _, err := Load("openapi:core", filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing root accepted")
	}
	path := putPack(t, root, "rules.yaml", "version: 1\nextends: [base.yaml]\noverrides:\n  - id: base\n    include: [api.yaml]\n    exclude: [generated/**]\n")
	p, err := Load(path, root)
	if err != nil || len(p.Rules[0].Include) != 1 || p.Rules[0].Exclude[0] != "generated/**" {
		t.Fatalf("%+v %v", p, err)
	}
}
func TestManagementRejectsInvalidConfigurationAndPreservesOverwriteIntent(t *testing.T) {
	r := NewRegistry()
	if err := RegisterStock(r); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	path := filepath.Join(root, "rules.yaml")
	var out, stderr bytes.Buffer
	for _, args := range [][]string{{"init", "--kind", "wrong", "--output", path}, {"config", "validate", "--root", root, "--rules", path}} {
		if _, code := Manage(args, "unused", r, &out, &stderr); code != 2 {
			t.Fatalf("%v code=%d", args, code)
		}
	}
	if err := os.WriteFile(path, []byte("version: 1\nrules: [{id: missing, check: missing}]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, code := Manage([]string{"config", "validate", "--root", root, "--rules", path}, "unused", r, &out, &stderr); code != 2 {
		t.Fatal("invalid check accepted")
	}
	if _, code := Manage([]string{"init", "--output", path, "--overwrite"}, "unused", r, &out, &stderr); code != 0 {
		t.Fatalf("overwrite failed %d %s", code, stderr.String())
	}
	if _, code := Manage([]string{"config", "validate", "--root", root, "--rules", path, "--kind", "wrong"}, "unused", r, &out, &stderr); code != 2 {
		t.Fatal("wrong mode accepted")
	}
	if _, code := Manage([]string{"init", "--output", filepath.Join(root, "second.yaml")}, "unused", r, brokenOutput{}, &stderr); code != 2 {
		t.Fatal("init output failure accepted")
	}
	if !strings.Contains(out.String(), "Created") {
		t.Fatal(out.String())
	}
}
