// Tests for the three renderers. Reports get pasted into PRs and parsed by
// scripts, so the assertions pin exact strings, valid JSON, and stability.
package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaydenCJ/xenolist/internal/census"
	"github.com/JaydenCJ/xenolist/internal/finding"
)

func sampleReport() census.Report {
	return census.Build("demo", map[string]int{"workflow": 1, "dockerfile": 1}, []finding.Finding{
		{File: "ci.yml", Line: 3, Kind: finding.KindAction, Ref: "actions/checkout@v4",
			Host: "github.com", Pin: finding.PinTag, Detail: "uses", Snippet: "- uses: actions/checkout@v4"},
		{File: "Dockerfile", Line: 1, Kind: finding.KindImage, Ref: "alpine:latest",
			Host: "docker.io", Pin: finding.PinFloating, Detail: "FROM", Snippet: "FROM alpine:latest"},
	})
}

func TestTextScanSummaryLines(t *testing.T) {
	var buf bytes.Buffer
	Scan(&buf, sampleReport())
	out := buf.String()
	for _, want := range []string{
		"xenolist scan — demo",
		"files scanned: 2 (1 workflow, 1 dockerfile)",
		"external code sources: 2   (0 pinned · 1 tagged · 1 floating)",
		"floating sources (1)",
		"alpine:latest",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	// Reports get diffed, so the output must be byte-identical per input.
	var again bytes.Buffer
	Scan(&again, sampleReport())
	if again.String() != out {
		t.Fatal("scan output not byte-identical")
	}
}

func TestTextScanEmptyTree(t *testing.T) {
	var buf bytes.Buffer
	Scan(&buf, census.Build("empty", nil, nil))
	if !strings.Contains(buf.String(), "external code sources: 0") {
		t.Fatalf("got %q", buf.String())
	}
}

func TestListShowsEvidence(t *testing.T) {
	var buf bytes.Buffer
	List(&buf, sampleReport())
	out := buf.String()
	if !strings.Contains(out, "ci.yml:3  github-action  actions/checkout@v4  [tag]") {
		t.Fatalf("got:\n%s", out)
	}
	if !strings.Contains(out, "└─ uses: - uses: actions/checkout@v4") {
		t.Fatalf("evidence missing:\n%s", out)
	}
}

func TestSingularCountsReadAsProse(t *testing.T) {
	// "1 occurrences" would undermine a tool built on precision, so the
	// text and Markdown headers must pluralize correctly at n=1.
	one := census.Build("demo", map[string]int{"dockerfile": 1}, []finding.Finding{
		{File: "Dockerfile", Line: 1, Kind: finding.KindImage, Ref: "alpine:latest",
			Host: "docker.io", Pin: finding.PinFloating, Detail: "FROM", Snippet: "FROM alpine:latest"},
	})
	var list bytes.Buffer
	List(&list, one)
	if !strings.Contains(list.String(), "1 occurrence of 1 external code source (1 file scanned)") {
		t.Fatalf("got:\n%s", list.String())
	}
	var md bytes.Buffer
	Markdown(&md, one)
	if !strings.Contains(md.String(), "**1 external code source** across 1 scanned file") {
		t.Fatalf("got:\n%s", md.String())
	}
}

func TestJSONEnvelope(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, sampleReport()); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if doc["tool"] != "xenolist" || doc["schema_version"] != float64(1) {
		t.Fatalf("got %v", doc)
	}
	totals := doc["totals"].(map[string]any)
	if totals["sources"] != float64(2) || totals["floating"] != float64(1) {
		t.Fatalf("got %v", totals)
	}
}

func TestJSONSourcesCarryOccurrences(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, sampleReport()); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Sources []struct {
			Kind        string `json:"kind"`
			Occurrences []struct {
				File string `json:"file"`
				Line int    `json:"line"`
			} `json:"occurrences"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Sources) != 2 || doc.Sources[0].Kind != "github-action" ||
		doc.Sources[0].Occurrences[0].Line != 3 {
		t.Fatalf("got %+v", doc.Sources)
	}
}

func TestJSONEmptyReportHasEmptyArray(t *testing.T) {
	// "sources": [] — never null; JSON consumers should not need nil checks.
	var buf bytes.Buffer
	if err := JSON(&buf, census.Build("empty", nil, nil)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"sources": []`) {
		t.Fatalf("got %s", buf.String())
	}
}

func TestMarkdownTable(t *testing.T) {
	var buf bytes.Buffer
	Markdown(&buf, sampleReport())
	out := buf.String()
	if !strings.Contains(out, "| Kind | Source | Pin | Host | Uses |") {
		t.Fatalf("got:\n%s", out)
	}
	if !strings.Contains(out, "| container-image | `alpine:latest` | floating | docker.io | 1 |") {
		t.Fatalf("got:\n%s", out)
	}
	if !strings.Contains(out, "### Floating sources") {
		t.Fatalf("got:\n%s", out)
	}
	var empty bytes.Buffer
	Markdown(&empty, census.Build("empty", nil, nil))
	if !strings.Contains(empty.String(), "Nothing in this tree executes code from the internet.") {
		t.Fatalf("got %q", empty.String())
	}
}
