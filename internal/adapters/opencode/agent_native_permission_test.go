package opencode

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_Agent_NativeV2ProjectPermissions(t *testing.T) {
	type probe struct{ action, resource, effect string }
	cases := []struct {
		name, project string
		tools         []any
		probes        []probe
	}{
		{"read deny", `{"permissions":[{"action":"read","resource":"secrets/*","effect":"deny"}]}`, []any{"read"}, []probe{
			{"read", "secrets/token.txt", "deny"}, {"read", "src/main.go", "allow"}, {"read", ".env", "ask"}, {"edit", "src/main.go", "deny"},
		}},
		{"read ask", `{"permissions":[{"action":"read","resource":"secrets/*","effect":"ask"}]}`, []any{"read"}, []probe{
			{"read", "secrets/token.txt", "ask"}, {"read", "src/main.go", "allow"},
		}},
		{"ordered repeated resources", `{"permissions":[{"action":"read","resource":"secrets/*","effect":"deny"},{"action":"r*","resource":"secrets/public/*","effect":"allow"},{"action":"read","resource":"secrets/*","effect":"ask"},{"action":"read","resource":"secrets/public/*","effect":"allow"}]}`, []any{"read"}, []probe{
			{"read", "secrets/token.txt", "ask"}, {"read", "secrets/public/readme.txt", "allow"},
		}},
		{"native after legacy", `{"permission":{"read":"deny"},"permissions":[{"action":"r*","resource":"src/*","effect":"allow"}]}`, []any{"read"}, []probe{
			{"read", "src/main.go", "allow"}, {"read", "secrets/token.txt", "deny"},
		}},
		{"native wildcard after legacy aliases", `{"permission":{"bash":"deny","task":"ask"},"permissions":[{"action":"she*","resource":"git *","effect":"allow"},{"action":"subagent","resource":"private*","effect":"deny"}]}`, []any{"shell", "Task"}, []probe{
			{"shell", "git status", "allow"}, {"shell", "rm file", "deny"}, {"subagent", "review", "ask"}, {"subagent", "private-review", "deny"},
		}},
		{"global native wildcard", `{"permissions":[{"action":"*","resource":"*","effect":"ask"},{"action":"shell","resource":"git status *","effect":"allow"}]}`, []any{"read", "shell"}, []probe{
			{"read", "src/main.go", "ask"}, {"shell", "git status", "allow"}, {"shell", "git push main", "ask"}, {"edit", "src/main.go", "deny"},
		}},
		{"wildcard native aliases", `{"permissions":[{"action":"she*","resource":"git *","effect":"ask"},{"action":"shell","resource":"git push *","effect":"deny"},{"action":"sub*","resource":"private*","effect":"deny"}]}`, []any{"shell(git *)", "Task"}, []probe{
			{"shell", "git status", "ask"}, {"shell", "git push main", "deny"}, {"shell", "rm file", "deny"}, {"subagent", "private-review", "deny"}, {"subagent", "review", "allow"},
		}},
		{"native actions are not legacy aliases", `{"permissions":[{"action":"b*","resource":"*","effect":"deny"},{"action":"task","resource":"*","effect":"deny"}]}`, []any{"shell", "Task"}, []probe{
			{"shell", "git status", "allow"}, {"subagent", "review", "allow"},
		}},
		{"wildcard action limits whole MCP server", `{"permissions":[{"action":"github_*","resource":"*","effect":"ask"},{"action":"*_delete","resource":"*","effect":"deny"}]}`, []any{"mcp:github"}, []probe{
			{"github_search", "x", "ask"}, {"github_delete", "x", "deny"}, {"other_search", "x", "deny"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			if err := os.WriteFile("opencode.json", []byte(tc.project), 0o644); err != nil {
				t.Fatal(err)
			}
			agent, problem := (spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"description": "Review", "can": tc.tools}, Body: "Review."}).NativeTools()
			if problem != "" {
				t.Fatal(problem)
			}
			if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{agent}), &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			rules := simAgentPermission(t, filepath.Join(dir, ".opencode/agents/reviewer.md"))
			// V2's legacy migrator renames exact bash/task keys, then its
			// evaluator uses the last matching action and resource rule.
			for i := range rules {
				switch rules[i].key {
				case "bash":
					rules[i].key = "shell"
				case "task":
					rules[i].key = "subagent"
				}
			}
			for _, p := range tc.probes {
				for _, windows := range []bool{false, true} {
					if got := simEvaluate(rules, p.action, p.resource, windows); got != p.effect {
						t.Errorf("%s %q (windows %t): got %s, want %s; emitted rules %v", p.action, p.resource, windows, got, p.effect, rules)
					}
				}
			}
		})
	}
}

