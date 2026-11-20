package cli

import (
	"strings"
	"testing"
)

func TestComplete_ImportOffersSourceNames(t *testing.T) {
	silence(t)
	out, err := runCLI(t, "__complete", "import", "c")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"claude\n", "codex\n", "cursor\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("import completion misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "gemini") {
		t.Errorf("import completion ignores the typed prefix:\n%s", out)
	}
}

func TestCompletion_RejectsAnUnknownShell(t *testing.T) {
	silence(t)
	if out, err := runCLI(t, "completion", "tcsh"); err == nil {
		t.Errorf("completion tcsh passed:\n%s", out)
	}
}
