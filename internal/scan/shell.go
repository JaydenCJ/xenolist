// Shell command scanning: the single engine behind every "this line runs
// code from the internet" detection. It is fed by shell scripts, Makefile
// recipes, Dockerfile RUN instructions, workflow `run:` blocks, GitLab
// `script:` lists, and package.json scripts — which is exactly why the
// census is cross-file: one set of rules, every surface.
package scan

import (
	"strings"

	"github.com/JaydenCJ/xenolist/internal/finding"
)

// interpreters are commands that execute whatever is piped into them.
var interpreters = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true,
	"ash": true, "fish": true, "python": true, "python2": true,
	"python3": true, "node": true, "perl": true, "ruby": true,
}

// fetchers are commands that pull bytes off the network.
var fetchers = map[string]bool{"curl": true, "wget": true, "fetch": true}

// ScanShellText scans a block of shell commands starting at baseLine
// (1-based) of file. Backslash continuations are joined; comments are
// stripped quote-aware; pipelines are split respecting quotes and
// substitution depth.
func ScanShellText(file, text string, baseLine int) []finding.Finding {
	var out []finding.Finding
	for _, ll := range logicalLines(text, baseLine) {
		out = append(out, scanLogicalLine(file, ll)...)
	}
	return out
}

// logicalLine is one command line after joining continuations.
type logicalLine struct {
	line int // 1-based line of the first physical line
	text string
}

func logicalLines(text string, baseLine int) []logicalLine {
	physical := strings.Split(text, "\n")
	var out []logicalLine
	for i := 0; i < len(physical); i++ {
		start := baseLine + i
		buf := stripShellComment(physical[i])
		for strings.HasSuffix(buf, "\\") && !strings.HasSuffix(buf, "\\\\") && i+1 < len(physical) {
			i++
			buf = strings.TrimSuffix(buf, "\\") + " " + stripShellComment(physical[i])
		}
		if strings.TrimSpace(buf) == "" {
			continue
		}
		out = append(out, logicalLine{line: start, text: buf})
	}
	return out
}

// stripShellComment removes a trailing comment, honoring quotes; "#" only
// starts a comment at line start or after whitespace ("$#"/"${#x}" survive).
func stripShellComment(s string) string {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\\':
			i++
		case c == '\'' || c == '"':
			quote = c
		case c == '#':
			if i == 0 || s[i-1] == ' ' || s[i-1] == '\t' {
				return s[:i]
			}
		}
	}
	return s
}

func scanLogicalLine(file string, ll logicalLine) []finding.Finding {
	var out []finding.Finding
	snippet := snippetOf(ll.text)
	add := func(f finding.Finding, ok bool) {
		if !ok {
			return
		}
		f.File, f.Line, f.Snippet = file, ll.line, snippet
		out = append(out, f)
	}

	segs := splitSegments(ll.text)

	// Pass 1: fetch-substitution feeding an interpreter —
	// bash <(curl …), eval "$(curl …)", sh -c "$(wget …)", source <(curl …).
	for _, seg := range segs {
		cmd, args := commandOf(seg.tokens)
		evaluating := cmd == "eval" || cmd == "source" || cmd == "." ||
			(interpreters[cmd] && hasFlag(args, "-c"))
		for _, open := range []string{"<(", "$("} {
			// `bash <(curl …)` executes the stream; `diff <(curl …)` does
			// not — process substitution only counts under an interpreter.
			// `$(curl …)` only counts when the result is evaluated.
			if open == "<(" && !evaluating && !interpreters[cmd] {
				continue
			}
			if open == "$(" && !evaluating {
				continue
			}
			idx := 0
			for {
				pos := strings.Index(seg.raw[idx:], open)
				if pos < 0 {
					break
				}
				pos += idx
				inner := balancedInner(seg.raw[pos+len(open):])
				idx = pos + len(open)
				icmd, iargs := commandOf(splitTokens(inner))
				if !fetchers[icmd] {
					continue
				}
				url := firstURL(iargs)
				if url == "" {
					continue
				}
				detail := icmd + " substitution"
				add(finding.Finding{
					Kind: finding.KindPipe, Ref: url,
					Host: finding.HostOfURL(url), Pin: finding.ClassifyURL(url),
					Detail: detail,
				}, true)
			}
		}
	}

	// Pass 2: pipelines (curl … | bash) and per-segment package runners.
	fetchURL, fetchCmd := "", ""
	for _, seg := range segs {
		if seg.sep != "|" && seg.sep != "|&" {
			fetchURL, fetchCmd = "", ""
		}
		cmd, args := commandOf(seg.tokens)
		if cmd == "" {
			continue
		}
		if fetchers[cmd] {
			if u := firstURL(args); u != "" {
				fetchURL, fetchCmd = u, cmd
			}
			continue
		}
		if interpreters[cmd] && fetchURL != "" {
			add(finding.Finding{
				Kind: finding.KindPipe, Ref: fetchURL,
				Host: finding.HostOfURL(fetchURL), Pin: finding.ClassifyURL(fetchURL),
				Detail: fetchCmd + " | " + cmd,
			}, true)
			fetchURL, fetchCmd = "", ""
			continue
		}
		add(scanRunner(cmd, args))
	}
	return out
}

