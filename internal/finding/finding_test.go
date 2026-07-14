// Tests for the reference parsers: version grading, image references,
// `uses:` values, and URL host/pin extraction. These are the rules every
// scanner leans on, so edge cases here matter more than anywhere else.
package finding

import "testing"

func TestClassifyRefGrading(t *testing.T) {
	cases := []struct {
		ref  string
		want Pin
	}{
		// Full SHAs and Go pseudo-versions resolve to exactly one commit.
		{"8f4b7f84864484a7bf31766abe9204da3cbe65b3", PinPinned},
		{"v0.0.0-20240116215550-a9fa1716bcac", PinPinned},
		// Version-looking tags are versioned but movable.
		{"v4", PinTag},
		{"v4.1.2", PinTag},
		{"1.2.3", PinTag},
		{"20.04", PinTag},
		{"3.19-alpine", PinTag},
		{"1.0.0+build.7", PinTag},
		// Branches and empty refs track a moving target.
		{"", PinFloating},
		{"main", PinFloating},
		{"master", PinFloating},
		{"latest", PinFloating},
		{"HEAD", PinFloating},
		{"develop", PinFloating},
		// A 7-char SHA is not an immutable pin on GitHub Actions (branch
		// names can shadow it), so xenolist refuses to call it pinned.
		{"abcdef1", PinFloating},
	}
	for _, c := range cases {
		if got := ClassifyRef(c.ref); got != c.want {
			t.Fatalf("ClassifyRef(%q) = %s, want %s", c.ref, got, c.want)
		}
	}
}

func TestParseImageRefForms(t *testing.T) {
	cases := []struct {
		in       string
		wantHost string
		wantPin  Pin
	}{
		{"alpine", "docker.io", PinFloating},
		{"node:20-alpine", "docker.io", PinTag},
		{"ubuntu:latest", "docker.io", PinFloating},
		{"redis:7@sha256:1b503bb77079ba644371969e06e1a6a1670bb34c2251107c0fc3a21ef9fdaeca", "docker.io", PinPinned},
		{"ghcr.io/example/tool:v1", "ghcr.io", PinTag},
		// A port in the first component marks it as a registry, and the
		// tag split must not be confused by the extra colon.
		{"localhost:5000/app:1.0", "localhost:5000", PinTag},
		// "library/nginx" has a slash but no dot in the first component —
		// it is still Docker Hub, not a registry called "library".
		{"library/nginx:1.25", "docker.io", PinTag},
	}
	for _, c := range cases {
		ref, host, pin, ok := ParseImageRef(c.in)
		if !ok || ref != c.in || host != c.wantHost || pin != c.wantPin {
			t.Fatalf("ParseImageRef(%q) = %q %q %s %v, want %q %s",
				c.in, ref, host, pin, ok, c.wantHost, c.wantPin)
		}
	}
}

func TestParseImageRefSkipsUnauditable(t *testing.T) {
	// scratch is not external code; ${BASE_IMAGE} cannot be audited
	// statically and a wrong guess is worse than an honest skip.
	for _, in := range []string{"", "scratch", "${BASE_IMAGE}:latest", "$IMG"} {
		if _, _, _, ok := ParseImageRef(in); ok {
			t.Fatalf("ParseImageRef(%q) must be skipped", in)
		}
	}
}

func TestParseUsesActionForms(t *testing.T) {
	cases := []struct {
		in      string
		wantPin Pin
	}{
		{"actions/setup-go@v5", PinTag},
		{"actions/checkout@8f4b7f84864484a7bf31766abe9204da3cbe65b3", PinPinned},
		{"github/codeql-action/analyze@v3", PinTag}, // subdirectory action
		{"someorg/action@main", PinFloating},        // branch ref
	}
	for _, c := range cases {
		f, ok := ParseUses(c.in)
		if !ok || f.Kind != KindAction || f.Host != "github.com" || f.Ref != c.in || f.Pin != c.wantPin {
			t.Fatalf("ParseUses(%q) = %+v %v", c.in, f, ok)
		}
	}
}

func TestParseUsesReusableWorkflow(t *testing.T) {
	f, ok := ParseUses("octo-org/workflows/.github/workflows/release.yml@main")
	if !ok || f.Detail != "reusable workflow" || f.Pin != PinFloating {
		t.Fatalf("got %+v %v", f, ok)
	}
}

func TestParseUsesDockerImage(t *testing.T) {
	f, ok := ParseUses("docker://node:20")
	if !ok || f.Kind != KindImage || f.Host != "docker.io" {
		t.Fatalf("got %+v %v", f, ok)
	}
}

func TestParseUsesLocalActionSkipped(t *testing.T) {
	// Local composite actions are the repository's own code.
	if _, ok := ParseUses("./.github/actions/setup"); ok {
		t.Fatal("local composite actions must not be census entries")
	}
}

func TestHostOfURL(t *testing.T) {
	cases := map[string]string{
		"https://get.example.test/install.sh":     "get.example.test",
		"http://127.0.0.1:8080/x":                 "127.0.0.1",
		"git+https://github.com/example/lib@v1":   "github.com",
		"https://user@internal.example.test/path": "internal.example.test",
	}
	for in, want := range cases {
		if got := HostOfURL(in); got != want {
			t.Fatalf("HostOfURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClassifyURL(t *testing.T) {
	cases := []struct {
		in   string
		want Pin
	}{
		// A commit-SHA path segment (raw file hosting) is immutable.
		{"https://raw.example.test/owner/repo/8f4b7f84864484a7bf31766abe9204da3cbe65b3/install.sh", PinPinned},
		{"https://static.example.test/v1.4.2/tool.sh", PinTag},
		{"https://get.example.test/install.sh", PinFloating},
	}
	for _, c := range cases {
		if got := ClassifyURL(c.in); got != c.want {
			t.Fatalf("ClassifyURL(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}
