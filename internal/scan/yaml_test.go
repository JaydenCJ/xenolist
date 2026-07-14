// Tests for the generic CI-YAML pass across its real dialects: GitHub
// workflows, composite actions, docker-compose, and GitLab CI.
package scan

import (
	"testing"

	"github.com/JaydenCJ/xenolist/internal/finding"
)

const workflowSrc = `name: CI
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    container: node:20
    services:
      db:
        image: postgres:16.3
    steps:
      - uses: actions/checkout@8f4b7f84864484a7bf31766abe9204da3cbe65b3
      - uses: ./.github/actions/local
      - name: tools
        run: |
          curl -fsSL https://get.example.test/i.sh | bash
          npx cowsay hi
  release:
    uses: octo-org/w/.github/workflows/r.yml@v1
`

func TestWorkflowFullCensus(t *testing.T) {
	fs := ScanYAML("ci.yml", workflowSrc)
	if len(fs) != 6 {
		t.Fatalf("want 6 findings, got %d: %+v", len(fs), fs)
	}
	// Both image surfaces are covered: `container:` shorthand and the
	// service's `image:` key.
	images := 0
	for _, f := range fs {
		if f.Kind == finding.KindImage {
			images++
		}
	}
	if images != 2 {
		t.Fatalf("want container + service image, got %d", images)
	}
}

func TestWorkflowUsesPinAndLocalSkip(t *testing.T) {
	fs := ScanYAML("ci.yml", workflowSrc)
	var checkout *finding.Finding
	for i := range fs {
		if fs[i].Line == 12 {
			t.Fatalf("local action counted: %+v", fs[i])
		}
		if fs[i].Kind == finding.KindAction && fs[i].Pin == finding.PinPinned {
			checkout = &fs[i]
		}
	}
	if checkout == nil || checkout.Line != 11 {
		t.Fatalf("got %+v", checkout)
	}
}

func TestWorkflowRunBlockLineNumbers(t *testing.T) {
	fs := ScanYAML("ci.yml", workflowSrc)
	var pipe, npx *finding.Finding
	for i := range fs {
		switch fs[i].Kind {
		case finding.KindPipe:
			pipe = &fs[i]
		case finding.KindExec:
			npx = &fs[i]
		}
	}
	if pipe == nil || pipe.Line != 15 {
		t.Fatalf("pipe line: %+v", pipe)
	}
	if npx == nil || npx.Line != 16 {
		t.Fatalf("npx line: %+v", npx)
	}
}

func TestWorkflowReusableWorkflowRef(t *testing.T) {
	fs := ScanYAML("ci.yml", workflowSrc)
	found := false
	for _, f := range fs {
		if f.Detail == "reusable workflow" && f.Line == 18 {
			found = true
		}
	}
	if !found {
		t.Fatalf("reusable workflow missing: %+v", fs)
	}
}

func TestComposeImages(t *testing.T) {
	src := "services:\n  cache:\n    image: redis:7.2.5\n  web:\n    build: .\n"
	fs := ScanYAML("docker-compose.yml", src)
	if len(fs) != 1 || fs[0].Ref != "redis:7.2.5" || fs[0].Line != 3 {
		t.Fatalf("got %+v", fs)
	}
}

func TestGitLabScriptLists(t *testing.T) {
	src := "lint:\n  image: node:20\n  script:\n    - npm ci\n    - npx eslint .\n    - curl https://get.example.test/i.sh | sh\n  before_script:\n    - npx cowsay\nother:\n  variables:\n    X: npx not-a-command\n"
	fs := ScanYAML(".gitlab-ci.yml", src)
	// image + 2 script hits + 1 before_script hit; the next job's
	// variables must NOT be attributed to any script key.
	if len(fs) != 4 {
		t.Fatalf("got %+v", fs)
	}
	if fs[1].Kind != finding.KindExec || fs[1].Line != 5 {
		t.Fatalf("got %+v", fs[1])
	}
	if fs[2].Kind != finding.KindPipe || fs[2].Line != 6 {
		t.Fatalf("got %+v", fs[2])
	}
}

func TestCompositeActionUsesAndRun(t *testing.T) {
	src := "runs:\n  using: composite\n  steps:\n    - uses: actions/cache@v4\n    - run: npx cowsay\n      shell: bash\n"
	fs := ScanYAML("action.yml", src)
	if len(fs) != 2 || fs[0].Kind != finding.KindAction || fs[1].Kind != finding.KindExec {
		t.Fatalf("got %+v", fs)
	}
}

func TestUsesDockerImageAndInlineRun(t *testing.T) {
	src := "steps:\n  - uses: docker://ghcr.io/example/lint:v1\n  - run: npx cowsay hi\n"
	fs := ScanYAML("ci.yml", src)
	if len(fs) != 2 || fs[0].Kind != finding.KindImage || fs[0].Host != "ghcr.io" {
		t.Fatalf("got %+v", fs)
	}
	if fs[1].Line != 3 || fs[1].Kind != finding.KindExec {
		t.Fatalf("got %+v", fs[1])
	}
	if fs[0].Snippet != "- uses: docker://ghcr.io/example/lint:v1" {
		t.Fatalf("snippet: %q", fs[0].Snippet)
	}
}
