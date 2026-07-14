// Tests for census aggregation: deduplication, pin escalation, rollups,
// and — because reports get diffed in code review — deterministic order.
package census

import (
	"testing"

	"github.com/JaydenCJ/xenolist/internal/finding"
)

func f(file string, line int, kind finding.Kind, ref string, pin finding.Pin) finding.Finding {
	return finding.Finding{File: file, Line: line, Kind: kind, Ref: ref, Host: "example.test", Pin: pin}
}

func TestBuildDeduplicatesByKindAndRef(t *testing.T) {
	r := Build("repo", map[string]int{"workflow": 2}, []finding.Finding{
		f("a.yml", 1, finding.KindAction, "x/y@v1", finding.PinTag),
		f("b.yml", 3, finding.KindAction, "x/y@v1", finding.PinTag),
	})
	if len(r.Sources) != 1 || len(r.Sources[0].Occurrences) != 2 {
		t.Fatalf("got %+v", r.Sources)
	}
	if len(r.Findings) != 2 {
		t.Fatalf("got %d findings", len(r.Findings))
	}
	// The same ref reached through a different mechanism stays separate.
	r = Build("repo", nil, []finding.Finding{
		f("a.yml", 1, finding.KindPipe, "https://x.example.test", finding.PinFloating),
		f("b.sh", 1, finding.KindDownload, "https://x.example.test", finding.PinFloating),
	})
	if len(r.Sources) != 2 {
		t.Fatalf("got %+v", r.Sources)
	}
}

func TestBuildFindingsSortedByFileAndLine(t *testing.T) {
	r := Build("repo", nil, []finding.Finding{
		f("b.yml", 9, finding.KindAction, "x/y@v1", finding.PinTag),
		f("a.yml", 5, finding.KindAction, "p/q@v2", finding.PinTag),
		f("a.yml", 2, finding.KindAction, "m/n@v3", finding.PinTag),
	})
	if r.Findings[0].File != "a.yml" || r.Findings[0].Line != 2 || r.Findings[2].File != "b.yml" {
		t.Fatalf("got %+v", r.Findings)
	}
}

func TestBuildPinEscalatesToLoosestOccurrence(t *testing.T) {
	// The same action pinned in one workflow and floating in another is a
	// floating source: an attacker only needs the loosest edge.
	r := Build("repo", nil, []finding.Finding{
		{File: "a.yml", Line: 1, Kind: finding.KindAction, Ref: "x/y", Pin: finding.PinPinned},
		{File: "b.yml", Line: 1, Kind: finding.KindAction, Ref: "x/y", Pin: finding.PinFloating},
	})
	if r.Sources[0].Pin != finding.PinFloating {
		t.Fatalf("got %s", r.Sources[0].Pin)
	}
}

func TestPinCounts(t *testing.T) {
	r := Build("repo", nil, []finding.Finding{
		f("a", 1, finding.KindAction, "a@sha", finding.PinPinned),
		f("a", 2, finding.KindAction, "b@v1", finding.PinTag),
		f("a", 3, finding.KindAction, "c@main", finding.PinFloating),
		f("a", 4, finding.KindImage, "d:latest", finding.PinFloating),
	})
	p, tg, fl := r.PinCounts()
	if p != 1 || tg != 1 || fl != 2 {
		t.Fatalf("got %d %d %d", p, tg, fl)
	}
}

func TestByKindLargestFirst(t *testing.T) {
	r := Build("repo", nil, []finding.Finding{
		f("a", 1, finding.KindImage, "x:1", finding.PinTag),
		f("a", 2, finding.KindImage, "y:2", finding.PinTag),
		f("a", 3, finding.KindAction, "p/q@v1", finding.PinTag),
	})
	kinds := r.ByKind()
	if kinds[0].Kind != finding.KindImage || kinds[0].Sources != 2 {
		t.Fatalf("got %+v", kinds)
	}
}

func TestByHostTiesAlphabetical(t *testing.T) {
	r := Build("repo", nil, []finding.Finding{
		{File: "a", Line: 1, Kind: finding.KindPipe, Ref: "u1", Host: "b.example.test", Pin: finding.PinFloating},
		{File: "a", Line: 2, Kind: finding.KindPipe, Ref: "u2", Host: "a.example.test", Pin: finding.PinFloating},
	})
	hosts := r.ByHost()
	if hosts[0].Host != "a.example.test" {
		t.Fatalf("got %+v", hosts)
	}
}

func TestFloatingListPreservesReportOrder(t *testing.T) {
	r := Build("repo", nil, []finding.Finding{
		f("a", 1, finding.KindAction, "a@main", finding.PinFloating),
		f("a", 2, finding.KindImage, "img:latest", finding.PinFloating),
		f("a", 3, finding.KindAction, "b@v1", finding.PinTag),
	})
	fl := r.Floating()
	if len(fl) != 2 || fl[0].Kind != finding.KindAction || fl[1].Kind != finding.KindImage {
		t.Fatalf("got %+v", fl)
	}
	if r2 := Build("repo", map[string]int{"workflow": 2, "dockerfile": 1}, nil); r2.FilesScanned != 3 {
		t.Fatalf("file counts total: got %d", r2.FilesScanned)
	}
}
