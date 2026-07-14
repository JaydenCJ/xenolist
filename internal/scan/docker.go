// Dockerfile scanning: base images (FROM), cross-image copies
// (COPY --from), remote downloads (ADD <url>), and every RUN instruction —
// including JSON-array form and heredocs — funneled into the shared shell
// scanner so `curl | bash` inside an image build is caught by the same
// rules as everywhere else.
package scan

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/JaydenCJ/xenolist/internal/finding"
)

var heredocRe = regexp.MustCompile(`<<-?["']?([A-Za-z_][A-Za-z0-9_]*)["']?`)

// ScanDockerfile scans Dockerfile/Containerfile content.
func ScanDockerfile(file, src string) []finding.Finding {
	var out []finding.Finding
	lines := strings.Split(src, "\n")
	// Stage aliases (FROM … AS build) are the repo's own intermediate
	// images; COPY --from=build must not count as external.
	stages := map[string]bool{}

	for i := 0; i < len(lines); i++ {
		startLine := i + 1
		raw := strings.TrimSpace(lines[i])
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		// Join backslash continuations; comment lines inside a
		// continuation are dropped, as BuildKit does.
		full := raw
		for strings.HasSuffix(full, "\\") && i+1 < len(lines) {
			i++
			next := strings.TrimSpace(lines[i])
			if strings.HasPrefix(next, "#") {
				full = strings.TrimSuffix(full, "\\") + "\\"
				continue
			}
			full = strings.TrimSuffix(full, "\\") + " " + next
		}
		full = strings.TrimSuffix(full, "\\")

		instr, rest, _ := strings.Cut(full, " ")
		rest = strings.TrimSpace(rest)
		snippet := snippetOf(full)

		switch strings.ToUpper(instr) {
		case "FROM":
			out = append(out, scanFrom(file, startLine, rest, snippet, stages)...)
		case "COPY":
			out = append(out, scanCopyFrom(file, startLine, rest, snippet, stages)...)
		case "ADD":
			for _, tok := range splitTokens(rest) {
				if strings.HasPrefix(tok, "http://") || strings.HasPrefix(tok, "https://") {
					out = append(out, finding.Finding{
						File: file, Line: startLine, Kind: finding.KindDownload,
						Ref: tok, Host: finding.HostOfURL(tok),
						Pin: finding.ClassifyURL(tok), Detail: "ADD", Snippet: snippet,
					})
				}
			}
		case "RUN":
			body, bodyLine, consumed := runBody(rest, lines, i, startLine)
			i = consumed
			out = append(out, ScanShellText(file, body, bodyLine)...)
		}
	}
	return out
}

func scanFrom(file string, line int, rest, snippet string, stages map[string]bool) []finding.Finding {
	toks := splitTokens(rest)
	image := ""
	for j := 0; j < len(toks); j++ {
		t := toks[j]
		if strings.HasPrefix(t, "--") {
			continue
		}
		if image == "" {
			image = t
			continue
		}
		if strings.EqualFold(t, "AS") && j+1 < len(toks) {
			stages[strings.ToLower(toks[j+1])] = true
			break
		}
	}
	if image == "" || stages[strings.ToLower(image)] {
		return nil
	}
	ref, host, pin, ok := finding.ParseImageRef(image)
	if !ok {
		return nil
	}
	return []finding.Finding{{
		File: file, Line: line, Kind: finding.KindImage,
		Ref: ref, Host: host, Pin: pin, Detail: "FROM", Snippet: snippet,
	}}
}

func scanCopyFrom(file string, line int, rest, snippet string, stages map[string]bool) []finding.Finding {
	for _, tok := range splitTokens(rest) {
		src, ok := strings.CutPrefix(tok, "--from=")
		if !ok {
			continue
		}
		if stages[strings.ToLower(src)] || isDigits(src) {
			return nil // named or numbered local build stage
		}
		ref, host, pin, imgOK := finding.ParseImageRef(src)
		if !imgOK {
			return nil
		}
		return []finding.Finding{{
			File: file, Line: line, Kind: finding.KindImage,
			Ref: ref, Host: host, Pin: pin, Detail: "COPY --from", Snippet: snippet,
		}}
	}
	return nil
}

// runBody extracts the shell text of a RUN instruction: plain commands,
// `RUN ["sh", "-c", "…"]` JSON form, and `RUN <<EOF … EOF` heredocs.
// It returns the body, the 1-based line the body starts on, and the index
// of the last consumed physical line.
func runBody(rest string, lines []string, i, startLine int) (string, int, int) {
	// Strip BuildKit flags such as --mount=… and --network=….
	for strings.HasPrefix(rest, "--") {
		_, after, found := strings.Cut(rest, " ")
		if !found {
			return "", startLine, i
		}
		rest = strings.TrimSpace(after)
	}
	if strings.HasPrefix(rest, "[") {
		var argv []string
		if err := json.Unmarshal([]byte(rest), &argv); err == nil {
			// ["sh", "-c", "script"] — the script is the real command.
			if len(argv) >= 3 && argv[1] == "-c" {
				return strings.Join(argv[2:], " "), startLine, i
			}
			return strings.Join(argv, " "), startLine, i
		}
		return rest, startLine, i
	}
	if m := heredocRe.FindStringSubmatch(rest); m != nil {
		delim := m[1]
		var body []string
		j := i + 1
		for ; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == delim {
				break
			}
			body = append(body, lines[j])
		}
		return strings.Join(body, "\n"), i + 2, j
	}
	return rest, startLine, i
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
