package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func geminiHookVarsProject(t *testing.T, targets, hook string) {
	t.Helper()
	dir := budgetProject(t, targets)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "check.yaml"), hook)
}

func TestLint_GeminiHookSingleQuotedVariableWarns(t *testing.T) {
	for _, name := range []string{"GEMINI_PROJECT_DIR", "GEMINI_CWD", "GEMINI_PLANS_DIR", "GEMINI_SESSION_ID", "CLAUDE_PROJECT_DIR"} {
		geminiHookVarsProject(t, "targets: [gemini]\n", "name: check\nevent: PreToolUse\ncommand: \"echo 'dir=$"+name+"'\"\n")

		out, err := runCLI(t, "lint")
		if err != nil {
			t.Fatalf("a warning alone must pass: %v\n%s", err, out)
		}
		lines := findingLines(out, "LINT030")
		if len(lines) != 1 || !strings.Contains(lines[0], "$"+name) || !strings.Contains(lines[0], "check") {
			t.Errorf("want one LINT030 naming the hook and $%s:\n%s", name, out)
		}
	}
}

func TestLint_GeminiHookVariableOutsideSingleQuotesIsClean(t *testing.T) {
	for name, command := range map[string]string{
		"unquoted":      `echo $GEMINI_PROJECT_DIR/x`,
		"double quoted": `echo "$GEMINI_PROJECT_DIR/x"`,
		"braced":        `echo '${GEMINI_PROJECT_DIR}/x'`,
		"after quotes":  `echo 'a' $GEMINI_CWD 'b'`,
		"escaped quote": `echo "it's $GEMINI_CWD"`,
	} {
		geminiHookVarsProject(t, "targets: [gemini]\n", "name: check\nevent: PreToolUse\ncommand: |-\n  "+command+"\n")

		if out, _ := runCLI(t, "lint"); len(findingLines(out, "LINT030")) != 0 {
			t.Errorf("%s: want no LINT030:\n%s", name, out)
		}
	}
}

func TestLint_SingleQuotedVariableWithoutGeminiIsClean(t *testing.T) {
	geminiHookVarsProject(t, "targets: [claude]\n", "name: check\nevent: PreToolUse\ncommand: \"echo '$GEMINI_PROJECT_DIR'\"\n")

	if out, _ := runCLI(t, "lint"); len(findingLines(out, "LINT030")) != 0 {
		t.Errorf("only Gemini rewrites the variable:\n%s", out)
	}
}

func TestLint_GeminiHookWarnsOnlyWhenHookEmitsToGemini(t *testing.T) {
	geminiHookVarsProject(t, "targets: [gemini, claude]\n", "name: check\nevent: PreToolUse\ntargets: [claude]\ncommand: \"echo '$GEMINI_PROJECT_DIR'\"\n")

	if out, _ := runCLI(t, "lint"); len(findingLines(out, "LINT030")) != 0 {
		t.Errorf("a hook scoped away from Gemini must not warn:\n%s", out)
	}
}

func TestSingleQuotedGeminiVariables(t *testing.T) {
	for command, want := range map[string][]string{
		`echo '$GEMINI_CWD'`:                                 {"$GEMINI_CWD"},
		`echo "'" '$GEMINI_CWD'`:                             {"$GEMINI_CWD"},
		`echo "'$GEMINI_CWD'"`:                               nil,
		`echo \'$GEMINI_CWD\'`:                               nil,
		`echo '$GEMINI_CWD $GEMINI_CWD' $CLAUDE_PROJECT_DIR`: {"$GEMINI_CWD"},
		`echo '$GEMINI_PLANS_DIR' '$CLAUDE_PROJECT_DIR'`:     {"$GEMINI_PLANS_DIR", "$CLAUDE_PROJECT_DIR"},
	} {
		got := singleQuotedGeminiVariables(command)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: got %v, want %v", command, got, want)
		}
	}
}
