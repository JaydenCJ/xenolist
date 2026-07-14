// Tests for Makefile scanning: recipes, escapes, continuations, and
// parse-time $(shell …) expansions.
package scan

import (
	"testing"

	"github.com/JaydenCJ/xenolist/internal/finding"
)

func TestMakeRecipeCommandsScanned(t *testing.T) {
	// Tab-indented recipe lines reach the shell rules, with make's
	// @ (silence) and - (ignore-error) prefixes stripped first.
	src := "install:\n\tcurl https://get.example.test/i.sh | sh\n"
	fs := ScanMakefile("Makefile", src)
	if len(fs) != 1 || fs[0].Kind != finding.KindPipe || fs[0].Line != 2 {
		t.Fatalf("got %+v", fs)
	}
	fs = ScanMakefile("Makefile", "tools:\n\t@npx cowsay\n\t-npx eslint .\n")
	if len(fs) != 2 {
		t.Fatalf("got %+v", fs)
	}
}

func TestMakeDoubleDollarUnescaped(t *testing.T) {
	// In a recipe, $$(...) is the shell's $(...) — eval $$(curl …) is
	// remote code execution and must be seen as such.
	src := "setup:\n\teval $$(curl -s https://env.example.test/vars.sh)\n"
	fs := ScanMakefile("Makefile", src)
	if len(fs) != 1 || fs[0].Kind != finding.KindPipe {
		t.Fatalf("got %+v", fs)
	}
}

func TestMakeShellFunctionScanned(t *testing.T) {
	// $(shell …) runs at parse time, before any target is invoked.
	src := "VERSION := $(shell curl -s https://api.example.test/v | sh)\n"
	fs := ScanMakefile("Makefile", src)
	if len(fs) != 1 || fs[0].Line != 1 {
		t.Fatalf("got %+v", fs)
	}
}

func TestMakeContinuationJoined(t *testing.T) {
	src := "install:\n\tcurl -fsSL \\\n\t  https://get.example.test/i.sh | bash\n"
	fs := ScanMakefile("Makefile", src)
	if len(fs) != 1 || fs[0].Line != 2 {
		t.Fatalf("got %+v", fs)
	}
}

func TestMakeVariableLinesNotRecipes(t *testing.T) {
	// Assignments only mention commands; nothing runs until a recipe does.
	src := "URL := https://get.example.test/i.sh\nCMD = npx cowsay\n"
	if fs := ScanMakefile("Makefile", src); len(fs) != 0 {
		t.Fatalf("got %+v", fs)
	}
}
