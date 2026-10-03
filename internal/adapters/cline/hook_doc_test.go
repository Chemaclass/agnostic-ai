package cline

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestHookScript_IsTheEventScriptSyncWritesForOneSpec(t *testing.T) {
	h := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "command": []any{"./a.sh", "./b.sh"}}}
	emitTargetHooks(t, &config.Config{}, h)

	got := HookScript(h)
	if got != "set -e\nexport AGNOSTIC_AI_TARGET=cline\n\n./a.sh\n\n./b.sh\n" {
		t.Errorf("HookScript = %q", got)
	}
	if synced := readTargetFile(t, ".cline/hooks/PreToolUse.sh"); !strings.HasSuffix(synced, got) {
		t.Errorf("the synced script ends with a different body:\n%s", synced)
	}
	if HookScript(spec.Entry{Meta: map[string]any{"event": "PreToolUse"}}) != "" {
		t.Error("a spec with no command has no script")
	}
}

func TestHookScriptPath_FoldsTheEventAndFollowsHooksDir(t *testing.T) {
	if got := HookScriptPath(&config.Config{}, "pretooluse"); got != filepath.Join(".cline", "hooks", "PreToolUse.sh") {
		t.Errorf("default path = %q", got)
	}
	cfg := &config.Config{Outputs: map[string]config.Output{"cline": {HooksDir: ".clinerules/hooks"}}}
	if got := HookScriptPath(cfg, "PostToolUse"); got != filepath.Join(".clinerules/hooks", "PostToolUse.sh") {
		t.Errorf("hooks-dir path = %q", got)
	}
	if got := HookScriptPath(&config.Config{}, "Stop"); got != "" {
		t.Errorf("Cline reads no Stop script, got %q", got)
	}
}