func TestEmit_Agent_NativeV2PlannedRulesAgreeAcrossModes(t *testing.T) {
	for _, mode := range []string{"write", "check", "dry-run"} {
		t.Run(mode, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			original := `{"permission":{"read":{"disk/*":"deny"}},"permissions":[{"action":"read","resource":"old/*","effect":"deny"}]}`
			if err := os.WriteFile("opencode.json", []byte(original), 0o644); err != nil {
				t.Fatal(err)
			}
			var native []any
			if err := json.Unmarshal([]byte(`[{"action":"read","resource":"secrets/*","effect":"ask"}]`), &native); err != nil {
				t.Fatal(err)
			}
			settings := spec.Entry{Kind: spec.KindSettings, Name: "s", Meta: map[string]any{"x-opencode": map[string]any{"permissions": native}}}
			agent := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"description": "Review", "tools": []any{"Read"}}, Body: "Review."}
			sess := emit.NewSession()
			if mode == "check" {
				sess.StartCapture()
			}
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			stdout := os.Stdout
			os.Stdout = w
			emitErr := New().Emit(sess, spec.NewBundle([]spec.Entry{settings, agent}), &config.Config{}, mode == "dry-run")
			os.Stdout = stdout
			_ = w.Close()
			printed, _ := io.ReadAll(r)
			_ = r.Close()
			if emitErr != nil {
				t.Fatal(emitErr)
			}
			var got string
			switch mode {
			case "write":
				got = readFile(t, filepath.Join(dir, ".opencode/agents/reviewer.md"))
			case "check":
				for _, f := range sess.StopCapture() {
					if filepath.Base(f.Path) == "reviewer.md" {
						got = f.Content
					}
				}
			case "dry-run":
				preview := string(printed)
				start := strings.Index(preview, filepath.FromSlash(".opencode/agents/reviewer.md")+" ---")
				if start < 0 {
					t.Fatalf("no agent preview in %s", preview)
				}
				got = preview[start:]
			}
			for _, want := range []string{"secrets/*: ask", "disk/*: deny"} {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in %s", want, got)
				}
			}
			if strings.Contains(got, "old/*") {
				t.Errorf("planned native array must replace the disk array: %s", got)
			}
			if mode != "write" && readFile(t, "opencode.json") != original {
				t.Error("preview changed the project file")
			}
		})
	}
}

func TestEmit_Agent_RejectsNativeV2RulesItCannotPreserve(t *testing.T) {
	for _, permissions := range []string{
		`{"read":"deny"}`,
		`[{"action":"read","resource":"secrets/*","effect":"ignore"}]`,
		`[{"action":"read","effect":"deny"}]`,
		`[{"resource":"secrets/*","effect":"deny"}]`,
		`[{"Action":"read","resource":"secrets/*","effect":"allow"}]`,
		`[{"action":"doom_loop","resource":"private*","effect":"deny"}]`,
	} {
		t.Run(permissions, func(t *testing.T) {
			testutil.TempCwd(t)
			if err := os.WriteFile("opencode.json", []byte(`{"permissions":`+permissions+`}`), 0o644); err != nil {
				t.Fatal(err)
			}
			agent := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"tools": []any{"Read"}}}
			err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{agent}), &config.Config{}, false)
			if err == nil || !strings.Contains(err.Error(), "permission") || !strings.Contains(err.Error(), "opencode.json") {
				t.Errorf("want path and permission context for unsupported native rules, got %v", err)
			}
		})
	}
}

func TestEmit_Agent_NativeV2ExplicitPermissionOverridesUnrepresentableRules(t *testing.T) {
	for _, override := range []any{
		map[string]any{"doom_loop": "deny"},
		"deny",
		"ask",
	} {
		t.Run(fmt.Sprint(override), func(t *testing.T) {
			dir := testutil.TempCwd(t)
			mustWrite := `{"permissions":[{"action":"doom_loop","resource":"private*","effect":"deny"}]}`
			if err := os.WriteFile("opencode.json", []byte(mustWrite), 0o644); err != nil {
				t.Fatal(err)
			}
			agent := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{
				"tools": []any{"Read"}, "x-opencode": map[string]any{"permission": override},
			}}
			if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{agent}), &config.Config{}, false); err != nil {
				t.Fatalf("explicit permission override must win: %v", err)
			}
			rules := simAgentPermission(t, filepath.Join(dir, ".opencode/agents/reviewer.md"))
			want := "deny"
			if action, _ := override.(string); action == "ask" {
				want = "ask"
			}
			if got := simEvaluate(rules, "doom_loop", "private-loop", false); got != want {
				t.Errorf("doom_loop = %s, want explicit %s", got, want)
			}
		})
	}
}

