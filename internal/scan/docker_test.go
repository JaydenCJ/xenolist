// Tests for Dockerfile scanning: base images, stage-alias suppression,
// remote ADDs, and RUN bodies (plain, JSON-array, heredoc) reaching the
// shared shell rules with correct line numbers.
package scan

import (
	"testing"

	"github.com/JaydenCJ/xenolist/internal/finding"
)

func TestDockerFromVariants(t *testing.T) {
	cases := []struct {
		src     string
		wantRef string
		wantPin finding.Pin
	}{
		{"FROM golang:1.22-alpine\n", "golang:1.22-alpine", finding.PinTag},
		{"FROM alpine:3.19@sha256:1b503bb77079ba644371969e06e1a6a1670bb34c2251107c0fc3a21ef9fdaeca\n",
			"alpine:3.19@sha256:1b503bb77079ba644371969e06e1a6a1670bb34c2251107c0fc3a21ef9fdaeca", finding.PinPinned},
		{"FROM --platform=linux/amd64 node:20\n", "node:20", finding.PinTag},
		{"from alpine:3.19\n", "alpine:3.19", finding.PinTag}, // instructions are case-insensitive
	}
	for _, c := range cases {
		fs := ScanDockerfile("Dockerfile", c.src)
		if len(fs) != 1 || fs[0].Kind != finding.KindImage || fs[0].Detail != "FROM" ||
			fs[0].Ref != c.wantRef || fs[0].Pin != c.wantPin || fs[0].Line != 1 {
			t.Fatalf("%q: got %+v", c.src, fs)
		}
	}
}

func TestDockerStagesAreNotExternal(t *testing.T) {
	// Stage aliases (AS build) and stage indexes are the repo's own
	// intermediate images; only the ghcr helper below is external.
	src := "FROM golang:1.22 AS build\nFROM alpine:3.19\nCOPY --from=build /app /app\nCOPY --from=ghcr.io/example/helper:v2 /h /h\nCOPY --from=0 /x /x\nFROM build\n"
	fs := ScanDockerfile("Dockerfile", src)
	if len(fs) != 3 {
		t.Fatalf("got %+v", fs)
	}
	if fs[2].Detail != "COPY --from" || fs[2].Host != "ghcr.io" || fs[2].Line != 4 {
		t.Fatalf("got %+v", fs[2])
	}
}

func TestDockerFromArgVariableSkipped(t *testing.T) {
	// ARG-driven bases cannot be audited statically; a wrong guess is
	// worse than an honest skip.
	fs := ScanDockerfile("Dockerfile", "ARG BASE=alpine\nFROM ${BASE}:latest\n")
	if len(fs) != 0 {
		t.Fatalf("got %+v", fs)
	}
}

func TestDockerAddRemoteVsLocal(t *testing.T) {
	fs := ScanDockerfile("Dockerfile", "FROM alpine:3.19\nADD https://example.test/tools/jq /usr/bin/jq\nADD rootfs.tar /\n")
	if len(fs) != 2 || fs[1].Kind != finding.KindDownload || fs[1].Detail != "ADD" || fs[1].Line != 2 {
		t.Fatalf("got %+v", fs)
	}
}

func TestDockerRunPipeToShell(t *testing.T) {
	src := "FROM alpine:3.19\nRUN curl -fsSL https://get.example.test/i.sh | sh\n"
	fs := ScanDockerfile("Dockerfile", src)
	if len(fs) != 2 || fs[1].Kind != finding.KindPipe || fs[1].Line != 2 {
		t.Fatalf("got %+v", fs)
	}
}

func TestDockerRunContinuationKeepsFirstLine(t *testing.T) {
	src := "FROM alpine:3.19\nRUN apk add curl && \\\n    curl https://get.example.test/i.sh | sh\n"
	fs := ScanDockerfile("Dockerfile", src)
	if len(fs) != 2 || fs[1].Line != 2 {
		t.Fatalf("got %+v", fs)
	}
}

func TestDockerRunJSONFormAndMountFlag(t *testing.T) {
	// Exec-form RUN unwraps the sh -c script; BuildKit --mount flags are
	// stripped before the shell rules see the command.
	src := "FROM alpine:3.19\nRUN [\"sh\", \"-c\", \"curl https://get.example.test/i.sh | sh\"]\n"
	fs := ScanDockerfile("Dockerfile", src)
	if len(fs) != 2 || fs[1].Kind != finding.KindPipe {
		t.Fatalf("got %+v", fs)
	}
	src = "FROM alpine:3.19\nRUN --mount=type=cache,target=/root/.cache npx cowsay\n"
	fs = ScanDockerfile("Dockerfile", src)
	if len(fs) != 2 || fs[1].Kind != finding.KindExec {
		t.Fatalf("got %+v", fs)
	}
}

func TestDockerRunHeredocLineNumbers(t *testing.T) {
	src := "FROM alpine:3.19\nRUN <<EOF\necho start\ncurl https://get.example.test/i.sh | sh\nEOF\nADD https://example.test/x /x\n"
	fs := ScanDockerfile("Dockerfile", src)
	if len(fs) != 3 {
		t.Fatalf("got %+v", fs)
	}
	if fs[1].Kind != finding.KindPipe || fs[1].Line != 4 {
		t.Fatalf("heredoc line wrong: %+v", fs[1])
	}
	if fs[2].Line != 6 {
		t.Fatalf("post-heredoc instruction line wrong: %+v", fs[2])
	}
}

func TestDockerCommentsIgnored(t *testing.T) {
	src := "# FROM evil:latest\nFROM alpine:3.19\n"
	fs := ScanDockerfile("Dockerfile", src)
	if len(fs) != 1 || fs[0].Ref != "alpine:3.19" {
		t.Fatalf("got %+v", fs)
	}
}
