// Tests for file discovery and classification: which files enter the
// census, which directories are never descended into, and how the
// include/exclude filters compose.
package walk

import (
	"os"
	"path/filepath"
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

func rels(files []File) []string {
	var out []string
	for _, f := range files {
		out = append(out, f.Rel)
	}
	return out
}

func TestCollectClassifiesAllSurfaces(t *testing.T) {
	root := writeTree(t, map[string]string{
		".github/workflows/ci.yml": "",
		"action.yml":               "",
		"Dockerfile":               "",
		"deploy/api.dockerfile":    "",
		"docker-compose.yml":       "",
		".gitlab-ci.yml":           "",
		".circleci/config.yml":     "",
		"scripts/setup.sh":         "",
		"Makefile":                 "",
		"package.json":             "{}",
		"README.md":                "",
		"main.go":                  "",
	})
	files, err := Collect(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 10 {
		t.Fatalf("want 10 candidates, got %d: %v", len(files), rels(files))
	}
}

func TestCollectSkipsDependencyDirs(t *testing.T) {
	root := writeTree(t, map[string]string{
		"node_modules/pkg/package.json": "{}",
		"vendor/lib/Makefile":           "",
		".git/hooks/pre-commit.sh":      "",
		"Dockerfile":                    "",
	})
	files, err := Collect(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Rel != "Dockerfile" {
		t.Fatalf("got %v", rels(files))
	}
}

func TestCollectSortedByPath(t *testing.T) {
	root := writeTree(t, map[string]string{
		"z.sh": "", "a.sh": "", "m/x.sh": "",
	})
	files, err := Collect(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	got := rels(files)
	want := []string{"a.sh", "m/x.sh", "z.sh"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v", got)
		}
	}
}

func TestCollectIncludeExcludeGlobs(t *testing.T) {
	root := writeTree(t, map[string]string{
		"Dockerfile": "", "scripts/a.sh": "", "examples/b.sh": "",
	})
	files, err := Collect(Options{Root: root, Exclude: []string{"examples/**"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("exclude: got %v", rels(files))
	}
	files, err = Collect(Options{Root: root, Include: []string{"**/*.sh"}, Exclude: []string{"examples/**"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Rel != "scripts/a.sh" {
		t.Fatalf("include: got %v", rels(files))
	}
}

func TestCollectSizeCap(t *testing.T) {
	root := writeTree(t, map[string]string{
		"big.sh":   "0123456789",
		"small.sh": "ok",
	})
	files, err := Collect(Options{Root: root, MaxFileSize: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Rel != "small.sh" {
		t.Fatalf("got %v", rels(files))
	}
}

func TestClassifyRandomYAMLNotScanned(t *testing.T) {
	// values.yaml, k8s manifests etc. are config, not executed CI files;
	// scanning them would over-report (tracked on the roadmap instead).
	if _, _, ok := Classify("chart/values.yaml"); ok {
		t.Fatal("random yaml must not be classified")
	}
}

func TestClassifyLabels(t *testing.T) {
	cases := map[string]string{
		".github/workflows/ci.yml": "workflow",
		"actions/setup/action.yml": "action",
		"Dockerfile.prod":          "dockerfile",
		"Containerfile":            "dockerfile",
		"compose.yaml":             "compose file",
		".gitlab-ci.yml":           "ci config",
		"tools/build.mk":           "makefile",
		"web/package.json":         "package.json",
	}
	for rel, want := range cases {
		_, label, ok := Classify(rel)
		if !ok || label != want {
			t.Fatalf("%q: got %q %v, want %q", rel, label, ok, want)
		}
	}
}

func TestMatchSegmentWildcards(t *testing.T) {
	cases := []struct {
		pattern, rel string
		want         bool
	}{
		{"*.sh", "setup.sh", true},
		{"*.sh", "deep/dir/setup.sh", true}, // no slash: matches base name
		{"scripts/*.sh", "scripts/a.sh", true},
		{"scripts/*.sh", "scripts/sub/a.sh", false},
		{"**/*.sh", "a/b/c.sh", true},
		{"examples/**", "examples/x/y.sh", true},
		{"examples/**", "src/examples.sh", false},
		{"Dockerfile.?", "Dockerfile.a", true},
		{"a*c", "abc", true},
		{"a*c", "ab", false},
	}
	for _, c := range cases {
		if got := Match(c.pattern, c.rel); got != c.want {
			t.Fatalf("Match(%q, %q) = %v, want %v", c.pattern, c.rel, got, c.want)
		}
	}
	// ** must also match zero segments.
	if !Match("**/Dockerfile", "Dockerfile") {
		t.Fatal("** must match zero segments")
	}
}