// scanRunner detects package runners and remote installs in one command.
func scanRunner(cmd string, args []string) (finding.Finding, bool) {
	switch cmd {
	case "npx":
		return npmExecFinding("npx", npxPackage(args))
	case "bunx":
		return npmExecFinding("bunx", firstNonFlag(args))
	case "pnpm":
		if len(args) > 0 && args[0] == "dlx" {
			return npmExecFinding("pnpm dlx", firstNonFlag(args[1:]))
		}
	case "yarn":
		if len(args) > 0 && args[0] == "dlx" {
			return npmExecFinding("yarn dlx", firstNonFlag(args[1:]))
		}
	case "npm":
		if len(args) > 0 && (args[0] == "exec" || args[0] == "x") {
			return npmExecFinding("npm exec", firstNonFlag(args[1:]))
		}
	case "uvx":
		return pypiExecFinding("uvx", firstNonFlag(args))
	case "pipx":
		if len(args) > 0 && args[0] == "run" {
			return pypiExecFinding("pipx run", firstNonFlag(args[1:]))
		}
	case "go":
		if len(args) > 0 && args[0] == "run" {
			return goRunFinding(firstNonFlag(args[1:]))
		}
	case "deno":
		if len(args) > 0 && args[0] == "run" {
			if u := firstURL(args[1:]); u != "" {
				return finding.Finding{
					Kind: finding.KindExec, Ref: u,
					Host: finding.HostOfURL(u), Pin: finding.ClassifyURL(u),
					Detail: "deno run",
				}, true
			}
		}
	case "pip", "pip3", "python", "python3":
		rest := args
		if cmd == "python" || cmd == "python3" {
			if len(rest) < 3 || rest[0] != "-m" || rest[1] != "pip" {
				return finding.Finding{}, false
			}
			rest = rest[2:]
		}
		if len(rest) > 0 && rest[0] == "install" {
			return pipInstallFinding(rest[1:])
		}
	}
	return finding.Finding{}, false
}

// npxPackage extracts the package from npx args; -p/--package name the
// package explicitly, other flags are skipped.
func npxPackage(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-p" || a == "--package":
			if i+1 < len(args) {
				return args[i+1]
			}
			return ""
		case strings.HasPrefix(a, "--package="):
			return strings.TrimPrefix(a, "--package=")
		case a == "--":
			return firstNonFlag(args[i+1:])
		case strings.HasPrefix(a, "-"):
			continue
		default:
			return a
		}
	}
	return ""
}

