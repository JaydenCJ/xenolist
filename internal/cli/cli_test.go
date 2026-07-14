// In-process CLI integration tests: Run() is exercised end-to-end against
// fabricated repository trees in temp dirs — the same surface the binary
// exposes, without building it.
package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// demoTree fabricates a small repository touching every scanner.
func demoTree(t *testing.T) string {
	t.Helper()
	return writeTree(t, map[string]string{
		".github/workflows/ci.yml": `jobs:
  build:
    steps:
      - uses: actions/checkout@8f4b7f84864484a7bf31766abe9204da3cbe65b3
      - uses: actions/setup-go@v5
      - run: |
          curl -fsSL https://get.example.test/i.sh | bash
          npx cowsay hi
`,
		"Dockerfile":       "FROM golang:1.22 AS build\nFROM alpine:latest\nCOPY --from=build /a /a\n",
		"scripts/setup.sh": "go run golang.org/x/tools/cmd/stringer@latest -h\n",
	})
}

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestVersionSubcommandAndAlias(t *testing.T) {
	code, out, _ := run(t, "version")
	if code != ExitOK || out != "xenolist 0.1.0\n" {
		t.Fatalf("got %d %q", code, out)
	}
	code, out, _ = run(t, "--version")
	if code != ExitOK || !strings.Contains(out, "0.1.0") {
		t.Fatalf("got %d %q", code, out)
	}
}

func TestHelpPrintsUsage(t *testing.T) {
	code, out, _ := run(t, "help")
	if code != ExitOK || !strings.Contains(out, "Usage:") || !strings.Contains(out, "xenolist check") {
		t.Fatalf("got %d %q", code, out)
	}
}

func TestUsageErrorsExitTwo(t *testing.T) {
	root := demoTree(t)
	cases := []struct {
		args       []string
		wantStderr string
	}{
		{[]string{"--bogus"}, "unknown flag"},
		{[]string{"scan", "--format", "yaml", root}, `unknown --format "yaml"`},
		{[]string{"scan", "--kind", "sorcery", root}, `unknown --kind "sorcery"`},
		{[]string{"scan", "a", "b"}, "at most one path"},
		{[]string{"check", root}, "--max-sources"},
	}
	for _, c := range cases {
		code, _, errOut := run(t, c.args...)
		if code != ExitUsage || !strings.Contains(errOut, c.wantStderr) {
			t.Fatalf("%v: got %d %q", c.args, code, errOut)
		}
	}
}

func TestMissingPathIsRuntimeError(t *testing.T) {
	code, _, errOut := run(t, "scan", filepath.Join(t.TempDir(), "nope"))
	if code != ExitRuntime || errOut == "" {
		t.Fatalf("got %d %q", code, errOut)
	}
}

func TestScanSummaryOnDemoTree(t *testing.T) {
	root := demoTree(t)
	code, out, _ := run(t, "scan", root)
	if code != ExitOK {
		t.Fatalf("got %d", code)
	}
	for _, want := range []string{
		"external code sources: 7",
		"github-action",
		"container-image",
		"floating sources (4)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	// A bare path is treated as `scan <path>`, and output is stable.
	code, again, _ := run(t, root)
	if code != ExitOK || again != out {
		t.Fatalf("bare path scan differs: %d", code)
	}
}

func TestScanJSONIsMachineReadable(t *testing.T) {
	code, out, _ := run(t, "scan", "--format", "json", demoTree(t))
	if code != ExitOK {
		t.Fatalf("got %d", code)
	}
	var doc struct {
		SchemaVersion int `json:"schema_version"`
		Totals        struct {
			Sources  int `json:"sources"`
			Pinned   int `json:"pinned"`
			Floating int `json:"floating"`
		} `json:"totals"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if doc.SchemaVersion != 1 || doc.Totals.Sources != 7 || doc.Totals.Pinned != 1 || doc.Totals.Floating != 4 {
		t.Fatalf("got %+v", doc)
	}
}

func TestListShowsEveryOccurrenceWithEvidence(t *testing.T) {
	code, out, _ := run(t, "list", demoTree(t))
	if code != ExitOK {
		t.Fatalf("got %d", code)
	}
	if !strings.Contains(out, "7 occurrences of 7 external code sources") {
		t.Fatalf("got:\n%s", out)
	}
	if !strings.Contains(out, "└─ curl | bash: curl -fsSL https://get.example.test/i.sh | bash") {
		t.Fatalf("evidence missing:\n%s", out)
	}
}

func TestKindAndExcludeFilters(t *testing.T) {
	root := demoTree(t)
	code, out, _ := run(t, "scan", "--kind", "github-action", root)
	if code != ExitOK || !strings.Contains(out, "external code sources: 2") {
		t.Fatalf("--kind: got %d:\n%s", code, out)
	}
	code, out, _ = run(t, "scan", "--exclude", "scripts/**", root)
	if code != ExitOK || !strings.Contains(out, "external code sources: 6") {
		t.Fatalf("--exclude: got %d:\n%s", code, out)
	}
}

func TestCheckLimits(t *testing.T) {
	root := demoTree(t)
	code, out, _ := run(t, "check", "--max-sources", "10", root)
	if code != ExitOK || !strings.Contains(out, "check: PASS") {
		t.Fatalf("under limit: got %d %q", code, out)
	}
	code, out, _ = run(t, "check", "--max-floating", "1", root)
	if code != ExitBreach || !strings.Contains(out, "BREACH") || !strings.Contains(out, "check: FAIL") {
		t.Fatalf("over limit: got %d %q", code, out)
	}
}

func TestCheckAllowHost(t *testing.T) {
	code, out, _ := run(t, "check",
		"--allow-host", "github.com",
		"--allow-host", "docker.io",
		"--allow-host", "registry.npmjs.org",
		"--allow-host", "proxy.golang.org",
		demoTree(t))
	if code != ExitBreach || !strings.Contains(out, "host get.example.test not allowed") {
		t.Fatalf("got %d %q", code, out)
	}
	root := writeTree(t, map[string]string{"Dockerfile": "FROM alpine:3.19\n"})
	code, out, _ = run(t, "check", "--allow-host", "docker.io", root)
	if code != ExitOK || !strings.Contains(out, "check: PASS") {
		t.Fatalf("got %d %q", code, out)
	}
}

func TestEmptyTreeScansClean(t *testing.T) {
	root := writeTree(t, map[string]string{"README.md": "# hi\n"})
	code, out, _ := run(t, "scan", root)
	if code != ExitOK || !strings.Contains(out, "external code sources: 0") {
		t.Fatalf("got %d %q", code, out)
	}
}
