// Tests for package.json script scanning: detection, line recovery, and
// graceful handling of files that are not what they claim to be.
package scan

import (
	"testing"

	"github.com/JaydenCJ/xenolist/internal/finding"
)

const pkgSrc = `{
  "name": "demo",
  "version": "1.0.0",
  "scripts": {
    "docs": "npx typedoc@0.25.13",
    "fmt": "prettier -w .",
    "setup": "curl https://get.example.test/i.sh | sh"
  }
}
`

func TestPackageJSONFindsRemoteExec(t *testing.T) {
	fs := ScanPackageJSON("package.json", pkgSrc)
	if len(fs) != 2 {
		t.Fatalf("got %+v", fs)
	}
}

func TestPackageJSONLineRecovery(t *testing.T) {
	fs := ScanPackageJSON("package.json", pkgSrc)
	if fs[0].Line != 5 || fs[0].Kind != finding.KindExec {
		t.Fatalf("got %+v", fs[0])
	}
	if fs[1].Line != 7 || fs[1].Kind != finding.KindPipe {
		t.Fatalf("got %+v", fs[1])
	}
}

func TestPackageJSONLocalToolsNotFlagged(t *testing.T) {
	// `prettier -w .` resolves from node_modules/.bin — that is the
	// lockfile's jurisdiction, not the census's.
	for _, f := range ScanPackageJSON("package.json", pkgSrc) {
		if f.Ref == "prettier" {
			t.Fatalf("locally installed tool flagged: %+v", f)
		}
	}
}

func TestPackageJSONDegenerateInputsIgnored(t *testing.T) {
	// Invalid JSON and manifests without scripts must produce nothing —
	// never an error that would abort the whole census.
	if fs := ScanPackageJSON("package.json", "{not json"); fs != nil {
		t.Fatalf("got %+v", fs)
	}
	if fs := ScanPackageJSON("package.json", `{"name":"x"}`); fs != nil {
		t.Fatalf("got %+v", fs)
	}
}

func TestPackageJSONDeterministicOrder(t *testing.T) {
	a := ScanPackageJSON("package.json", pkgSrc)
	for i := 0; i < 10; i++ {
		b := ScanPackageJSON("package.json", pkgSrc)
		for j := range a {
			if a[j] != b[j] {
				t.Fatalf("order not stable: %+v vs %+v", a, b)
			}
		}
	}
}
