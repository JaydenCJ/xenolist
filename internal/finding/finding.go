// Package finding defines the census data model — what an external code
// source is, how tightly it is pinned, and where it lives — plus the pure
// reference parsers shared by every scanner (container image references,
// workflow `uses:` values, URLs, and version strings).
package finding

import (
	"regexp"
	"strings"
)

// Kind classifies how the external code reaches the repository's execution
// path. The string values are part of the JSON schema (schema_version 1).
type Kind string

const (
	// KindAction is a GitHub Action or reusable workflow (`uses:`).
	KindAction Kind = "github-action"
	// KindImage is a container image: Dockerfile FROM, workflow
	// container/services, compose services, `uses: docker://…`.
	KindImage Kind = "container-image"
	// KindPipe is an installer piped straight into an interpreter:
	// curl|bash, wget|sh, bash <(curl …), eval "$(curl …)".
	KindPipe Kind = "pipe-to-shell"
	// KindExec is a package runner fetching and executing on the spot:
	// npx, pnpm/yarn dlx, bunx, uvx, pipx run, go run mod@ver, deno run URL.
	KindExec Kind = "package-exec"
	// KindDownload is remote code pulled into the build artifact:
	// Dockerfile ADD <url>, pip install <url|git+…>.
	KindDownload Kind = "remote-download"
)

// AllKinds lists every kind in its canonical report order.
var AllKinds = []Kind{KindAction, KindImage, KindExec, KindPipe, KindDownload}

// Pin grades how reproducible a reference is.
type Pin string

const (
	// PinPinned is immutable: a full commit SHA, an image digest, or a Go
	// pseudo-version — the bytes cannot change under you.
	PinPinned Pin = "pinned"
	// PinTag is versioned but mutable: v4, 1.2.3, node:20 — the publisher
	// can move it.
	PinTag Pin = "tag"
	// PinFloating tracks a moving target: a branch, :latest, an
	// unversioned package, or a bare URL.
	PinFloating Pin = "floating"
)

// Finding is one occurrence of external code in one file.
type Finding struct {
	File    string // repo-relative, slash-separated
	Line    int    // 1-based
	Kind    Kind
	Ref     string // canonical reference, e.g. actions/checkout@v4, node:20-alpine
	Host    string // where the code is fetched from, e.g. github.com, docker.io
	Pin     Pin
	Detail  string // the mechanism, e.g. "uses:", "FROM", "curl | bash", "npx"
	Snippet string // trimmed source evidence
}

// SourceKey identifies the deduplicated source this occurrence belongs to.
func (f Finding) SourceKey() string { return string(f.Kind) + "\x00" + f.Ref }

var (
	hex40Re = regexp.MustCompile(`^[0-9a-f]{40}$`)
	// Go pseudo-versions embed a timestamp and a 12-hex commit prefix and
	// therefore resolve to exactly one commit.
	pseudoRe = regexp.MustCompile(`^v\d+\.\d+\.\d+-(?:(?:alpha|beta|rc)?[.\d]*-)?\d{14}-[0-9a-f]{12}$`)
	// Version-looking tags: v4, 1.2.3, 20.04, 3.19-alpine, 1.2.3+build.
	tagRe = regexp.MustCompile(`^v?\d+(\.\d+)*([.+-].+)?$`)
)

// ClassifyRef grades a bare version/ref string (an action ref, image tag,
// or package version) into a Pin level.
func ClassifyRef(ref string) Pin {
	switch {
	case ref == "", ref == "latest", ref == "main", ref == "master",
		ref == "HEAD", ref == "stable", ref == "edge", ref == "nightly":
		return PinFloating
	case hex40Re.MatchString(ref):
		return PinPinned
	case pseudoRe.MatchString(ref):
		return PinPinned
	case tagRe.MatchString(ref):
		return PinTag
	default:
		return PinFloating
	}
}

// ParseImageRef parses a container image reference of the form
// [registry/]repo[:tag][@digest]. ok is false for references xenolist
// cannot audit: empty strings, `scratch`, and values built from variables.
func ParseImageRef(s string) (ref, host string, pin Pin, ok bool) {
	s = strings.TrimSpace(strings.Trim(strings.TrimSpace(s), `"'`))
	if s == "" || strings.EqualFold(s, "scratch") || strings.ContainsAny(s, "$ \t") {
		return "", "", "", false
	}
	rest := s
	digest := ""
	if i := strings.Index(rest, "@"); i >= 0 {
		digest = rest[i+1:]
		rest = rest[:i]
	}
	tag := ""
	if i := strings.LastIndex(rest, ":"); i >= 0 && !strings.Contains(rest[i+1:], "/") {
		tag = rest[i+1:]
		rest = rest[:i]
	}
	host = "docker.io"
	if i := strings.Index(rest, "/"); i > 0 {
		first := rest[:i]
		if strings.ContainsAny(first, ".:") || first == "localhost" {
			host = first
		}
	}
	switch {
	case strings.HasPrefix(digest, "sha256:"):
		pin = PinPinned
	case tag == "":
		pin = PinFloating
	default:
		pin = ClassifyRef(tag)
	}
	return s, host, pin, true
}

// ParseUses interprets a workflow/action `uses:` value. Local composite
// actions ("./…") are the repository's own code and return ok=false;
// `docker://…` values are classified as container images.
func ParseUses(v string) (Finding, bool) {
	v = strings.TrimSpace(v)
	if v == "" || strings.HasPrefix(v, "./") || strings.HasPrefix(v, ".\\") {
		return Finding{}, false
	}
	if img, found := strings.CutPrefix(v, "docker://"); found {
		ref, host, pin, ok := ParseImageRef(img)
		if !ok {
			return Finding{}, false
		}
		return Finding{Kind: KindImage, Ref: ref, Host: host, Pin: pin, Detail: "uses docker://"}, true
	}
	spec, refPart := v, ""
	if i := strings.LastIndex(v, "@"); i >= 0 {
		spec, refPart = v[:i], v[i+1:]
	}
	if !strings.Contains(spec, "/") || strings.ContainsAny(v, "$ \t") {
		return Finding{}, false
	}
	detail := "uses"
	if strings.Contains(spec, "/.github/workflows/") {
		detail = "reusable workflow"
	}
	return Finding{Kind: KindAction, Ref: v, Host: "github.com", Pin: ClassifyRef(refPart), Detail: detail}, true
}

// HostOfURL extracts the bare hostname (no scheme, userinfo, port, or
// path) from an http(s) or git+http(s) URL. Empty when nothing host-like
// can be found.
func HostOfURL(raw string) string {
	s := raw
	for _, p := range []string{"git+", "http://", "https://", "ftp://"} {
		s = strings.TrimPrefix(s, p)
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.Index(s, ":"); i >= 0 {
		s = s[:i]
	}
	return s
}

// ClassifyURL grades a URL: a 40-hex path segment (e.g. a raw.githubusercontent
// commit path) is pinned, a version-looking segment is a tag, anything else
// floats.
func ClassifyURL(raw string) Pin {
	s := raw
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	best := PinFloating
	for _, seg := range strings.Split(s, "/") {
		if hex40Re.MatchString(seg) {
			return PinPinned
		}
		if tagRe.MatchString(seg) {
			best = PinTag
		}
	}
	return best
}
