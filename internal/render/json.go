// Stable JSON rendering. schema_version identifies the envelope layout;
// bump it only for breaking changes, per the README's compatibility note.
package render

import (
	"encoding/json"
	"io"

	"github.com/JaydenCJ/xenolist/internal/census"
	"github.com/JaydenCJ/xenolist/internal/finding"
	"github.com/JaydenCJ/xenolist/internal/version"
)

type jsonOccurrence struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Detail  string `json:"detail"`
	Snippet string `json:"snippet"`
}

type jsonSource struct {
	Kind        finding.Kind     `json:"kind"`
	Ref         string           `json:"ref"`
	Host        string           `json:"host"`
	Pin         finding.Pin      `json:"pin"`
	Occurrences []jsonOccurrence `json:"occurrences"`
}

type jsonTotals struct {
	Sources     int `json:"sources"`
	Occurrences int `json:"occurrences"`
	Pinned      int `json:"pinned"`
	Tagged      int `json:"tagged"`
	Floating    int `json:"floating"`
}

type jsonReport struct {
	Tool          string         `json:"tool"`
	Version       string         `json:"version"`
	SchemaVersion int            `json:"schema_version"`
	Root          string         `json:"root"`
	FilesScanned  int            `json:"files_scanned"`
	FileCounts    map[string]int `json:"file_counts"`
	Totals        jsonTotals     `json:"totals"`
	Sources       []jsonSource   `json:"sources"`
}

// JSON renders the census as an indented, stably-ordered JSON document.
func JSON(w io.Writer, r census.Report) error {
	pinned, tagged, floating := r.PinCounts()
	doc := jsonReport{
		Tool:          "xenolist",
		Version:       version.Version,
		SchemaVersion: 1,
		Root:          r.Root,
		FilesScanned:  r.FilesScanned,
		FileCounts:    r.FileCounts,
		Totals: jsonTotals{
			Sources: len(r.Sources), Occurrences: len(r.Findings),
			Pinned: pinned, Tagged: tagged, Floating: floating,
		},
		Sources: []jsonSource{},
	}
	for _, s := range r.Sources {
		js := jsonSource{Kind: s.Kind, Ref: s.Ref, Host: s.Host, Pin: s.Pin}
		for _, o := range s.Occurrences {
			js.Occurrences = append(js.Occurrences, jsonOccurrence{
				File: o.File, Line: o.Line, Detail: o.Detail, Snippet: o.Snippet,
			})
		}
		doc.Sources = append(doc.Sources, js)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
