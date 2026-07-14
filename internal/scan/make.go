// Makefile scanning: recipe lines (tab-indented, with @/-/+ prefixes and
// backslash continuations) go through the shared shell scanner, and
// `$(shell …)` expansions anywhere in the file are unwrapped and scanned —
// a `$(shell curl …)` in a variable runs at parse time, before any target.
package scan

import (
	"strings"

	"github.com/JaydenCJ/xenolist/internal/finding"
)

// ScanMakefile scans GNU-make syntax.
func ScanMakefile(file, src string) []finding.Finding {
	var out []finding.Finding
	lines := strings.Split(src, "\n")
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		startLine := i + 1
		if strings.HasPrefix(raw, "\t") {
			cmd := strings.TrimLeft(raw[1:], "@-+ \t")
			for strings.HasSuffix(cmd, "\\") && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "\t") {
				i++
				cmd = strings.TrimSuffix(cmd, "\\") + " " + strings.TrimSpace(lines[i][1:])
			}
			// Make escapes a shell dollar as $$; undo before scanning.
			cmd = strings.ReplaceAll(cmd, "$$", "$")
			out = append(out, ScanShellText(file, cmd, startLine)...)
			continue
		}
		// Parse-time execution: VAR := $(shell curl …)
		rest := raw
		for {
			pos := strings.Index(rest, "$(shell ")
			if pos < 0 {
				break
			}
			inner := balancedInner(rest[pos+len("$(shell "):])
			out = append(out, ScanShellText(file, inner, startLine)...)
			rest = rest[pos+len("$(shell ")+len(inner):]
		}
	}
	return out
}
