// Tests for the shell command scanner — the engine behind every
// "this line runs code from the internet" rule. Each case documents a real
// installer/runner shape; the negative cases pin down what must NOT count,
// because a census nobody trusts is a census nobody reads.
package scan

import (
	"testing"

	"github.com/JaydenCJ/xenolist/internal/finding"
)

func one(t *testing.T, text string) finding.Finding {
	t.Helper()
	fs := ScanShellText("f.sh", text, 1)
	if len(fs) != 1 {
		t.Fatalf("%q: want exactly 1 finding, got %d: %+v", text, len(fs), fs)
	}
	return fs[0]
}

func none(t *testing.T, text string) {
	t.Helper()
	if fs := ScanShellText("f.sh", text, 1); len(fs) != 0 {
		t.Fatalf("%q: want no findings, got %+v", text, fs)
	}
}

func TestPipeToShellVariants(t *testing.T) {
	cases := []struct {
		text       string
		wantRef    string
		wantDetail string
	}{
		{"curl -fsSL https://get.example.test/install.sh | bash",
			"https://get.example.test/install.sh", "curl | bash"},
		{"wget -qO- https://sh.example.test | sh -s -- --yes",
			"https://sh.example.test", "wget | sh"},
		// The classic hardened-looking sudo variant is still remote code
		// execution, and tee in the middle does not launder it.
		{"curl -sSf https://x.example.test/i.sh | sudo bash",
			"https://x.example.test/i.sh", "curl | bash"},
		{"curl https://x.example.test/i.sh | tee /tmp/i.sh | bash",
			"https://x.example.test/i.sh", "curl | bash"},
	}
	for _, c := range cases {
		f := one(t, c.text)
		if f.Kind != finding.KindPipe || f.Ref != c.wantRef || f.Detail != c.wantDetail {
			t.Fatalf("%q: got %+v", c.text, f)
		}
	}
}

func TestFetchWithoutExecutionNotFlagged(t *testing.T) {
	for _, text := range []string{
		"curl -o /tmp/tool.tar.gz https://dl.example.test/tool.tar.gz",
		"curl https://dl.example.test/t.tgz | tar xz", // tar extracts, it does not execute
		// `curl …; bash local.sh` — the fetch and the shell are different statements.
		"curl https://x.example.test/a.txt -o a.txt; bash local.sh",
		`echo "curl https://x.example.test | bash"`, // pipe inside quotes
		"# curl https://x.example.test | bash",      // whole line is a comment
		"echo done # npx cowsay",                    // trailing comment
	} {
		none(t, text)
	}
}

func TestSubstitutionExecution(t *testing.T) {
	for _, text := range []string{
		"bash <(curl -sL https://get.example.test/boot.sh)",
		`eval "$(curl -fsSL https://get.example.test/boot.sh)"`,
		`sh -c "$(wget -qO- https://get.example.test/boot.sh)"`,
	} {
		f := one(t, text)
		if f.Kind != finding.KindPipe || f.Ref != "https://get.example.test/boot.sh" {
			t.Fatalf("%q: got %+v", text, f)
		}
	}
}

func TestSubstitutionWithoutExecutionNotFlagged(t *testing.T) {
	// diff reads the bytes; echo captures them — neither executes them.
	none(t, "diff <(curl -s https://a.example.test/x) <(curl -s https://b.example.test/x)")
	none(t, `echo "$(curl -s https://api.example.test/version)"`)
}

func TestNpxVariants(t *testing.T) {
	cases := []struct {
		text    string
		wantRef string
		wantPin finding.Pin
	}{
		{"npx cowsay hello", "cowsay", finding.PinFloating},
		{"npx --yes prettier@3.3.2 --check .", "prettier@3.3.2", finding.PinTag},
		{"npx @scope/tool@1.0.0 build", "@scope/tool@1.0.0", finding.PinTag},
		{"npx -p typescript tsc --noEmit", "typescript", finding.PinFloating},
	}
	for _, c := range cases {
		f := one(t, c.text)
		if f.Kind != finding.KindExec || f.Host != "registry.npmjs.org" ||
			f.Ref != c.wantRef || f.Pin != c.wantPin {
			t.Fatalf("%q: got %+v", c.text, f)
		}
	}
}