func TestEmit_Agent_NativeV2RejectsHomeResourcesThatNeedRuntimeExpansion(t *testing.T) {
	cases := []struct{ action, resource, tool string }{
		{"read", "~/secrets/*", "read(/home/u/secrets/*)"},
		{"edit", "$HOME/secrets/*", "edit(/home/u/secrets/*)"},
		{"read", `$HOME\secrets\*`, `read(C:\Users\u\secrets\*)`},
		{"read", "~", "read(/home/u/*)"},
		{"read", "$HOME", "read(/home/u/*)"},
		{"read", "/home/u/secrets/*", "read(~/secrets/*)"},
		{"r*", "~/secrets/*", "read"},
		{"*", "$HOME/secrets/*", "read"},
	}
	for _, tc := range cases {
		t.Run(tc.action+"/"+tc.resource+"/"+tc.tool, func(t *testing.T) {
			testutil.TempCwd(t)
			settings := spec.Entry{Kind: spec.KindSettings, Name: "settings", Meta: map[string]any{
				"x-opencode": map[string]any{"permissions": []any{
					map[string]any{"action": tc.action, "resource": tc.resource, "effect": "deny"},
				}},
			}}
			agent, problem := (spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"can": []any{tc.tool}}}).NativeTools()
			if problem != "" {
				t.Fatal(problem)
			}
			err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{settings, agent}), &config.Config{}, false)
			if err == nil || !strings.Contains(err.Error(), "home") || !strings.Contains(err.Error(), "opencode.json") || !strings.Contains(err.Error(), "reviewer.md") {
				t.Errorf("want home expansion error with project and agent paths, got %v", err)
			}
		})
	}
}

func TestEmit_Agent_NativeV2WholeToolKeepsSymbolicHomeResources(t *testing.T) {
	cases := []struct{ action, resource, effect, tool, outside string }{
		{"read", "~/secrets/*", "deny", "read", "allow"},
		{"edit", "$HOME/secrets/*", "ask", "edit", "allow"},
		{"external_directory", `$HOME\secrets\*`, "deny", "read", "ask"},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			settings := spec.Entry{Kind: spec.KindSettings, Name: "settings", Meta: map[string]any{
				"x-opencode": map[string]any{"permissions": []any{
					map[string]any{"action": tc.action, "resource": tc.resource, "effect": tc.effect},
				}},
			}}
			agent, problem := (spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"can": []any{tc.tool}}}).NativeTools()
			if problem != "" {
				t.Fatal(problem)
			}
			if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{settings, agent}), &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			original := simAgentPermission(t, filepath.Join(dir, ".opencode/agents/reviewer.md"))
			for _, home := range []string{"/runtime/alice", "/runtime/bob"} {
				rules := append([]simRule(nil), original...)
				found := false
				for i := range rules {
					if rules[i].key == tc.action && rules[i].pattern == tc.resource {
						found = true
						rules[i].pattern = home + "/secrets/*"
					}
				}
				if !found {
					t.Fatalf("symbolic home rule lost from emitted permissions: %v", original)
				}
				if got := simEvaluate(rules, tc.action, home+"/secrets/token.txt", false); got != tc.effect {
					t.Errorf("runtime home %s: got %s, want %s", home, got, tc.effect)
				}
				if got := simEvaluate(rules, tc.action, home+"/public/readme.txt", false); got != tc.outside {
					t.Errorf("outside the home rule: got %s, want %s", got, tc.outside)
				}
			}
		})
	}
}

func TestEmit_Agent_NativeV2HomeRulesKeepExplicitOverrides(t *testing.T) {
	for _, override := range []any{map[string]any{"read": "deny"}, "deny", "ask"} {
		t.Run(fmt.Sprint(override), func(t *testing.T) {
			dir := testutil.TempCwd(t)
			if err := os.WriteFile("opencode.json", []byte(`{"permissions":[{"action":"read","resource":"~/secrets/*","effect":"deny"}]}`), 0o644); err != nil {
				t.Fatal(err)
			}
			agent := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{
				"tools": []any{"Read(/home/u/secrets/*)"}, "x-opencode": map[string]any{"permission": override},
			}}
			if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{agent}), &config.Config{}, false); err != nil {
				t.Fatalf("explicit permission override must win: %v", err)
			}
			rules := simAgentPermission(t, filepath.Join(dir, ".opencode/agents/reviewer.md"))
			want := "deny"
			if action, _ := override.(string); action == "ask" {
				want = "ask"
			}
			if got := simEvaluate(rules, "read", "/home/u/secrets/token.txt", false); got != want {
				t.Errorf("read = %s, want explicit %s", got, want)
			}
		})
	}
}
