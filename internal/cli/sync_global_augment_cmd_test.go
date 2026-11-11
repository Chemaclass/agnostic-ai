package cli

import (
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Auggie hands a command without args to cmd.exe on Windows, so a
// shared script path in a home with spaces is double-quoted there.
func TestSyncGlobal_AugmentWindowsHookQuotesForCmd(t *testing.T) {
	home, source := globalAgentTestHome(t)
	home = filepath.Join(home, "user home")
	mustWriteGlobalTest(t, filepath.Join(source, "scripts", "guard.cmd"), "@echo off\n")
	hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "SessionStart", "command": ".agnostic-ai/scripts/guard.cmd"}}
	writes, _, err := buildGlobalWritesForPlatform(home, source, []string{"augment"}, nil, spec.Bundle{Hooks: []spec.Entry{hook}}, globalState{Version: globalStateVersion}, nil, io.Discard, "warn", "windows")
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range writes {
		if w.path != filepath.Join(home, ".augment", "settings.json") {
			continue
		}
		var doc map[string]any
		if err := json.Unmarshal(w.data, &doc); err != nil {
			t.Fatal(err)
		}
		command := firstGlobalHandler(t, doc, "SessionStart")["command"].(string)
		if !strings.HasPrefix(command, `"`) || strings.Contains(command, "'") || !strings.Contains(command, "user home") {
			t.Errorf("command = %q, want a double-quoted Windows path", command)
		}
		return
	}
	t.Fatal("no ~/.augment/settings.json write")
}