func TestDlxFamily(t *testing.T) {
	cases := map[string]string{
		"pnpm dlx create-vite@5 my-app": "pnpm dlx",
		"yarn dlx cowsay moo":           "yarn dlx",
		"bunx eslint .":                 "bunx",
		"npm exec --yes -- cowsay":      "npm exec",
	}
	for text, wantDetail := range cases {
		if f := one(t, text); f.Detail != wantDetail || f.Kind != finding.KindExec {
			t.Fatalf("%q: got %+v", text, f)
		}
	}
}

func TestLockfileInstallsNotFlagged(t *testing.T) {
	// Regular dependency installs resolve through the lockfile/index —
	// that is dependency management, not ad-hoc remote code.
	none(t, "pnpm install --frozen-lockfile")
	none(t, "pip install -r requirements.txt")
	none(t, "pip install requests==2.32.0")
}

func TestPythonRunners(t *testing.T) {
	f := one(t, "uvx ruff@0.4.4 check .")
	if f.Kind != finding.KindExec || f.Host != "pypi.org" || f.Pin != finding.PinTag {
		t.Fatalf("got %+v", f)
	}
	f = one(t, "pipx run black==24.4.2 --check src")
	if f.Ref != "black==24.4.2" || f.Pin != finding.PinTag {
		t.Fatalf("got %+v", f)
	}
}

func TestGoRunRemoteVsLocal(t *testing.T) {
	f := one(t, "go run golang.org/x/tools/cmd/stringer@latest -type Kind")
	if f.Kind != finding.KindExec || f.Host != "proxy.golang.org" || f.Pin != finding.PinFloating {
		t.Fatalf("got %+v", f)
	}
	// Pseudo-versions resolve to exactly one commit.
	if f := one(t, "go run example.test/cmd/tool@v0.0.0-20240116215550-a9fa1716bcac"); f.Pin != finding.PinPinned {
		t.Fatalf("got %+v", f)
	}
	// Local packages and files never leave the repository.
	none(t, "go run ./cmd/xenolist scan .")
	none(t, "go run main.go")
}

func TestDenoRunRemoteURL(t *testing.T) {
	f := one(t, "deno run --allow-net https://deno.example.test/std/http/server.ts")
	if f.Kind != finding.KindExec || f.Detail != "deno run" {
		t.Fatalf("got %+v", f)
	}
}

func TestPipInstallRemoteCode(t *testing.T) {
	f := one(t, "pip install git+https://github.com/example/lib@v2.1.0")
	if f.Kind != finding.KindDownload || f.Host != "github.com" || f.Pin != finding.PinTag {
		t.Fatalf("got %+v", f)
	}
	if f := one(t, "pip install git+https://github.com/example/lib@0123456789abcdef0123456789abcdef01234567"); f.Pin != finding.PinPinned {
		t.Fatalf("got %+v", f)
	}
	if f := one(t, "python3 -m pip install https://files.example.test/pkg-1.0-py3-none-any.whl"); f.Kind != finding.KindDownload {
		t.Fatalf("got %+v", f)
	}
}

func TestContinuationsAndLineNumbers(t *testing.T) {
	// Backslash continuations join into one command reported at its first
	// physical line, and the base offset shifts every reported line.
	f := one(t, "curl -fsSL \\\n  https://get.example.test/i.sh \\\n  | bash")
	if f.Kind != finding.KindPipe || f.Line != 1 {
		t.Fatalf("got %+v", f)
	}
	fs := ScanShellText("f.sh", "echo one\nnpx cowsay\n", 10)
	if len(fs) != 1 || fs[0].Line != 11 {
		t.Fatalf("got %+v", fs)
	}
}

func TestEnvPrefixAndChains(t *testing.T) {
	if f := one(t, "CI=1 FOO=bar npx cowsay"); f.Ref != "cowsay" {
		t.Fatalf("got %+v", f)
	}
	// Both sides of && are separate commands and both are scanned.
	fs := ScanShellText("f.sh", "npx eslint . && curl https://x.example.test/i.sh | sh", 1)
	if len(fs) != 2 {
		t.Fatalf("got %+v", fs)
	}
}

func TestSnippetIsCompactEvidence(t *testing.T) {
	f := one(t, "curl   -fsSL   https://get.example.test/i.sh   |   bash")
	if f.Snippet != "curl -fsSL https://get.example.test/i.sh | bash" {
		t.Fatalf("got %q", f.Snippet)
	}
}
