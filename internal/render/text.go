// Package render turns a census.Report into terminal text, stable JSON
// (schema_version 1), and PR-ready Markdown. All output is deterministic:
// same tree in, same bytes out.
package render

import (
	"fmt"
	"io"

	"github.com/JaydenCJ/xenolist/internal/census"
)

// fileOrder fixes how file categories appear in the header line.
var fileOrder = []string{
	"workflow", "action", "dockerfile", "compose file", "ci config",
	"shell script", "makefile", "package.json",
}

// plurals maps a category label to its plural form for the header.
var plurals = map[string]string{
	"workflow": "workflows", "action": "actions", "dockerfile": "dockerfiles",
	"compose file": "compose files", "ci config": "ci configs",
	"shell script": "shell scripts", "makefile": "makefiles",
	"package.json": "package.json",
}

// Scan renders the summary census.
func Scan(w io.Writer, r census.Report) {
	fmt.Fprintf(w, "xenolist scan — %s\n", r.Root)
	fmt.Fprintf(w, "files scanned: %d%s\n\n", r.FilesScanned, fileBreakdown(r))

	if len(r.Sources) == 0 {
		fmt.Fprintf(w, "external code sources: 0 — nothing in this tree executes code from the internet\n")
		return
	}
	pinned, tagged, floating := r.PinCounts()
	fmt.Fprintf(w, "external code sources: %d   (%d pinned · %d tagged · %d floating)\n\n",
		len(r.Sources), pinned, tagged, floating)

	fmt.Fprintf(w, "by kind                  sources   floating\n")
	for _, kc := range r.ByKind() {
		fmt.Fprintf(w, "  %-22s %7d %10d\n", kc.Kind, kc.Sources, kc.Floating)
	}
	fmt.Fprintf(w, "\nby host                        sources\n")
	for _, hc := range r.ByHost() {
		fmt.Fprintf(w, "  %-28s %7d\n", hc.Host, hc.Sources)
	}
	if floating > 0 {
		fmt.Fprintf(w, "\nfloating sources (%d)\n", floating)
		for _, s := range r.Floating() {
			occ := s.Occurrences[0]
			fmt.Fprintf(w, "  %-34s %-16s %s\n",
				fmt.Sprintf("%s:%d", occ.File, occ.Line), s.Kind, s.Ref)
		}
	}
}

// List renders every finding with its evidence line.
func List(w io.Writer, r census.Report) {
	fmt.Fprintf(w, "%s of %s (%s scanned)\n\n",
		pluralize(len(r.Findings), "occurrence", "occurrences"),
		pluralize(len(r.Sources), "external code source", "external code sources"),
		pluralize(r.FilesScanned, "file", "files"))
	if len(r.Findings) == 0 {
		fmt.Fprintf(w, "nothing in this tree executes code from the internet\n")
		return
	}
	for _, f := range r.Findings {
		fmt.Fprintf(w, "%s:%d  %s  %s  [%s]\n", f.File, f.Line, f.Kind, f.Ref, f.Pin)
		fmt.Fprintf(w, "         └─ %s: %s\n", f.Detail, f.Snippet)
	}
}

func fileBreakdown(r census.Report) string {
	out := ""
	for _, label := range fileOrder {
		n := r.FileCounts[label]
		if n == 0 {
			continue
		}
		name := plurals[label]
		if n == 1 {
			name = label
		}
		if out != "" {
			out += ", "
		}
		out += fmt.Sprintf("%d %s", n, name)
	}
	if out == "" {
		return ""
	}
	return " (" + out + ")"
}

// pluralize formats "1 occurrence" / "3 occurrences" — reports read like
// prose, and "1 occurrences" would undermine a tool built on precision.
func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}
