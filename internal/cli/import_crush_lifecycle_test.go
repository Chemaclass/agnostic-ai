package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/crush"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportCrush_LifecycleHooksRoundTrip(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [crush]\nbuiltins: []\n")
	events := []string{
		"PreToolUse", "PostToolUse", "PostToolUseFailure", "PermissionRequest", "PermissionResult",
		"SessionStart", "SessionEnd", "TurnStart", "TurnEnd", "StopFailure", "Interrupt", "PreCompact", "PostCompact",
	}
	hooks := map[string][]map[string]any{}
	for i, event := range events {
		hook := map[string]any{"name": "notify", "command": "echo " + event, "timeout": float64(1)}
		if i < 5 {
			hook["matcher"] = "^bash$"
		}
		hooks[event] = []map[string]any{hook}
	}
	body, err := json.Marshal(map[string]any{"hooks": hooks, "options": map[string]any{"debug": true}})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "crush.json"), string(body))
	execCLI(t, "import", "crush")
	files, err := os.ReadDir(filepath.Join(dir, ".agnostic-ai", "hooks"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(events) {
		t.Fatalf("imported %d hook files, want %d distinct events sharing a friendly name", len(files), len(events))
	}
	execCLI(t, "validate")
	execCLI(t, "sync", "--all")
	execCLI(t, "sync", "--check")
	execCLI(t, "lint")
	var got struct {
		Hooks   map[string][]map[string]any `json:"hooks"`
		Options map[string]any              `json:"options"`
	}
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "crush.json"))), &got); err != nil {
		t.Fatal(err)
	}
	for _, entries := range got.Hooks {
		for _, entry := range entries {
			entry["command"] = adapters.StripHookTargetExport(entry["command"].(string), "crush")
		}
	}
	if !reflect.DeepEqual(got.Hooks, hooks) {
		t.Errorf("native lifecycle hooks changed: got %#v, want %#v", got.Hooks, hooks)
	}
	if got.Options["debug"] != true {
		t.Errorf("unrelated user settings changed: %v", got.Options)
	}
	first := readFile(t, filepath.Join(dir, "crush.json"))
	execCLI(t, "import", "crush")
	execCLI(t, "sync", "--all")
	if second := readFile(t, filepath.Join(dir, "crush.json")); second != first {
		t.Errorf("re-importing duplicate labels changed native hooks:\n%s", second)
	}
}

func TestValidate_AcceptsReleasedCrushLifecycleEvents(t *testing.T) {
	for _, event := range []string{
		"PreToolUse", "PostToolUse", "PostToolUseFailure", "PermissionRequest", "PermissionResult",
		"SessionStart", "SessionEnd", "TurnStart", "TurnEnd", "StopFailure", "Interrupt", "PreCompact", "PostCompact",
		"session_start", "tUrN_eNd", "PRE_TOOL_USE",
	} {
		t.Run(event, func(t *testing.T) {
			entry := spec.Entry{Kind: spec.KindHook, Path: "hooks/audit.yaml", Meta: map[string]any{"event": event, "command": "echo audit"}}
			if issues := lintHookEvents([]spec.Entry{entry}, []string{"crush"}); len(issues) != 0 {
				t.Errorf("released Crush event %s rejected: %v", event, issues)
			}
		})
	}
}

func TestImportCrush_LifecycleAliasesStayNative(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "crush.json"), `{"hooks":{"session_start":[{"name":"start","command":"echo start"}],"tUrN_eNd":[{"name":"end","command":"echo end"}]}}`)
	hooksDir := filepath.Join(dir, ".agnostic-ai", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if n, err := importCrushHooks(dir, hooksDir); err != nil || n != 2 {
		t.Fatalf("imported %d hooks: %v", n, err)
	}
	for name, event := range map[string]string{"start": "SessionStart", "end": "TurnEnd"} {
		if got := readFile(t, filepath.Join(hooksDir, name+".yaml")); !strings.Contains(got, "event: "+event) {
			t.Errorf("%s lost native event: %s", name, got)
		}
	}
}

