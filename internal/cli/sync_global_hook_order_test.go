package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_KeepsHandWrittenHookKeyOrder(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "fmt.yaml"), "event: PostToolUse\nmatcher: Edit\ncommand: gofmt -l .\n")
	path := filepath.Join(home, ".claude", "settings.json")
	mine := `{
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "rtk hook claude"
          }
        ]
      }`
	mustWriteGlobalTest(t, path, "{\n  \"hooks\": {\n    \"PreToolUse\": [\n      "+mine+"\n    ]\n  }\n}\n")
	if _, w, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	got := readGlobalTest(t, path)
	if !strings.Contains(got, mine) {
		t.Errorf("the hand-written hook must keep its key order:\n%s", got)
	}
	if strings.Index(got, `"PreToolUse"`) > strings.Index(got, `"PostToolUse"`) {
		t.Errorf("the file's own event must stay first:\n%s", got)
	}
}
