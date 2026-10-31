package integration

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestGlobalHookExplain_MatchesNativeSyncFiles(t *testing.T) {
	packageDir, err := os.Getwd()
	must(t, err)
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Join(packageDir, "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	for _, tc := range []struct {
		target, rootEnv, filename string
	}{
		{"kiro", "KIRO_HOME", "guard.json"},
		{"grok", "GROK_HOME", "guard.json"},
		{"cline", "CLINE_DIR", "PreToolUse"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			home := t.TempDir()
			source, root := filepath.Join(home, "source"), filepath.Join(home, "custom profile")
			must(t, os.MkdirAll(filepath.Join(source, "hooks"), 0o755))
			must(t, os.WriteFile(filepath.Join(source, "agnostic-ai.yaml"), []byte("targets: ["+tc.target+"]\n"), 0o644))
			must(t, os.WriteFile(filepath.Join(source, "hooks", "guard.yaml"), []byte("name: guard\nevent: PreToolUse\ncommand: echo guard\n"), 0o644))
			run := func(args ...string) string {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, binary, args...)
				cmd.Dir = home
				cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "AGNOSTIC_AI_HOME="+source, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"), "AGNOSTIC_AI_NO_UPDATE_CHECK=1", tc.rootEnv+"="+root)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
				return string(out)
			}
			out := run("explain", "--global", "hooks/guard.yaml", "--json")
			var got struct {
				Contributions []struct{ Target, Path, Mode string }
			}
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("parse explain: %v\n%s", err, out)
			}
			path := filepath.Join(root, "hooks", tc.filename)
			if len(got.Contributions) != 1 {
				t.Fatalf("explain contributions = %+v, want %s", got.Contributions, path)
			}
			c := got.Contributions[0]
			if c.Target != tc.target || c.Path != filepath.ToSlash(path) || c.Mode != "full" {
				t.Errorf("explain contribution = %+v, want [%s] %s (full)", c, tc.target, path)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("explain wrote native hook: %v", err)
			}
			if preview := run("sync", "--global", "--only", tc.target, "--dry-run"); !strings.Contains(preview, path) {
				t.Errorf("sync preview omits explained file:\n%s", preview)
			}
			run("sync", "--global", "--only", tc.target)
			if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), "guard") {
				t.Errorf("sync did not write explained file: %v\n%s", err, data)
			}
			run("sync", "--global", "--only", tc.target, "--check")
		})
	}
}