func TestImportCrush_RejectsLifecycleMatcherBeforeWritingSpecs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "crush.json"), `{"hooks":{"PreToolUse":[{"command":"echo valid"}],"SessionStart":[{"matcher":"^bash$","command":"echo invalid"}]}}`)
	hooksDir := filepath.Join(dir, ".agnostic-ai", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if n, err := importCrushHooks(dir, hooksDir); n != 0 || err == nil || !strings.Contains(err.Error(), "crush.json") || !strings.Contains(err.Error(), "SessionStart") || !strings.Contains(err.Error(), "matcher") {
		t.Errorf("expected invalid native matcher rejected before import, got %d, %v", n, err)
	}
	if files, err := os.ReadDir(hooksDir); err != nil || len(files) != 0 {
		t.Errorf("invalid native config wrote hook specs: %v, %v", files, err)
	}
}

func TestImportCrush_AddingDuplicateLabelKeepsExistingIdentity(t *testing.T) {
	for _, first := range []string{"PreToolUse", "SessionStart"} {
		t.Run(first+" first", func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [crush]\nbuiltins: []\n")
			hooks := map[string][]map[string]any{}
			writeNative := func() {
				t.Helper()
				body, err := json.Marshal(map[string]any{"hooks": hooks})
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(dir, "crush.json"), string(body))
			}
			hooks[first] = []map[string]any{{"name": "audit", "command": "echo " + first}}
			writeNative()
			execCLI(t, "import", "crush")
			originalPath := filepath.Join(dir, ".agnostic-ai", "hooks", "audit.yaml")
			original := readFile(t, originalPath)
			second := "SessionStart"
			if first == second {
				second = "PreToolUse"
			}
			hooks[second] = []map[string]any{{"name": "audit", "command": "echo " + second}}
			writeNative()
			for range 2 {
				execCLI(t, "import", "crush")
				files, err := os.ReadDir(filepath.Join(dir, ".agnostic-ai", "hooks"))
				if err != nil {
					t.Fatal(err)
				}
				if len(files) != 2 {
					t.Errorf("adding a second label left stale hook specs: %v", files)
				}
				if got := readFile(t, originalPath); got != original {
					t.Errorf("existing matching identity changed:\n%s", got)
				}
				execCLI(t, "sync", "--all")
				execCLI(t, "sync", "--check")
				var got struct {
					Hooks map[string][]map[string]any `json:"hooks"`
				}
				if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "crush.json"))), &got); err != nil {
					t.Fatal(err)
				}
				for event := range hooks {
					entries := got.Hooks[event]
					if len(entries) != 1 || entries[0]["name"] != "audit" || adapters.StripHookTargetExport(entries[0]["command"].(string), "crush") != "echo "+event {
						t.Errorf("%s should run exactly once: %v", event, entries)
					}
				}
			}
		})
	}
}

func TestImportCrush_KeepsAnExistingHashedHookIdentity(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [crush]\nbuiltins: []\n")
	name := hookSpecName("SessionStart", "", []string{"echo start"})
	path := filepath.Join(dir, ".agnostic-ai", "hooks", name+".yaml")
	raw, err := marshalImportedHook(map[string]any{
		"name": name, "event": "SessionStart", "command": "echo start", "target": "crush",
		"x-crush": map[string]any{"name": "audit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(raw))
	writeFile(t, filepath.Join(dir, "crush.json"), `{"hooks":{"SessionStart":[{"name":"audit","command":"echo start"}]}}`)
	execCLI(t, "import", "crush")
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name() != name+".yaml" {
		t.Errorf("existing hashed identity was duplicated or renamed: %v", files)
	}
	execCLI(t, "sync", "--all")
	var got struct {
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "crush.json"))), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Hooks["SessionStart"]) != 1 {
		t.Errorf("hashed hook runs more than once: %v", got.Hooks)
	}
}

