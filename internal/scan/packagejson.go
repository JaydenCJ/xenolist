// package.json scanning: every value in the "scripts" object is a shell
// command a contributor (or CI) will run, so each one goes through the
// shared shell scanner. Line numbers are recovered by locating the script
// name inside the raw text, since encoding/json does not report positions.
package scan

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/JaydenCJ/xenolist/internal/finding"
)

// ScanPackageJSON scans npm manifest scripts. Files that are not valid
// JSON, or have no scripts object, produce no findings.
func ScanPackageJSON(file, src string) []finding.Finding {
	var manifest struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal([]byte(src), &manifest); err != nil || len(manifest.Scripts) == 0 {
		return nil
	}
	lines := strings.Split(src, "\n")
	scriptsAt := 0
	for i, l := range lines {
		if strings.Contains(l, `"scripts"`) {
			scriptsAt = i + 1
			break
		}
	}
	names := make([]string, 0, len(manifest.Scripts))
	for name := range manifest.Scripts {
		names = append(names, name)
	}
	sort.Strings(names) // deterministic output regardless of map order

	var out []finding.Finding
	for _, name := range names {
		line := scriptsAt
		needle := `"` + name + `"`
		for i := scriptsAt; i < len(lines); i++ {
			if strings.Contains(lines[i], needle) {
				line = i + 1
				break
			}
		}
		if line == 0 {
			line = 1
		}
		out = append(out, ScanShellText(file, manifest.Scripts[name], line)...)
	}
	sortFindings(out)
	return out
}

// sortFindings orders findings by line then ref, so multi-script files
// report in file order.
func sortFindings(fs []finding.Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		if fs[i].Line != fs[j].Line {
			return fs[i].Line < fs[j].Line
		}
		return fs[i].Ref < fs[j].Ref
	})
}
