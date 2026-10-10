package kiro

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// A user scripts directory under KIRO_HOME on Windows has backslashes. The
// command must name it with forward slashes, since a quoted folder or mixed
// separators do not run in cmd.exe or PowerShell.
func TestUserHookFiles_ScriptsDirUsesForwardSlashes(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "guard.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	const scriptsDir = `C:\Users\me\.kiro\scripts`
	for name, tc := range map[string]struct {
		meta map[string]any
		want string
	}{
		"portable command": {
			meta: map[string]any{"event": "PreToolUse", "command": ".agnostic-ai/scripts/guard.sh"},
			want: "C:/Users/me/.kiro/scripts/guard.sh",
		},
		"portable command with args": {
			meta: map[string]any{"event": "PreToolUse", "command": ".agnostic-ai/scripts/guard.sh", "args": []any{"--strict"}},
			want: "C:/Users/me/.kiro/scripts/guard.sh '--strict'",
		},
		"native command action": {
			meta: map[string]any{"event": "PreToolUse", "x-kiro": map[string]any{"action": map[string]any{"type": "command", "command": ".agnostic-ai/scripts/guard.sh"}}},
			want: "C:/Users/me/.kiro/scripts/guard.sh",
		},
	} {
		t.Run(name, func(t *testing.T) {
			files, _, err := New().UserHookFiles([]spec.Entry{{Kind: spec.KindHook, Name: "guard", Meta: tc.meta}}, source, scriptsDir)
			if err != nil {
				t.Fatal(err)
			}
			var doc hooksFile
			if err := json.Unmarshal([]byte(files["guard.json"]), &doc); err != nil {
				t.Fatalf("invalid JSON: %v\n%s", err, files["guard.json"])
			}
			action, _ := doc.Hooks[0]["action"].(map[string]any)
			if got := action["command"]; got != tc.want {
				t.Errorf("command = %q, want %q", got, tc.want)
			}
		})
	}
}