func firstNonFlag(args []string) string {
	for _, a := range args {
		if a == "--" {
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		return a
	}
	return ""
}

func npmExecFinding(detail, pkg string) (finding.Finding, bool) {
	if pkg == "" || strings.HasPrefix(pkg, ".") || strings.HasPrefix(pkg, "/") ||
		strings.Contains(pkg, "$") {
		return finding.Finding{}, false
	}
	version := ""
	if i := strings.LastIndex(pkg, "@"); i > 0 {
		version = pkg[i+1:]
	}
	pin := finding.PinFloating
	if version != "" {
		pin = finding.ClassifyRef(version)
	}
	return finding.Finding{
		Kind: finding.KindExec, Ref: pkg, Host: "registry.npmjs.org",
		Pin: pin, Detail: detail,
	}, true
}

func pypiExecFinding(detail, pkg string) (finding.Finding, bool) {
	if pkg == "" || strings.HasPrefix(pkg, ".") || strings.HasPrefix(pkg, "/") ||
		strings.Contains(pkg, "$") {
		return finding.Finding{}, false
	}
	version := ""
	if i := strings.Index(pkg, "=="); i > 0 {
		version = pkg[i+2:]
	} else if i := strings.LastIndex(pkg, "@"); i > 0 {
		version = pkg[i+1:]
	}
	pin := finding.PinFloating
	if version != "" {
		pin = finding.ClassifyRef(version)
	}
	return finding.Finding{
		Kind: finding.KindExec, Ref: pkg, Host: "pypi.org",
		Pin: pin, Detail: detail,
	}, true
}

// goRunFinding flags `go run module@version` only for remote modules —
// the first path element must look like a domain, so `go run ./cmd/x`
// and `go run main.go` stay local.
func goRunFinding(arg string) (finding.Finding, bool) {
	if arg == "" || strings.HasPrefix(arg, ".") || strings.HasPrefix(arg, "/") ||
		strings.Contains(arg, "$") || strings.HasSuffix(arg, ".go") {
		return finding.Finding{}, false
	}
	modPath := arg
	version := ""
	if i := strings.LastIndex(arg, "@"); i > 0 {
		modPath, version = arg[:i], arg[i+1:]
	}
	first, _, _ := strings.Cut(modPath, "/")
	if !strings.Contains(first, ".") {
		return finding.Finding{}, false
	}
	pin := finding.PinFloating
	if version != "" {
		pin = finding.ClassifyRef(version)
	}
	return finding.Finding{
		Kind: finding.KindExec, Ref: arg, Host: "proxy.golang.org",
		Pin: pin, Detail: "go run",
	}, true
}

// pipInstallFinding flags pip installs that bypass the index: bare URLs
// and git+ requirement specs.
func pipInstallFinding(args []string) (finding.Finding, bool) {
	for _, a := range args {
		if !strings.HasPrefix(a, "git+") && !strings.HasPrefix(a, "http://") &&
			!strings.HasPrefix(a, "https://") {
			continue
		}
		pin := finding.ClassifyURL(a)
		if strings.HasPrefix(a, "git+") {
			pin = finding.PinFloating
			if i := strings.LastIndex(a, "@"); i > strings.Index(a, "://") {
				pin = finding.ClassifyRef(a[i+1:])
			}
		}
		return finding.Finding{
			Kind: finding.KindDownload, Ref: a,
			Host: finding.HostOfURL(a), Pin: pin, Detail: "pip install",
		}, true
	}
	return finding.Finding{}, false
}

// --- tokenizing helpers -------------------------------------------------

type segment struct {
	sep    string // separator before this segment: "", "|", "|&", "&&", "||", ";", "&"
	raw    string
	tokens []string
}

// splitSegments splits a logical line at pipeline/list operators, outside
// quotes and outside $(…)/<(…)/(…) nesting.
func splitSegments(s string) []segment {
	var segs []segment
	var quote byte
	depth := 0
	start := 0
	sep := ""
	emit := func(end int, nextSep string, skip int) {
		raw := strings.TrimSpace(s[start:end])
		if raw != "" {
			segs = append(segs, segment{sep: sep, raw: raw, tokens: splitTokens(raw)})
		}
		sep = nextSep
		start = end + skip
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '\\':
			i++
		case c == '\'' || c == '"':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			if depth > 0 {
				depth--
			}
		case depth > 0:
			// operators inside substitutions belong to the inner command
		case c == '|':
			switch {
			case i+1 < len(s) && s[i+1] == '|':
				emit(i, "||", 2)
				i++
			case i+1 < len(s) && s[i+1] == '&':
				emit(i, "|&", 2)
				i++
			default:
				emit(i, "|", 1)
			}
		case c == '&':
			if i+1 < len(s) && s[i+1] == '&' {
				emit(i, "&&", 2)
				i++
			} else {
				emit(i, "&", 1)
			}
		case c == ';':
			emit(i, ";", 1)
		}
	}
	emit(len(s), "", 0)
	return segs
}

