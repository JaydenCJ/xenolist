// Package yamlite is a deliberately small, line-oriented YAML scanner —
// just enough structure to walk CI configuration files without a full
// YAML engine: key/value pairs with indentation, sequence items, comment
// stripping, quoted scalars, and literal/folded block scalars with exact
// line numbers. It never allocates a document tree and never guesses at
// anchors, tags, or flow collections; unknown shapes simply produce no
// entries, which is the safe behavior for a scanner.
package yamlite

import "strings"

// BlockLine is one dedented line of a literal (|) or folded (>) block
// scalar, carrying its 1-based line number in the original file.
type BlockLine struct {
	Line int
	Text string
}

// Entry is one scanned YAML line of interest.
type Entry struct {
	Line     int    // 1-based line number
	Indent   int    // effective indentation (past any "- " markers)
	ListItem bool   // the line began a sequence item
	Key      string // empty for plain sequence items
	Value    string // inline scalar value (comment-stripped, unquoted)
	Block    []BlockLine
	Raw      string // trimmed original line, for evidence snippets
}

// Scan walks src and returns every key/value pair and sequence item it can
// recognize, in file order.
func Scan(src string) []Entry {
	lines := strings.Split(src, "\n")
	var out []Entry
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		indent := 0
		for indent < len(raw) && raw[indent] == ' ' {
			indent++
		}
		body := raw[indent:]
		trimmed := strings.TrimSpace(body)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(body, "\t") {
			continue
		}
		listItem := false
		for strings.HasPrefix(body, "- ") || body == "-" {
			listItem = true
			if body == "-" {
				body = ""
				break
			}
			body = body[2:]
			indent += 2
			for strings.HasPrefix(body, " ") { // "-   key:" style
				body = body[1:]
				indent++
			}
		}
		if body == "" {
			continue
		}
		e := Entry{Line: i + 1, Indent: indent, ListItem: listItem, Raw: trimmed}
		key, val, hasKey := splitKey(body)
		if !hasKey {
			if listItem {
				e.Value = unquote(stripComment(body))
				if e.Value != "" {
					out = append(out, e)
				}
			}
			continue
		}
		e.Key = key
		if style, ok := blockIndicator(val); ok {
			var block []BlockLine
			block, i = readBlock(lines, i, indent, style)
			e.Block = block
		} else {
			e.Value = unquote(stripComment(val))
		}
		out = append(out, e)
	}
	return out
}

// splitKey splits "key: value" at the first colon that terminates a plain
// key (colon followed by space or end of line, key contains no whitespace).
// "https://x" is not a key; "run: echo a:b" splits at the first colon.
func splitKey(body string) (key, val string, ok bool) {
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == ' ' || c == '\t' || c == '#' {
			return "", "", false // whitespace before any colon: not a mapping key
		}
		if c == ':' {
			if i == 0 {
				return "", "", false
			}
			if i+1 == len(body) {
				return unquote(body[:i]), "", true
			}
			if body[i+1] == ' ' || body[i+1] == '\t' {
				return unquote(body[:i]), strings.TrimSpace(body[i+1:]), true
			}
			return "", "", false // "https://…" or "a:b" — not a key
		}
	}
	return "", "", false
}

// stripComment removes a trailing " # …" comment, honoring single and
// double quotes so `echo "#5"` survives intact.
func stripComment(s string) string {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '#' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t'):
			return strings.TrimSpace(s[:i])
		}
	}
	return strings.TrimSpace(s)
}

// unquote strips one matching pair of surrounding quotes.
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// blockIndicator reports whether an inline value announces a block scalar
// ("|", ">", with optional chomping/indentation modifiers and a comment).
func blockIndicator(val string) (byte, bool) {
	v := strings.TrimSpace(val)
	if v == "" || (v[0] != '|' && v[0] != '>') {
		return 0, false
	}
	rest := strings.TrimLeft(v[1:], "+-0123456789")
	rest = strings.TrimSpace(rest)
	if rest == "" || strings.HasPrefix(rest, "#") {
		return v[0], true
	}
	return 0, false
}

// readBlock consumes the lines of a block scalar that starts on lines[i]
// (whose key sits at keyIndent), returning the dedented body and the index
// of the last consumed line.
func readBlock(lines []string, i, keyIndent int, _ byte) ([]BlockLine, int) {
	var raws []int
	end := i
	for j := i + 1; j < len(lines); j++ {
		line := lines[j]
		if strings.TrimSpace(line) == "" {
			raws = append(raws, j)
			continue
		}
		indent := 0
		for indent < len(line) && line[indent] == ' ' {
			indent++
		}
		if indent <= keyIndent {
			break
		}
		raws = append(raws, j)
		end = j
	}
	// Trim trailing blank lines, then find the common indentation.
	for len(raws) > 0 && strings.TrimSpace(lines[raws[len(raws)-1]]) == "" {
		raws = raws[:len(raws)-1]
	}
	common := -1
	for _, j := range raws {
		if strings.TrimSpace(lines[j]) == "" {
			continue
		}
		indent := 0
		for indent < len(lines[j]) && lines[j][indent] == ' ' {
			indent++
		}
		if common == -1 || indent < common {
			common = indent
		}
	}
	var block []BlockLine
	for _, j := range raws {
		text := lines[j]
		if len(text) >= common && common > 0 {
			text = text[common:]
		} else {
			text = strings.TrimLeft(text, " ")
		}
		block = append(block, BlockLine{Line: j + 1, Text: text})
	}
	if len(raws) > 0 {
		last := raws[len(raws)-1]
		if last > end {
			end = last
		}
	}
	return block, end
}
