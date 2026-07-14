// CI-YAML scanning: one generic pass covers GitHub workflows and composite
// actions (`uses:`, `container:`/`services:` images, `run:` blocks),
// docker-compose files (`image:`), GitLab CI (`image:`, `script:` lists),
// and CircleCI (`image:`, `run:`). The YAML surface differs per system;
// the external-code shapes do not.
package scan

import (
	"strings"

	"github.com/JaydenCJ/xenolist/internal/finding"
	"github.com/JaydenCJ/xenolist/internal/yamlite"
)

// scriptKeys introduce a sequence of shell commands in GitLab/CircleCI-style
// configs.
var scriptKeys = map[string]bool{
	"script": true, "before_script": true, "after_script": true,
}

// ScanYAML scans any supported CI/compose YAML file.
func ScanYAML(file, src string) []finding.Finding {
	var out []finding.Finding
	entries := yamlite.Scan(src)
	for i := 0; i < len(entries); i++ {
		e := entries[i]
		switch {
		case e.Key == "uses":
			f, ok := finding.ParseUses(e.Value)
			if !ok {
				continue
			}
			f.File, f.Line, f.Snippet = file, e.Line, snippetOf(e.Raw)
			out = append(out, f)
		case e.Key == "image":
			ref, host, pin, ok := finding.ParseImageRef(e.Value)
			if !ok {
				continue
			}
			out = append(out, finding.Finding{
				File: file, Line: e.Line, Kind: finding.KindImage,
				Ref: ref, Host: host, Pin: pin, Detail: "image",
				Snippet: snippetOf(e.Raw),
			})
		case e.Key == "container" && e.Value != "":
			// Workflow shorthand: `container: node:20`.
			ref, host, pin, ok := finding.ParseImageRef(e.Value)
			if !ok {
				continue
			}
			out = append(out, finding.Finding{
				File: file, Line: e.Line, Kind: finding.KindImage,
				Ref: ref, Host: host, Pin: pin, Detail: "container",
				Snippet: snippetOf(e.Raw),
			})
		case e.Key == "run":
			out = append(out, scanYAMLCommands(file, e)...)
		case scriptKeys[e.Key]:
			out = append(out, scanYAMLCommands(file, e)...)
			// A script key may also introduce a list of commands.
			for j := i + 1; j < len(entries); j++ {
				item := entries[j]
				if item.Indent <= e.Indent || !item.ListItem || item.Key != "" {
					break
				}
				out = append(out, ScanShellText(file, item.Value, item.Line)...)
			}
		}
	}
	return out
}

// scanYAMLCommands feeds an inline or block-scalar command value through
// the shell scanner, preserving real line numbers for block scalars.
func scanYAMLCommands(file string, e yamlite.Entry) []finding.Finding {
	if e.Value != "" {
		return ScanShellText(file, e.Value, e.Line)
	}
	if len(e.Block) == 0 {
		return nil
	}
	var b strings.Builder
	for k, bl := range e.Block {
		if k > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(bl.Text)
	}
	return ScanShellText(file, b.String(), e.Block[0].Line)
}
