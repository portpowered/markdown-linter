package rulepack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedManifestMatchesSourceFiles(t *testing.T) {
	data, e := embeddedPacks.ReadFile("packs/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	var manifest []struct{ Name, File, SHA256 string }
	if e = json.Unmarshal(data, &manifest); e != nil {
		t.Fatal(e)
	}
	if len(manifest) != len(PresetNames()) {
		t.Fatal("manifest membership differs")
	}
	for _, entry := range manifest {
		data, e := embeddedPacks.ReadFile("packs/" + entry.File)
		if e != nil {
			t.Fatal(e)
		}
		hash := sha256.Sum256(data)
		if entry.SHA256 != hex.EncodeToString(hash[:]) {
			t.Fatalf("%s hash mismatch", entry.Name)
		}
		if _, ok := Preset(entry.Name); !ok {
			t.Fatalf("missing preset %s", entry.Name)
		}
	}
}

func putPack(t *testing.T, root, name, content string) string {
	t.Helper()
	p := filepath.Join(root, name)
	if e := os.WriteFile(p, []byte(content), 0600); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestCompositionAndOverrides(t *testing.T) {
	root := t.TempDir()
	putPack(t, root, "base.yaml", `version: 1
rules:
  - id: custom.base
    check: customer.check
    severity: error
    options: {one: first, two: second}
`)
	p := putPack(t, root, "root.yaml", `version: 1
extends: [./base.yaml]
overrides:
  - id: custom.base
    enabled: false
    severity: warning
    options: {three: third}
rules: []
`)
	pack, e := Load(p, root)
	if e != nil {
		t.Fatal(e)
	}
	if len(pack.Rules) != 1 || pack.Rules[0].Enabled == nil || *pack.Rules[0].Enabled || pack.Rules[0].Severity != "warning" {
		t.Fatalf("%#v", pack)
	}
	var opts map[string]string
	if e = pack.Rules[0].Options.Decode(&opts); e != nil {
		t.Fatal(e)
	}
	if len(opts) != 1 || opts["three"] != "third" {
		t.Fatalf("options were merged: %#v", opts)
	}
	if len(pack.Imports) != 2 || !strings.Contains(pack.Rules[0].Origin, "override:") {
		t.Fatalf("missing provenance %#v", pack)
	}
}
func TestCompositionRejectsUnsafeAndAmbiguousImports(t *testing.T) {
	for _, tt := range []struct{ name, body, expect string }{
		{"cycle", "version: 1\nextends: [./root.yaml]\nrules: []\n", "cycle"},
		{"unknown", "version: 1\nextends: [missing:preset]\nrules: []\n", "unknown preset"},
		{"override", "version: 1\noverrides: [{id: missing, enabled: false}]\nrules: []\n", "override"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			p := putPack(t, root, "root.yaml", tt.body)
			_, e := Load(p, root)
			if e == nil || !strings.Contains(e.Error(), tt.expect) {
				t.Fatalf("%v", e)
			}
		})
	}
	parent := t.TempDir()
	outside := putPack(t, parent, "outside.yaml", "version: 1\nrules: []\n")
	root := filepath.Join(parent, "inside")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	p := putPack(t, root, "root.yaml", "version: 1\nextends: [../outside.yaml]\nrules: []\n")
	if _, e := Load(p, root); e == nil {
		t.Fatalf("accepted outside import %s", outside)
	}
}
func TestDiamondDependencyLoadedOnce(t *testing.T) {
	root := t.TempDir()
	putPack(t, root, "base.yaml", "version: 1\nrules: [{id: base, check: customer.check}]\n")
	putPack(t, root, "a.yaml", "version: 1\nextends: [base.yaml]\nrules: []\n")
	putPack(t, root, "b.yaml", "version: 1\nextends: [base.yaml]\nrules: []\n")
	p := putPack(t, root, "root.yaml", "version: 1\nextends: [a.yaml, b.yaml]\nrules: []\n")
	pack, e := Load(p, root)
	if e != nil || len(pack.Rules) != 1 {
		t.Fatalf("%#v %v", pack, e)
	}
}