func TestImportCrush_LocalLifecycleCommandsStayPersonal(t *testing.T) {
	for _, event := range crush.HookEvents() {
		t.Run(event, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [crush]\nbuiltins: []\n")
			shared := "name: shared\nevent: " + event + "\ncommand: echo shared\n"
			local := "name: personal\nevent: " + event + "\ncommand: [echo personal-one, echo personal-two]\n"
			writeFile(t, ".agnostic-ai/hooks/shared.yaml", shared)
			writeFile(t, filepath.Join(defaultProjectUser, "hooks/personal.yaml"), local)
			execCLI(t, "sync")
			before := readFile(t, "crush.json")
			execCLI(t, "import", "crush")
			files := sharedHookFiles(t)
			if len(files) != 1 || files["shared.yaml"] != shared {
				t.Errorf("personal commands copied into shared specs: %v", files)
			}
			if got := readFile(t, filepath.Join(defaultProjectUser, "hooks/personal.yaml")); got != local {
				t.Error("import changed personal spec")
			}
			execCLI(t, "sync")
			execCLI(t, "sync", "--check")
			if got := readFile(t, "crush.json"); got != before {
				t.Errorf("import duplicated native commands:\n%s", got)
			}
		})
	}
}

func TestImportCrush_SameCommandKeepsNativeIdentities(t *testing.T) {
	for _, tc := range []struct {
		name  string
		hooks []crushHookEntry
	}{
		{"unnamed then named", []crushHookEntry{{Command: "echo start"}, {Name: "audit", Command: "echo start"}}},
		{"label equals generated filename", []crushHookEntry{{Command: "echo start"}, {Name: hookSpecName("SessionStart", "", []string{"echo start"}), Command: "echo start"}}},
		{"spaced label different timeout", []crushHookEntry{{Name: " audit ", Command: "echo start", Timeout: 1}, {Name: " audit ", Command: "echo start", Timeout: 2}}},
		{"same label different timeout", []crushHookEntry{{Name: "audit", Command: "echo start", Timeout: 1}, {Name: "audit", Command: "echo start", Timeout: 2}}},
		{"unnamed different timeout", []crushHookEntry{{Command: "echo start", Timeout: 1}, {Command: "echo start", Timeout: 2}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [crush]\nbuiltins: []\n")
			body, err := json.Marshal(map[string]any{"hooks": map[string]any{"SessionStart": tc.hooks}})
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, "crush.json", string(body))
			dst := filepath.Join(dir, ".agnostic-ai/hooks")
			if err := os.MkdirAll(dst, 0o755); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if n, err := importCrushHooks(dir, dst); err != nil || n != 2 {
					t.Fatalf("import = %d, %v", n, err)
				}
				files, err := os.ReadDir(dst)
				if err != nil {
					t.Fatal(err)
				}
				if len(files) != 2 {
					t.Fatalf("distinct native handlers overwrote a spec: %v", files)
				}
				execCLI(t, "sync")
				var got struct {
					Hooks map[string][]crushHookEntry `json:"hooks"`
				}
				if err := json.Unmarshal([]byte(readFile(t, "crush.json")), &got); err != nil {
					t.Fatal(err)
				}
				counts := map[crushHookEntry]int{}
				for _, hook := range got.Hooks["SessionStart"] {
					hook.Command = adapters.StripHookTargetExport(hook.Command, "crush")
					counts[hook]++
				}
				for _, hook := range tc.hooks {
					hook.Name = strings.TrimSpace(hook.Name)
					if counts[hook] != 1 {
						t.Errorf("native identity %+v should run once: %v", hook, counts)
					}
				}
			}
		})
	}
}

func TestImportCrush_FilenameCollisionKeepsExistingHook(t *testing.T) {
	dir := t.TempDir()
	last := crushHookEntry{Name: "audit", Command: "echo start", Timeout: 2}
	identity, err := json.Marshal(last)
	if err != nil {
		t.Fatal(err)
	}
	name := hookSpecName("SessionStart", "", []string{string(identity)})
	first := crushHookEntry{Name: name, Command: "echo other"}
	hooks := []crushHookEntry{first, {Name: "audit", Command: "echo start", Timeout: 1}, last}
	body, err := json.Marshal(map[string]any{"hooks": map[string]any{"SessionStart": hooks}})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "crush.json"), string(body))
	dst := filepath.Join(dir, ".agnostic-ai/hooks")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dst, name+".yaml")
	if _, err := importCrushHooks(dir, dst); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("filename collision must diagnose the conflicting path: %v", err)
	}
	if !matchingCrushHook(path, "SessionStart", first) {
		t.Errorf("filename collision overwrote another native identity: %s", readFile(t, path))
	}
}
