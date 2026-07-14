// Package census aggregates raw findings into the one report the tool
// exists to produce: deduplicated external sources with pinning grades,
// kind and host rollups, and a deterministic ordering so identical trees
// always render byte-identical output.
package census

import (
	"sort"

	"github.com/JaydenCJ/xenolist/internal/finding"
)

// Source is one deduplicated external code source (kind + ref) with every
// place it occurs.
type Source struct {
	Kind        finding.Kind
	Ref         string
	Host        string
	Pin         finding.Pin
	Occurrences []finding.Finding
}

// KindCount is a per-kind rollup row.
type KindCount struct {
	Kind     finding.Kind
	Sources  int
	Floating int
}

// HostCount is a per-host rollup row.
type HostCount struct {
	Host    string
	Sources int
}

// Report is the full census for one repository tree.
type Report struct {
	Root         string         // base name of the scanned root
	FilesScanned int            // classified files actually read
	FileCounts   map[string]int // label → count, e.g. "workflow" → 3
	Findings     []finding.Finding
	Sources      []Source
}

// Build deduplicates findings into sources and freezes all orderings.
func Build(root string, fileCounts map[string]int, findings []finding.Finding) Report {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].Ref < findings[j].Ref
	})
	index := map[string]int{}
	var sources []Source
	for _, f := range findings {
		key := f.SourceKey()
		if i, ok := index[key]; ok {
			sources[i].Occurrences = append(sources[i].Occurrences, f)
			// A source is only as safe as its loosest occurrence.
			if pinRank(f.Pin) > pinRank(sources[i].Pin) {
				sources[i].Pin = f.Pin
			}
			continue
		}
		index[key] = len(sources)
		sources = append(sources, Source{
			Kind: f.Kind, Ref: f.Ref, Host: f.Host, Pin: f.Pin,
			Occurrences: []finding.Finding{f},
		})
	}
	sort.SliceStable(sources, func(i, j int) bool {
		if sources[i].Kind != sources[j].Kind {
			return kindRank(sources[i].Kind) < kindRank(sources[j].Kind)
		}
		return sources[i].Ref < sources[j].Ref
	})
	total := 0
	for _, n := range fileCounts {
		total += n
	}
	return Report{
		Root: root, FilesScanned: total, FileCounts: fileCounts,
		Findings: findings, Sources: sources,
	}
}

// PinCounts returns how many unique sources sit at each pin level.
func (r Report) PinCounts() (pinned, tagged, floating int) {
	for _, s := range r.Sources {
		switch s.Pin {
		case finding.PinPinned:
			pinned++
		case finding.PinTag:
			tagged++
		default:
			floating++
		}
	}
	return
}

// ByKind returns kind rollups, largest first, ties by canonical kind order.
func (r Report) ByKind() []KindCount {
	byKind := map[finding.Kind]*KindCount{}
	for _, s := range r.Sources {
		kc := byKind[s.Kind]
		if kc == nil {
			kc = &KindCount{Kind: s.Kind}
			byKind[s.Kind] = kc
		}
		kc.Sources++
		if s.Pin == finding.PinFloating {
			kc.Floating++
		}
	}
	var out []KindCount
	for _, kc := range byKind {
		out = append(out, *kc)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sources != out[j].Sources {
			return out[i].Sources > out[j].Sources
		}
		return kindRank(out[i].Kind) < kindRank(out[j].Kind)
	})
	return out
}

// ByHost returns host rollups, largest first, ties alphabetical.
func (r Report) ByHost() []HostCount {
	byHost := map[string]int{}
	for _, s := range r.Sources {
		byHost[s.Host]++
	}
	var out []HostCount
	for h, n := range byHost {
		out = append(out, HostCount{Host: h, Sources: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sources != out[j].Sources {
			return out[i].Sources > out[j].Sources
		}
		return out[i].Host < out[j].Host
	})
	return out
}

// Floating returns the unpinned sources, in report order.
func (r Report) Floating() []Source {
	var out []Source
	for _, s := range r.Sources {
		if s.Pin == finding.PinFloating {
			out = append(out, s)
		}
	}
	return out
}

func kindRank(k finding.Kind) int {
	for i, kk := range finding.AllKinds {
		if kk == k {
			return i
		}
	}
	return len(finding.AllKinds)
}

func pinRank(p finding.Pin) int {
	switch p {
	case finding.PinPinned:
		return 0
	case finding.PinTag:
		return 1
	default:
		return 2
	}
}
