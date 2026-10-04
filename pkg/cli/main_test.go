package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestVersionAndBreakingConfiguration(t *testing.T) {
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"--version"}, &out, &stderr); code != 0 || !strings.Contains(out.String(), "marklint") {
		t.Fatal(code, out.String(), stderr.String())
	}
	out.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"--rules", "unused", "--json"}, &out, &stderr); code != 2 || !strings.Contains(out.String(), `"schemaVersion":2`) {
		t.Fatal(code, out.String())
	}
}
