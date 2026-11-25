package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportClaude_KeepsHandWrittenHookWithSimilarScriptName(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".claude", "settings.json"), `{"hooks":{"SessionStart":[
		{"matcher":"startup|resume","hooks":[
			{"type":"command","command":"sh ./scripts/agnostic-ai-cloud-install.sh.backup"},
			{"type":"command","command":"f=\"$CLAUDE_PROJECT_DIR\"/'.claude/hooks/agnostic-ai-cloud-install.sh'; [ ! -f \"$f\" ] || sh \"$f\""}
		]}
	]}}`)
	dst := filepath.Join(root, "hooks")
	if err := os.Mkdir(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	n, err := importClaudeHooks(root, dst)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("imported %d hooks, want the single hand-written hook", n)
	}
	files, err := os.ReadDir(dst)
	if err != nil || len(files) != 1 {
		t.Fatalf("imported files: %v, %v", files, err)
	}
	body, err := os.ReadFile(filepath.Join(dst, files[0].Name()))
	if err != nil || !strings.Contains(string(body), "agnostic-ai-cloud-install.sh.backup") {
		t.Errorf("imported hook: %s, %v", body, err)
	}
}
