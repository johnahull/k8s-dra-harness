package verification

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewCreatesEvidenceWithoutScripts(t *testing.T) {
	evidence := filepath.Join(t.TempDir(), "evidence")
	runner, err := New(Config{EvidenceDir: evidence}, "")
	if err != nil {
		t.Fatal(err)
	}
	if runner.Enabled() {
		t.Fatal("runner with no scripts should be disabled")
	}
	if _, err := os.Stat(evidence); err != nil {
		t.Fatalf("evidence directory was not created: %v", err)
	}
}

func TestWriteJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := writeJSON(path, map[string]any{"items": []string{"one"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded["items"].([]any)) != 1 {
		t.Fatalf("snapshot has unexpected items: %s", data)
	}
}

func TestSanitize(t *testing.T) {
	if got := sanitize("topology/counters --run"); strings.ContainsAny(got, "/ ") {
		t.Fatalf("sanitize()=%q contains path or whitespace", got)
	}
}

func TestResolveRejectsCommandOutsideScriptsDir(t *testing.T) {
	dir := t.TempDir()
	runner := &Runner{config: Config{ScriptsDir: dir}}
	outside := filepath.Join(dir, "..", "outside.sh")
	if _, _, err := runner.resolve([]string{outside}); err == nil || !strings.Contains(err.Error(), "outside scriptsDir") {
		t.Fatalf("resolve() error=%v, want outside scriptsDir", err)
	}
}

func TestResolveRunsPythonScriptsThroughPython(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "check.py")
	if err := os.WriteFile(path, []byte("print('ok')\n"), 0700); err != nil {
		t.Fatal(err)
	}
	runner := &Runner{config: Config{ScriptsDir: dir}}
	command, args, err := runner.resolve([]string{"check.py", "one"})
	if err != nil {
		t.Fatal(err)
	}
	if command != "python3" || len(args) != 2 || args[0] != path || args[1] != "one" {
		t.Fatalf("resolve()=(%q, %#v), want python3 and %q", command, args, path)
	}
}
