// Markdown rendering: a paste-into-the-PR census table.
package render

import (
	"fmt"
	"io"

	"github.com/JaydenCJ/xenolist/internal/census"
)

// Markdown renders the census as a Markdown report.
func Markdown(w io.Writer, r census.Report) {
	fmt.Fprintf(w, "## xenolist census — %s\n\n", r.Root)
	pinned, tagged, floating := r.PinCounts()
	fmt.Fprintf(w, "**%s** across %s — %d pinned, %d tagged, %d floating.\n\n",
		pluralize(len(r.Sources), "external code source", "external code sources"),
		pluralize(r.FilesScanned, "scanned file", "scanned files"),
		pinned, tagged, floating)
	if len(r.Sources) == 0 {
		fmt.Fprintf(w, "Nothing in this tree executes code from the internet.\n")
		return
	}
	fmt.Fprintf(w, "| Kind | Source | Pin | Host | Uses |\n")
	fmt.Fprintf(w, "|---|---|---|---|---|\n")
	for _, s := range r.Sources {
		fmt.Fprintf(w, "| %s | `%s` | %s | %s | %d |\n",
			s.Kind, s.Ref, s.Pin, s.Host, len(s.Occurrences))
	}
	if floating > 0 {
		fmt.Fprintf(w, "\n### Floating sources\n\n")
		for _, s := range r.Floating() {
			occ := s.Occurrences[0]
			fmt.Fprintf(w, "- `%s` — %s at %s:%d\n", s.Ref, s.Kind, occ.File, occ.Line)
		}
	}
}