// splitTokens splits a command into whitespace-separated tokens, keeping
// quoted and $(…)-nested spans intact (quotes are stripped from fully
// quoted tokens).
func splitTokens(s string) []string {
	var tokens []string
	var cur strings.Builder
	var quote byte
	depth := 0
	quoted := false
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		t := cur.String()
		if !quoted {
			t = strings.TrimSpace(t)
		}
		if t != "" {
			tokens = append(tokens, t)
		}
		cur.Reset()
		quoted = false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' && i+1 < len(s) {
				cur.WriteByte(s[i+1])
				i++
				continue
			}
			if c == quote {
				quote = 0
				continue
			}
			cur.WriteByte(c)
		case c == '\'' || c == '"':
			quote = c
			quoted = true
		case c == '\\':
			if i+1 < len(s) {
				cur.WriteByte(s[i+1])
				i++
			}
		case c == '(':
			depth++
			cur.WriteByte(c)
		case c == ')':
			if depth > 0 {
				depth--
			}
			cur.WriteByte(c)
		case (c == ' ' || c == '\t') && depth == 0:
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return tokens
}

// commandOf strips env assignments and wrapper commands (sudo, env, exec,
// command, time, nohup) and returns the effective command basename plus
// its arguments.
func commandOf(tokens []string) (string, []string) {
	i := 0
	for i < len(tokens) {
		t := tokens[i]
		switch {
		case isEnvAssign(t):
			i++
		case t == "sudo" || t == "env" || t == "nohup":
			i++
			for i < len(tokens) && (strings.HasPrefix(tokens[i], "-") || isEnvAssign(tokens[i])) {
				i++
			}
		case t == "command" || t == "exec" || t == "time" || t == "builtin":
			i++
		default:
			cmd := t
			if j := strings.LastIndex(cmd, "/"); j >= 0 && !strings.Contains(cmd, "://") {
				cmd = cmd[j+1:]
			}
			return cmd, tokens[i+1:]
		}
	}
	return "", nil
}

func isEnvAssign(t string) bool {
	i := strings.Index(t, "=")
	if i <= 0 {
		return false
	}
	for _, c := range t[:i] {
		if c != '_' && !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(c >= '0' && c <= '9') {
			return false
		}
	}
	return t[0] == '_' || (t[0] >= 'a' && t[0] <= 'z') || (t[0] >= 'A' && t[0] <= 'Z')
}

func firstURL(args []string) string {
	for _, a := range args {
		a = strings.Trim(a, `"'`)
		if strings.HasPrefix(a, "http://") || strings.HasPrefix(a, "https://") ||
			strings.HasPrefix(a, "ftp://") {
			return a
		}
	}
	return ""
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// balancedInner returns the content of s up to the parenthesis matching an
// already-consumed opening one.
func balancedInner(s string) string {
	depth := 1
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
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return s[:i]
			}
		}
	}
	return s
}

// snippetOf produces compact one-line evidence, capped for report width.
func snippetOf(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 160 {
		s = s[:157] + "..."
	}
	return s
}
