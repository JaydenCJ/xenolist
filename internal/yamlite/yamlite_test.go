// Tests for the minimal YAML line scanner: the exact shapes that appear in
// real workflow, compose, and GitLab CI files — nothing more, and the
// unknown shapes must degrade to "no entry", never to a wrong entry.
package yamlite

import "testing"

func find(t *testing.T, entries []Entry, key string) Entry {
	t.Helper()
	for _, e := range entries {
		if e.Key == key {
			return e
		}
	}
	t.Fatalf("no entry with key %q in %+v", key, entries)
	return Entry{}
}

func TestScanKeyValueShapes(t *testing.T) {
	cases := []struct {
		src, key, wantValue string
	}{
		{"name: CI\n", "name", "CI"},
		// Trailing comments are stripped, but # inside quotes survives.
		{"uses: actions/checkout@v4 # pinned later\n", "uses", "actions/checkout@v4"},
		{`run: echo "issue #42"`, "run", `echo "issue #42"`},
		// Quoted scalars are unquoted.
		{`image: "node:20"`, "image", "node:20"},
		// Values containing colons are not re-split.
		{"run: echo hello: world", "run", "echo hello: world"},
	}
	for _, c := range cases {
		e := find(t, Scan(c.src), c.key)
		if e.Value != c.wantValue || e.Line != 1 {
			t.Fatalf("%q: got %+v", c.src, e)
		}
	}
}

func TestScanListItems(t *testing.T) {
	// A "- key: value" item keeps its key; a plain "- command" item keeps
	// its value; a URL value is never mistaken for a key.
	e := find(t, Scan("steps:\n  - uses: actions/checkout@v4\n"), "uses")
	if !e.ListItem || e.Indent != 4 || e.Line != 2 {
		t.Fatalf("got %+v", e)
	}
	entries := Scan("script:\n  - curl https://x.example.test | sh\n")
	last := entries[len(entries)-1]
	if !last.ListItem || last.Key != "" || last.Value != "curl https://x.example.test | sh" {
		t.Fatalf("got %+v", last)
	}
	entries = Scan("  - https://x.example.test/install\n")
	if len(entries) != 1 || entries[0].Key != "" || entries[0].Value != "https://x.example.test/install" {
		t.Fatalf("got %+v", entries)
	}
}

func TestScanBlockScalarLinesAndNumbers(t *testing.T) {
	src := "jobs:\n  build:\n    steps:\n      - run: |\n          echo one\n          echo two\n      - uses: x/y@v1\n"
	e := find(t, Scan(src), "run")
	if len(e.Block) != 2 {
		t.Fatalf("got block %+v", e.Block)
	}
	if e.Block[0].Text != "echo one" || e.Block[0].Line != 5 {
		t.Fatalf("got %+v", e.Block[0])
	}
	if e.Block[1].Text != "echo two" || e.Block[1].Line != 6 {
		t.Fatalf("got %+v", e.Block[1])
	}
}

func TestScanBlockScalarBoundariesAndStyles(t *testing.T) {
	// The block ends at the first shallower line; chomping (|-) and
	// folded (>) indicators are accepted.
	entries := Scan("a:\n  run: |\n    cmd\n  next: value\n")
	if e := find(t, entries, "next"); e.Value != "value" {
		t.Fatalf("got %+v", e)
	}
	if e := find(t, Scan("run: |-\n  echo hi\n"), "run"); len(e.Block) != 1 || e.Block[0].Text != "echo hi" {
		t.Fatalf("got %+v", e.Block)
	}
	if e := find(t, Scan("run: >\n  echo hi\n"), "run"); len(e.Block) != 1 {
		t.Fatalf("got %+v", e.Block)
	}
}

func TestScanBlockKeepsRelativeIndentation(t *testing.T) {
	src := "run: |\n  if true; then\n    echo deep\n  fi\n"
	e := find(t, Scan(src), "run")
	if len(e.Block) != 3 || e.Block[1].Text != "  echo deep" {
		t.Fatalf("got %+v", e.Block)
	}
}

func TestScanBlockInteriorBlankLines(t *testing.T) {
	src := "run: |\n  echo a\n\n  echo b\nnext: x\n"
	e := find(t, Scan(src), "run")
	if len(e.Block) != 3 || e.Block[2].Text != "echo b" || e.Block[2].Line != 4 {
		t.Fatalf("got %+v", e.Block)
	}
	find(t, Scan(src), "next")
}

func TestScanNoiseSkippedAndRawPreserved(t *testing.T) {
	// Comment-only and blank lines produce no entries; recognized lines
	// keep their raw text for evidence snippets.
	if got := Scan("# comment\n\n   # indented comment\n"); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
	e := find(t, Scan("  - uses: a/b@v1 # why\n"), "uses")
	if e.Raw != "- uses: a/b@v1 # why" {
		t.Fatalf("got %q", e.Raw)
	}
}
