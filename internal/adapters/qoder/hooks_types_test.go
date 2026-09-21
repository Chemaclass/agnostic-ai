package qoder

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_HookHTTPAndPromptUseDocumentedFields(t *testing.T) {
	testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "policy", Meta: map[string]any{"event": "PreToolUse", "matcher": "Bash", "type": "http", "url": "https://policy.example.test/check", "headers": map[string]any{"Authorization": "Bearer ${TOKEN}"}, "allowedEnvVars": []any{"TOKEN"}, "timeout": 45, "if": "Bash(*)", "once": true, "statusMessage": "Checking", "async": true, "command": "ignored", "prompt": "ignored"}},
		{Kind: spec.KindHook, Name: "done", Meta: map[string]any{"event": "PreToolUse", "matcher": "Bash", "type": "prompt", "prompt": "Verify completion.", "model": "haiku", "timeout": 30, "once": true, "statusMessage": "Reviewing", "shell": "bash", "url": "ignored"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(defaultMCPFile)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	groups := doc["hooks"].(map[string]any)["PreToolUse"].([]any)
	if len(groups) != 1 {
		t.Fatalf("groups = %#v", groups)
	}
	handlers := groups[0].(map[string]any)["hooks"].([]any)
	if len(handlers) != 2 {
		t.Fatalf("handlers = %#v", handlers)
	}
	http := handlers[0].(map[string]any)
	prompt := handlers[1].(map[string]any)
	if http["type"] != "http" || http["url"] != "https://policy.example.test/check" || http["headers"] == nil || http["allowedEnvVars"] == nil || http["timeout"] != float64(45) || http["once"] != true || http["if"] != "Bash(*)" {
		t.Errorf("http = %#v", http)
	}
	if prompt["type"] != "prompt" || prompt["prompt"] != "Verify completion." || prompt["model"] != "haiku" || prompt["timeout"] != float64(30) {
		t.Errorf("prompt = %#v", prompt)
	}
	for _, key := range []string{"command", "async", "prompt"} {
		if _, ok := http[key]; ok {
			t.Errorf("unexpected HTTP field %s", key)
		}
	}
	for _, key := range []string{"shell", "url"} {
		if _, ok := prompt[key]; ok {
			t.Errorf("unexpected prompt field %s", key)
		}
	}
}

func TestEmit_HookUnsupportedTypeReportsCoverage(t *testing.T) {
	testutil.TempCwd(t)
	notes := swapNoteWarner(t)
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindHook, Name: "agent", Meta: map[string]any{"event": "Stop", "type": "agent", "prompt": "Review", "command": "must-not-run"}}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(notes.String(), "type") {
		t.Errorf("missing coverage: %s", notes.String())
	}
	if _, err := os.Stat(defaultMCPFile); !os.IsNotExist(err) {
		t.Errorf("unexpected settings file: %v", err)
	}
}

func TestEmit_HookHTTPPreservesEmptyEnvironmentWhitelist(t *testing.T) {
	testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "none", Meta: map[string]any{"event": "PreToolUse", "type": "http", "url": "https://policy.example.test/none", "allowedEnvVars": []any{}}},
		{Kind: spec.KindHook, Name: "all", Meta: map[string]any{"event": "PreToolUse", "type": "http", "url": "https://policy.example.test/all"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(defaultMCPFile)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	groups := doc["hooks"].(map[string]any)["PreToolUse"].([]any)
	handlers := groups[0].(map[string]any)["hooks"].([]any)
	whitelist, ok := handlers[0].(map[string]any)["allowedEnvVars"].([]any)
	if !ok || len(whitelist) != 0 {
		t.Errorf("explicit empty whitelist lost: %s", raw)
	}
	if _, exists := handlers[1].(map[string]any)["allowedEnvVars"]; exists {
		t.Errorf("omitted whitelist changed: %s", raw)
	}
}
