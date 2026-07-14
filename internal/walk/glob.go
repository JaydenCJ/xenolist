// Glob matching for --include/--exclude: `*` and `?` within a path
// segment, `**` across segments. Patterns without a slash match against
// the base name, so `--exclude '*.md'` works anywhere in the tree.
package walk

import "strings"

// Match reports whether the slash-separated relative path matches pattern.
func Match(pattern, rel string) bool {
	if !strings.Contains(pattern, "/") {
		base := rel
		if i := strings.LastIndex(rel, "/"); i >= 0 {
			base = rel[i+1:]
		}
		return matchSegments(strings.Split(pattern, "/"), []string{base})
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(rel, "/"))
}

func matchSegments(pat, segs []string) bool {
	if len(pat) == 0 {
		return len(segs) == 0
	}
	if pat[0] == "**" {
		// `**` spans zero or more whole segments.
		for skip := 0; skip <= len(segs); skip++ {
			if matchSegments(pat[1:], segs[skip:]) {
				return true
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	return matchSegment(pat[0], segs[0]) && matchSegments(pat[1:], segs[1:])
}

// matchSegment matches one path segment with `*` and `?` wildcards.
func matchSegment(pat, s string) bool {
	// Iterative wildcard match with backtracking on the last `*`.
	pi, si := 0, 0
	star, mark := -1, 0
	for si < len(s) {
		switch {
		case pi < len(pat) && (pat[pi] == '?' || pat[pi] == s[si]):
			pi++
			si++
		case pi < len(pat) && pat[pi] == '*':
			star, mark = pi, si
			pi++
		case star >= 0:
			mark++
			pi, si = star+1, mark
		default:
			return false
		}
	}
	for pi < len(pat) && pat[pi] == '*' {
		pi++
	}
	return pi == len(pat)
}
