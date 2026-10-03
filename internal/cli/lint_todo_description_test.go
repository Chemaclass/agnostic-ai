package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLint_ScaffoldedTodoDescriptionWarnsForEveryKind(t *testing.T) {
	for _, kind := range []string{"agent", "skill", "rule", "hook", "mcp"} {
		t.Run(kind, func(t *testing.T) {
			budgetProject(t, "targets: [claude]\n")
			if out, err := runCLI(t, "new", kind, "foo"); err != nil {
				t.Fatalf("new %s: %v\n%s", kind, err, out)
			}

			out, _ := runCLI(t, "lint")
			lines := findingLines(out, "LINT031")
			if len(lines) != 1 || !strings.Contains(lines[0], "foo") || !strings.Contains(lines[0], "description") {
				t.Errorf("want one LINT031 naming the file and the description field:\n%s", out)
			}
			if err := os.WriteFile(scaffoldedSpecPath(t, kind), []byte(filledSpec(kind)), 0o644); err != nil {
				t.Fatal(err)
			}
			if out, _ := runCLI(t, "lint"); len(findingLines(out, "LINT031")) != 0 {
				t.Errorf("a filled description must clear LINT031:\n%s", out)
			}
		})
	}
}

func scaffoldedSpecPath(t *testing.T, kind string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(".agnostic-ai", kind+"s", "foo.*"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("want one scaffolded %s spec, got %v (%v)", kind, matches, err)
	}
	return matches[0]
}

func filledSpec(kind string) string {
	switch kind {
	case "hook":
		return "name: foo\ndescription: Formats files after an edit.\nevent: PostToolUse\nmatcher: \"Edit|Write\"\ncommand: \"echo ok\"\n"
	case "mcp":
		return "name: foo\ndescription: Example server.\ncommand: npx\nargs: [-y, \"@example/server\"]\n"
	}
	return "---\nname: foo\ndescription: Does the foo work.\n---\n\nBody.\n"
}

func TestLint_TodoDescriptionOnlyMatchesAPlaceholderPrefix(t *testing.T) {
	budgetProject(t, "targets: [claude]\n")
	mustWriteFile(t, filepath.Join(".agnostic-ai", "skills", "notes.md"), "---\nname: notes\ndescription: Lists TODO comments in a file.\n---\n\nBody.\n")

	if out, _ := runCLI(t, "lint"); len(findingLines(out, "LINT031")) != 0 {
		t.Errorf("TODO inside a description is not the placeholder:\n%s", out)
	}
}
