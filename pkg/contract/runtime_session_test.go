package contract

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMDXSessionMatchesIsolatedAndStopsWorker(t *testing.T) {
	ctx, closeRuntime := parserRuntimeContext(context.Background())
	defer closeRuntime()
	var worker int
	for _, source := range []string{
		"# Devices 😀\r\n\r\nRead **café** and `API`.\r\n",
		"import NeverExecute from 'not-installed';\n\n<NeverExecute>{untrusted()}</NeverExecute>\n",
		"<Unclosed>\n", "# Changed input\n\n| Key | Value |\n| --- | --- |\n| α | β |\n",
	} {
		before, err := parseRuntime(context.Background(), "mdx", source)
		if err != nil {
			t.Fatal(err)
		}
		after, err := parseRuntime(ctx, "mdx", source)
		if err != nil || !reflect.DeepEqual(after, before) {
			t.Fatalf("stream differs from isolated parser: %v\n%+v\n%+v", err, after, before)
		}
		session := ctx.Value(runtimeSessionKey{}).(*runtimeSession)
		if session.command == nil {
			t.Fatal("installed runtime did not use its declared stream protocol")
		}
		if worker != 0 && session.command.Process.Pid != worker {
			t.Fatal("restarted the worker between documents")
		}
		worker = session.command.Process.Pid
	}
	command := ctx.Value(runtimeSessionKey{}).(*runtimeSession).command
	closeRuntime()
	closeRuntime()
	session := ctx.Value(runtimeSessionKey{}).(*runtimeSession)
	session.mu.Lock()
	stopped := session.command == nil
	session.mu.Unlock()
	if command.ProcessState == nil || !stopped {
		t.Fatal("worker was not reaped when its owner closed")
	}
	if _, err := parseRuntime(ctx, "mdx", "Text."); err == nil {
		t.Fatal("closed session accepted another request")
	}
}

func sessionRuntime(t *testing.T, source, manifest string) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "analyze.mjs"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if manifest != "" {
		if err := os.WriteFile(filepath.Join(directory, "package.json"), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("MARKLINT_RUNTIME", directory)
	return directory
}

func TestMDXSessionLeavesCustomOneShotRuntimesCompatible(t *testing.T) {
	for _, manifest := range []string{"", "not-json", `{"marklintMdxStream":2}`} {
		t.Run(manifest, func(t *testing.T) {
			sessionRuntime(t, `import {readFileSync} from 'node:fs';
if(process.argv.includes('--stream-mdx')) throw Error('unexpected protocol');
const request=JSON.parse(readFileSync(0,'utf8'));
process.stdout.write(JSON.stringify({ok:true,tree:{type:request.source}}));`, manifest)
			ctx, closeRuntime := parserRuntimeContext(context.Background())
			defer closeRuntime()
			result, err := parseRuntime(ctx, "mdx", "original protocol")
			if err != nil || result.Tree.Type != "original protocol" {
				t.Fatalf("custom runtime changed: %+v, %v", result, err)
			}
			if ctx.Value(runtimeSessionKey{}).(*runtimeSession).command != nil {
				t.Fatal("started a stream for an undeclared protocol")
			}
		})
	}
}

func TestMDXSessionTransportFailuresPoisonTheWorker(t *testing.T) {
	for _, source := range []string{
		`process.exit(0);`,
		`import {createInterface} from 'node:readline'; for await(const line of createInterface({input:process.stdin})) {process.stdout.write('not-json\n');}`,
	} {
		t.Run(source, func(t *testing.T) {
			sessionRuntime(t, source, `{"marklintMdxStream":1}`)
			ctx, closeRuntime := parserRuntimeContext(context.Background())
			defer closeRuntime()
			_, first := parseRuntime(ctx, "mdx", "Text.")
			if first == nil || !strings.Contains(first.Error(), "analysis runtime") {
				t.Fatalf("transport failure accepted: %v", first)
			}
			_, second := parseRuntime(ctx, "mdx", "Changed.")
			if second != first {
				t.Fatalf("restarted a failed stream: %v, %v", first, second)
			}
		})
	}
}

func TestMDXSessionCancelsBlockedWritesAndReads(t *testing.T) {
	for _, source := range []string{
		`setInterval(()=>{},1000);`,
		`import {createInterface} from 'node:readline'; for await(const line of createInterface({input:process.stdin})) {await new Promise(()=>{});}`,
	} {
		t.Run(source, func(t *testing.T) {
			sessionRuntime(t, source, `{"marklintMdxStream":1}`)
			ctx, closeRuntime := parserRuntimeContext(context.Background())
			defer closeRuntime()
			limited, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			_, err := parseRuntime(limited, "mdx", strings.Repeat("x", 2<<20))
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("blocked request did not observe its deadline: %v", err)
			}
			if ctx.Value(runtimeSessionKey{}).(*runtimeSession).command != nil {
				t.Fatal("cancelled worker was not reaped")
			}
		})
	}
}

func TestMDXSessionStartupFailureAndCancelledOwner(t *testing.T) {
	directory := sessionRuntime(t, "", `{"marklintMdxStream":1}`)
	t.Setenv("PATH", t.TempDir())
	ctx, closeRuntime := parserRuntimeContext(context.Background())
	defer closeRuntime()
	payload, err := json.Marshal(map[string]string{"mode": "mdx", "source": "Text."})
	if err != nil {
		t.Fatal(err)
	}
	session := ctx.Value(runtimeSessionKey{}).(*runtimeSession)
	if _, handled, err := session.request(ctx, filepath.Join(directory, "analyze.mjs"), payload); !handled || err == nil {
		t.Fatal("missing executable did not fail")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := session.request(cancelled, "unused", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context was ignored: %v", err)
	}
}
