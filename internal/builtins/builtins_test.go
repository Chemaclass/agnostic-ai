package builtins_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/builtins"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func cacheForTest(t *testing.T) string {
	t.Helper()
	root := testutil.TempCwd(t)
	home := filepath.Join(root, "home")
	cache := filepath.Join(home, "cache")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("LocalAppData", cache)
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("user cache dir: %v", err)
	}
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatalf("create cache %s: %v", cache, err)
	}
	temp := filepath.Join(root, "temp")
	if err := os.MkdirAll(temp, 0o755); err != nil {
		t.Fatalf("create temp %s: %v", temp, err)
	}
	t.Setenv("TMPDIR", temp)
	t.Setenv("TEMP", temp)
	t.Setenv("TMP", temp)
	return cache
}

func materializeHandoff(t *testing.T) string {
	t.Helper()
	root, cleanup, err := builtins.Materialize("handoff")
	if err != nil {
		t.Fatalf("materialize handoff: %v", err)
	}
	if cleanup == nil {
		t.Fatal("materialize returned no cleanup")
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup %s: %v", root, err)
		}
	})
	return root
}

func skillPath(root string) string {
	return filepath.Join(root, "skills", "handoff", "SKILL.md")
}

func TestNames_ReturnsValidNamesWithoutSharingTheList(t *testing.T) {
	want := []string{"caveman", "cloud-handoff", "handoff", "handoff-hook", "memory", "rtk", "side-conversation", "wrap-up"}
	if got := builtins.Names(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	names := builtins.Names()
	names[0] = "changed"
	if got := builtins.Names(); !reflect.DeepEqual(got, want) {
		t.Errorf("Names() after caller mutation = %v, want %v", got, want)
	}
}

func TestHash_IsStableSHA256AndRejectsUnknownNames(t *testing.T) {
	hash := builtins.Hash("handoff")
	decoded, err := hex.DecodeString(hash)
	if err != nil || len(decoded) != 32 {
		t.Errorf("Hash(handoff) = %q, want a SHA256 hex digest", hash)
	}
	if got := builtins.Hash("handoff"); got != hash {
		t.Errorf("second hash = %q, want %q", got, hash)
	}
	for _, name := range []string{"nope", "", "../handoff"} {
		if got := builtins.Hash(name); got != "" {
			t.Errorf("Hash(%q) = %q, want empty", name, got)
		}
	}
}

func TestMaterialize_RejectsUnknownNames(t *testing.T) {
	cache := cacheForTest(t)
	root, cleanup, err := builtins.Materialize("../handoff")
	if err == nil || !strings.Contains(err.Error(), "../handoff") {
		t.Errorf("Materialize unknown error = %v, want the rejected name", err)
	}
	if root != "" || cleanup != nil {
		t.Errorf("unknown builtin returned root %q and cleanup %v", root, cleanup != nil)
	}
	entries, err := os.ReadDir(cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("unknown builtin created cache entries: %v", entries)
	}
}

func TestMaterialize_LoadsTheFullHandoffAsAnOrdinaryLayer(t *testing.T) {
	cache := cacheForTest(t)
	root := materializeHandoff(t)
	wantRoot := filepath.Join(cache, "agnostic-ai", "builtins", builtins.Hash("handoff"))
	if root != wantRoot {
		t.Errorf("root = %q, want %q", root, wantRoot)
	}
	bundle, err := spec.LoadLayered([]spec.Layer{{
		Name: "builtin", Root: root, Sources: config.Sources{Skills: "skills"},
	}})
	if err != nil {
		t.Fatalf("load builtin layer: %v", err)
	}
	if len(bundle.All()) != 1 || len(bundle.Skills) != 1 {
		t.Fatalf("builtin entries = %v, want one skill", bundle.All())
	}
	skill := bundle.Skills[0]
	if skill.Name != "handoff" || skill.Layer != "builtin" || skill.Scope != "" {
		t.Errorf("skill identity = %q layer %q scope %q", skill.Name, skill.Layer, skill.Scope)
	}
	if skill.Description() != "Write or resume a session handoff between AI tools." {
		t.Errorf("description = %q", skill.Description())
	}
	if skill.SkillAssetDir() != filepath.Dir(skillPath(root)) {
		t.Errorf("skill asset dir = %q", skill.SkillAssetDir())
	}
	template := "# Handoff\n" +
		"tool: codex | date: 2026-10-05T14:02Z | branch: feat/x | head: abc1234\n" +
		"worktree: /path/to/checkout | status: active | pr: none\n\n" +
		"## Goal\n## Acceptance\n## Done\n## In progress\n## Next steps\n" +
		"## Open decisions\n## Gotchas\n## Verify\n"
	if !strings.Contains(skill.Body, template) {
		t.Error("skill is missing the agreed handoff template")
	}
	for _, rule := range []string{
		"write a handoff", "continue where", "explicit argument", "current project",
		"globally", ".agnostic-ai/local/HANDOFF.md", "100 lines", "Gotchas", "Next steps",
		"git branch --show-current", "git rev-parse HEAD", "git status --short",
		"secrets", "tokens", ".env", "memory store", "HANDOFF.auto.md", "newer",
		"drift", "confirmation", "overwrites", "session start",
		"git worktree list --porcelain", "handoffs/", "0600", "temporary file", "rename",
		"branch-slug", "check-ignore", "removed",
		"nearest ancestor", "agnostic-ai.yaml", "agnostic.config.yaml",
	} {
		if !strings.Contains(skill.Body, rule) {
			t.Errorf("skill is missing rule %q", rule)
		}
	}
	rootInstructions, _, _ := strings.Cut(skill.Body, "\n## Write\n")
	for _, rule := range []string{
		"canonical", "AGNOSTIC_AI_HOME", "~/.agnostic-ai",
		"exclude the effective global source root", "git rev-parse --show-toplevel",
	} {
		if !strings.Contains(rootInstructions, rule) {
			t.Errorf("skill root lookup is missing rule %q", rule)
		}
	}
	for _, rule := range []string{
		"Write the file before proposing learnings",
		"[project]", "[personal]", "[global]",
		"append-only diff per file", "YAML frontmatter", "alwaysApply: true",
		"name: learnings-local", "omit `scope`, `globs`, and `paths`",
		"confirmation of each item's content and scope", "only approved items",
		"never rewrite existing entries", "explicitly picks it for that item",
		"Refresh the handoff header, Done, and Verify", "failed or pending syncs",
	} {
		if !strings.Contains(skill.Body, rule) {
			t.Errorf("skill is missing learning contract %q", rule)
		}
	}
	if strings.Contains(skill.Body, "hooks/") {
		t.Error("handoff skill unexpectedly names a hooks path")
	}
}

func TestMaterialize_CachesReadOnlyFilesAndKeepsThemAfterCleanup(t *testing.T) {
	cacheForTest(t)
	root, cleanup, err := builtins.Materialize("handoff")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{skillPath(root), filepath.Join(root, ".complete")} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o222 != 0 {
			t.Errorf("%s has mode %s, want a read-only regular file", path, info.Mode())
		}
	}
	if err := cleanup(); err != nil {
		t.Errorf("cached cleanup: %v", err)
	}
	if _, err := os.ReadFile(skillPath(root)); err != nil {
		t.Errorf("cached cleanup removed the skill: %v", err)
	}
	if again := materializeHandoff(t); again != root {
		t.Errorf("second root = %q, want %q", again, root)
	}
}

func TestMaterialize_NewerAutoSnapshotKeepsManualTaskDetails(t *testing.T) {
	cacheForTest(t)
	root := materializeHandoff(t)
	bundle, err := spec.LoadLayered([]spec.Layer{{
		Name: "builtin", Root: root, Sources: config.Sources{Skills: "skills"},
	}})
	if err != nil {
		t.Fatalf("load builtin layer: %v", err)
	}
	if len(bundle.Skills) != 1 {
		t.Fatalf("skills = %v, want one handoff skill", bundle.Skills)
	}
	for _, rule := range []string{
		"Keep the manual Goal and Next steps",
		"additional recent git evidence",
		"Compare the chosen manual header",
		"even when a snapshot is newer",
	} {
		if !strings.Contains(bundle.Skills[0].Body, rule) {
			t.Errorf("resume instructions are missing %q", rule)
		}
	}
}

func TestMaterialize_RebuildsIncompleteOrEditedCache(t *testing.T) {
	for _, damage := range []string{"missing marker", "edited marker", "missing skill", "edited skill", "writable skill", "extra file"} {
		t.Run(damage, func(t *testing.T) {
			cacheForTest(t)
			root := materializeHandoff(t)
			original, err := os.ReadFile(skillPath(root))
			if err != nil {
				t.Fatal(err)
			}
			switch damage {
			case "missing marker":
				err = os.Remove(filepath.Join(root, ".complete"))
			case "edited marker":
				path := filepath.Join(root, ".complete")
				if err = os.Chmod(path, 0o644); err == nil {
					err = os.WriteFile(path, []byte("other hash\n"), 0o644)
				}
			case "missing skill":
				err = os.Remove(skillPath(root))
			case "edited skill":
				if err = os.Chmod(skillPath(root), 0o644); err == nil {
					err = os.WriteFile(skillPath(root), []byte("edited\n"), 0o644)
				}
			case "writable skill":
				err = os.Chmod(skillPath(root), 0o644)
			case "extra file":
				err = os.WriteFile(filepath.Join(root, "extra.md"), []byte("extra\n"), 0o644)
			}
			if err != nil {
				t.Fatalf("damage cache: %v", err)
			}
			if got := materializeHandoff(t); got != root {
				t.Errorf("rebuilt root = %q, want %q", got, root)
			}
			got, err := os.ReadFile(skillPath(root))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(original) {
				t.Error("rebuilt skill differs from embedded content")
			}
			if _, err := os.Stat(filepath.Join(root, ".complete")); err != nil {
				t.Errorf("rebuilt marker: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "extra.md")); !os.IsNotExist(err) {
				t.Errorf("extra cache file survived: %v", err)
			}
		})
	}
}

func TestMaterialize_FallsBackWhenCacheCannotBeWrittenAndCleansTemp(t *testing.T) {
	for _, failure := range []string{"missing cache", "blocked cache", "blocked namespace", "no cache location"} {
		t.Run(failure, func(t *testing.T) {
			cache := cacheForTest(t)
			if failure != "blocked namespace" {
				if err := os.Remove(cache); err != nil {
					t.Fatal(err)
				}
			}
			switch failure {
			case "blocked cache":
				if err := os.WriteFile(cache, []byte("not a directory\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "blocked namespace":
				if err := os.WriteFile(filepath.Join(cache, "agnostic-ai"), []byte("not a directory\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "no cache location":
				t.Setenv("HOME", "")
				t.Setenv("XDG_CACHE_HOME", "")
				t.Setenv("LocalAppData", "")
			}
			root, cleanup, err := builtins.Materialize("handoff")
			if err != nil {
				t.Fatalf("fallback: %v", err)
			}
			if strings.HasPrefix(root, cache+string(filepath.Separator)) {
				t.Errorf("fallback root %q is under unusable cache %q", root, cache)
			}
			if _, err := os.ReadFile(skillPath(root)); err != nil {
				t.Errorf("fallback skill: %v", err)
			}
			if err := cleanup(); err != nil {
				t.Errorf("fallback cleanup: %v", err)
			}
			if _, err := os.Stat(root); !os.IsNotExist(err) {
				t.Errorf("fallback root remains after cleanup: %v", err)
			}
		})
	}
}

func TestMaterialize_ConcurrentProcessesUseOneCompleteCache(t *testing.T) {
	cache := cacheForTest(t)
	wantRoot := filepath.Join(cache, "agnostic-ai", "builtins", builtins.Hash("handoff"))
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			cmd := exec.Command(executable, "-test.run=^TestMaterialize_ProcessHelper$")
			cmd.Env = append(os.Environ(), "AGNOSTIC_AI_BUILTINS_TEST_PROCESS=1", "AGNOSTIC_AI_BUILTINS_TEST_ROOT="+wantRoot)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("concurrent materialize: %v\n%s", err, out)
			}
		})
	}
	wg.Wait()
	if got := materializeHandoff(t); got != wantRoot {
		t.Errorf("cache root = %q, want %q", got, wantRoot)
	}
}

func TestMaterialize_ProcessHelper(t *testing.T) {
	if os.Getenv("AGNOSTIC_AI_BUILTINS_TEST_PROCESS") != "1" {
		return
	}
	root, cleanup, err := builtins.Materialize("handoff")
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if want := os.Getenv("AGNOSTIC_AI_BUILTINS_TEST_ROOT"); root != want {
		t.Errorf("root = %q, want %q", root, want)
	}
	if _, err := spec.LoadLayered([]spec.Layer{{Name: "builtin", Root: root, Sources: config.Sources{Skills: "skills"}}}); err != nil {
		t.Errorf("concurrent load: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".complete")); err != nil {
		t.Errorf("concurrent complete marker: %v", err)
	}
	if err := cleanup(); err != nil {
		t.Error(fmt.Errorf("cleanup %s: %w", root, err))
	}
}

func TestMaterialize_MemoryIncludesBothIndexesForKiroAlone(t *testing.T) {
	cacheForTest(t)
	root, cleanup, err := builtins.Materialize("memory")
	if err != nil {
		t.Fatalf("materialize memory: %v", err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup %s: %v", root, err)
		}
	})
	bundle, err := spec.LoadLayered([]spec.Layer{{Name: "builtin", Root: root, Sources: config.Sources{Rules: "rules"}}})
	if err != nil {
		t.Fatalf("load memory layer: %v", err)
	}
	var include spec.Entry
	var names []string
	for _, rule := range bundle.Rules {
		names = append(names, rule.Name)
		if rule.Name == "kiro-memory-index" {
			include = rule
		}
	}
	if include.Name == "" {
		t.Fatalf("memory rules = %v, want a kiro-memory-index rule", names)
	}
	if include.Meta["alwaysApply"] != true {
		t.Errorf("alwaysApply = %v, want true", include.Meta["alwaysApply"])
	}
	for _, target := range adapters.Names() {
		if got, want := include.EmitsTo(target), target == "kiro"; got != want {
			t.Errorf("EmitsTo(%s) = %t, want %t", target, got, want)
		}
	}
	for _, index := range []string{".agnostic-ai/memory/MEMORY.md", ".agnostic-ai/local/memory/MEMORY.md"} {
		if got := strings.Count(include.Body, "#[[file:"+index+"]]"); got != 1 {
			t.Errorf("body includes %s %d times, want once:\n%s", index, got, include.Body)
		}
	}
	if got := strings.Count(include.Body, "#[["); got != 2 {
		t.Errorf("body has %d includes, want the two indexes only:\n%s", got, include.Body)
	}
}

func TestMaterialize_LoadsHandoffHooksAsAnOrdinaryLayer(t *testing.T) {
	cacheForTest(t)
	root, cleanup, err := builtins.Materialize("handoff-hook")
	if err != nil {
		t.Fatalf("materialize handoff-hook: %v", err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup %s: %v", root, err)
		}
	})
	bundle, err := spec.LoadLayered([]spec.Layer{{Name: "builtin", Root: root, Sources: config.Sources{Hooks: "hooks"}}})
	if err != nil {
		t.Fatalf("load handoff-hook layer: %v", err)
	}
	if len(bundle.All()) != 3 || len(bundle.Hooks) != 3 {
		t.Fatalf("entries = %v, want three hooks", bundle.All())
	}
	for _, target := range []string{"claude", "codex", "gemini", "qoder", "factory"} {
		if hooks := bundle.HooksFor(target); len(hooks) != 3 {
			t.Errorf("%s hook count = %d, want three", target, len(hooks))
		}
	}
	for _, target := range []string{"goose", "copilot", "augment", "openhands"} {
		if hooks := bundle.HooksFor(target); len(hooks) != 0 {
			t.Errorf("unverified target %s received %d hooks", target, len(hooks))
		}
	}
	wantEvents := map[string]string{"handoff-pre-compact": "PreCompact", "handoff-session-end": "SessionEnd", "handoff-session-start": "SessionStart"}
	for _, hook := range bundle.Hooks {
		if hook.Layer != "builtin" || hook.Meta["event"] != wantEvents[hook.Name] {
			t.Errorf("hook %s layer %s event %v", hook.Name, hook.Layer, hook.Meta["event"])
		}
		if _, exists := hook.Meta["matcher"]; exists {
			t.Errorf("hook %s filters lifecycle sources", hook.Name)
		}
	}
}

func TestMaterialize_LoadsWrapUpAsOneSkill(t *testing.T) {
	cacheForTest(t)
	root, cleanup, err := builtins.Materialize("wrap-up")
	if err != nil {
		t.Fatalf("materialize wrap-up: %v", err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup %s: %v", root, err)
		}
	})
	var files []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == ".complete" {
			return err
		}
		rel, err := filepath.Rel(root, path)
		files = append(files, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(files, []string{"skills/wrap-up/SKILL.md"}) {
		t.Errorf("materialized files = %v, want only the skill", files)
	}
	bundle, err := spec.LoadLayered([]spec.Layer{{
		Name: "builtin", Root: root, Sources: config.Sources{Skills: "skills", Hooks: "hooks"},
	}})
	if err != nil {
		t.Fatalf("load builtin layer: %v", err)
	}
	if len(bundle.All()) != 1 || len(bundle.Skills) != 1 || len(bundle.Hooks) != 0 {
		t.Fatalf("builtin entries = %v, want one skill", bundle.All())
	}
	skill := bundle.Skills[0]
	if skill.Name != "wrap-up" || skill.Layer != "builtin" {
		t.Errorf("skill identity = %q layer %q", skill.Name, skill.Layer)
	}
	if skill.Description() != `Write an end-of-session summary for people. Use when asked to "wrap up" or "write a wrap-up".` {
		t.Errorf("description = %q", skill.Description())
	}
	for _, rule := range []string{
		"wrap up", "write a wrap-up", "## What changed\n## Still open\n## Note to share\n",
		"nothing", "one screen", "git branch --show-current", "git status --short",
		"git log --oneline", "Make no network call", "Never guess", "secrets",
		"absolute home paths", "only read", "unless the user asks", "`handoff` skill",
	} {
		if !strings.Contains(skill.Body, rule) {
			t.Errorf("skill is missing rule %q", rule)
		}
	}
	for _, banned := range []string{"hooks/", "/wrap-up", "Claude", "Codex", "Cursor", "\u2013", "\u2014"} {
		if strings.Contains(skill.Body, banned) {
			t.Errorf("skill contains %q", banned)
		}
	}
}

func TestMaterialize_LoadsCloudHandoffAsOneSkill(t *testing.T) {
	cacheForTest(t)
	root, cleanup, err := builtins.Materialize("cloud-handoff")
	if err != nil {
		t.Fatalf("materialize cloud-handoff: %v", err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup %s: %v", root, err)
		}
	})
	var files []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == ".complete" {
			return err
		}
		rel, err := filepath.Rel(root, path)
		files = append(files, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(files, []string{"skills/cloud-handoff/SKILL.md"}) {
		t.Errorf("materialized files = %v, want only the skill", files)
	}
	bundle, err := spec.LoadLayered([]spec.Layer{{
		Name: "builtin", Root: root, Sources: config.Sources{Skills: "skills", Hooks: "hooks"},
	}})
	if err != nil {
		t.Fatalf("load builtin layer: %v", err)
	}
	if len(bundle.All()) != 1 || len(bundle.Skills) != 1 || len(bundle.Hooks) != 0 {
		t.Fatalf("builtin entries = %v, want one skill", bundle.All())
	}
	skill := bundle.Skills[0]
	if skill.Name != "cloud-handoff" || skill.Layer != "builtin" {
		t.Errorf("skill identity = %q layer %q", skill.Name, skill.Layer)
	}
	if skill.Description() != `Push the current task to a branch and continue it in a cloud session. Use when asked to "send this to a remote agent", to "run it in the cloud", or when the user is "closing my laptop".` {
		t.Errorf("description = %q", skill.Description())
	}
	for _, rule := range []string{
		// when
		"Only when the user asks", "Never at session start",
		// preflight
		"git rev-parse --is-inside-work-tree", "git remote get-url origin", "command -v",
		// branch, commit, push
		"default branch", "detached HEAD", "git status --short", "`.env`", "looks like a secret",
		"commit hook fails", "Never force-push", "git push -u origin <branch>",
		// prompt
		"## Done", "## Next step", "## Branch", "## Verify", "under 60 lines", "quoted heredoc",
		// launch table and its exact lines from decision 021
		"## Launch", "| Tool | Launch | Needs |",
		"script -q /dev/null claude --cloud \"$CLOUD_PROMPT\"",
		"script -q -c 'claude --cloud \"$CLOUD_PROMPT\"' /dev/null",
		"codex cloud exec --env \"<id>\" --branch \"<branch>\" -",
		"View: ", "kill", "requires an interactive terminal",
		"Cloud sessions need a claude.ai sign-in", "Not signed in.",
		"no cloud environments are available for this workspace", "not found; run `codex cloud`",
		"chatgpt.com/codex/settings/environments",
		// after
		"Sent to the cloud: ", "claude --teleport", "codex cloud apply", "within about a minute",
		// refusals and fallback
		"Do not work around it", "ask the user for it and stop", "`handoff` skill",
		"Use another row only when the user names that tool", "Stage only the files of this task",
		"name it in your reply", "in brackets", "Never write secrets", "stop and show the error",
		"stays theirs", "no cloud launch", "sleep 150 | script", " | codex cloud exec",
		"Set it and launch in one shell command", "cat >\"$f\" <<'PROMPT'", "pkill -P $$ -f '^sleep 150$'", "top level of the shell command", "[ -n \"$CLOUD_PROMPT\" ] || exit 1",
	} {
		if !strings.Contains(skill.Body, rule) {
			t.Errorf("skill is missing rule %q", rule)
		}
	}
	for _, banned := range []string{"hooks/", "--force", "</dev/null &", "\u2013", "\u2014"} {
		if strings.Contains(skill.Body, banned) {
			t.Errorf("skill contains %q", banned)
		}
	}
}

func TestWrapUpAndHandoff_PointToEachOther(t *testing.T) {
	cacheForTest(t)
	for name, pointer := range map[string]string{"handoff": "`wrap-up` skill", "wrap-up": "`handoff` skill"} {
		root, cleanup, err := builtins.Materialize(name)
		if err != nil {
			t.Fatalf("materialize %s: %v", name, err)
		}
		body, err := os.ReadFile(filepath.Join(root, "skills", name, "SKILL.md"))
		if cerr := cleanup(); cerr != nil {
			t.Errorf("cleanup %s: %v", root, cerr)
		}
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), pointer) {
			t.Errorf("%s skill does not point to %s", name, pointer)
		}
	}
}

func TestMaterialize_LoadsSideConversationAsOneSkill(t *testing.T) {
	cacheForTest(t)
	root, cleanup, err := builtins.Materialize("side-conversation")
	if err != nil {
		t.Fatalf("materialize side-conversation: %v", err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup %s: %v", root, err)
		}
	})
	bundle, err := spec.LoadLayered([]spec.Layer{{
		Name: "builtin", Root: root, Sources: config.Sources{Skills: "skills", Hooks: "hooks"},
	}})
	if err != nil {
		t.Fatalf("load builtin layer: %v", err)
	}
	if len(bundle.All()) != 1 || len(bundle.Skills) != 1 || len(bundle.Hooks) != 0 {
		t.Fatalf("builtin entries = %v, want one skill", bundle.All())
	}
	skill := bundle.Skills[0]
	if skill.Name != "side-conversation" || skill.Layer != "builtin" {
		t.Errorf("skill identity = %q layer %q", skill.Name, skill.Layer)
	}
	if skill.Description() != "Start a side conversation in a new terminal tab, linked to this one. Use when asked to spin a task off, or to follow something up in its own conversation." {
		t.Errorf("description = %q", skill.Description())
	}
	for _, rule := range []string{
		"Only when the user asks", "never start one on your own", "not a subagent", "delegate instead",
		"not a handoff", "## Acceptance\n## Context\n## Report back\n", "one plain sentence",
		"under 40 lines", "secrets", "Do not write the `Continues` line", "agnostic-ai conversations start <<'BRIEF'",
		"--tool", "Do not wait", "Do not work around it", "`handoff` skill", "shows under this one in the Conversations list while both are in the same group",
	} {
		if !strings.Contains(skill.Body, rule) {
			t.Errorf("skill is missing rule %q", rule)
		}
	}
	for _, banned := range []string{"hooks/", "osascript", "Claude", "Codex", "Cursor", "\u2013", "\u2014"} {
		if strings.Contains(skill.Body, banned) {
			t.Errorf("skill contains %q", banned)
		}
	}
}

func TestHandoffAndWrapUp_PointToSideConversation(t *testing.T) {
	cacheForTest(t)
	for _, name := range []string{"handoff", "wrap-up"} {
		root, cleanup, err := builtins.Materialize(name)
		if err != nil {
			t.Fatalf("materialize %s: %v", name, err)
		}
		body, err := os.ReadFile(filepath.Join(root, "skills", name, "SKILL.md"))
		if cerr := cleanup(); cerr != nil {
			t.Errorf("cleanup %s: %v", root, cerr)
		}
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "`side-conversation` skill") {
			t.Errorf("%s skill does not point to the side-conversation skill", name)
		}
	}
}

func TestSessionSkills_PointToCloudHandoff(t *testing.T) {
	cacheForTest(t)
	for name, pointer := range map[string]string{
		"handoff":           "`cloud-handoff` skill",
		"wrap-up":           "`cloud-handoff` skill",
		"side-conversation": "`cloud-handoff` skill",
		"cloud-handoff":     "`handoff` skill",
	} {
		root, cleanup, err := builtins.Materialize(name)
		if err != nil {
			t.Fatalf("materialize %s: %v", name, err)
		}
		body, err := os.ReadFile(filepath.Join(root, "skills", name, "SKILL.md"))
		if cerr := cleanup(); cerr != nil {
			t.Errorf("cleanup %s: %v", root, cerr)
		}
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), pointer) {
			t.Errorf("%s skill does not point to the %s", name, pointer)
		}
	}
}

func TestCloudHandoff_LaunchSnippetKeepsThePromptIntact(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the snippet is for macOS and Linux shells")
	}
	cacheForTest(t)
	root, cleanup, err := builtins.Materialize("cloud-handoff")
	if err != nil {
		t.Fatalf("materialize cloud-handoff: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "skills", "cloud-handoff", "SKILL.md"))
	if cerr := cleanup(); cerr != nil {
		t.Errorf("cleanup %s: %v", root, cerr)
	}
	if err != nil {
		t.Fatal(err)
	}
	blocks := bashBlocks(string(body))
	if len(blocks) < 2 {
		t.Fatalf("bash blocks = %d, want the prompt and the launch", len(blocks))
	}
	prompt := "Fix it's \"quoted\" `tick` (paren) $(date) $HOME. [cloud/x]\n\n## Done\n- done: a) lexer\n- b) parser ) ("
	start := strings.Index(blocks[0], "<<'PROMPT'\n")
	end := strings.Index(blocks[0], "\nPROMPT\n")
	if start < 0 || end < start {
		t.Fatalf("prompt block has no quoted PROMPT heredoc:\n%s", blocks[0])
	}
	setPrompt := blocks[0][:start+len("<<'PROMPT'\n")] + prompt + blocks[0][end:]
	const macLaunch = "sleep 150 | script -q /dev/null claude --cloud \"$CLOUD_PROMPT\" >\"$log\" 2>&1 &"
	if !strings.Contains(blocks[1], macLaunch) {
		t.Fatalf("launch block lacks %q:\n%s", macLaunch, blocks[1])
	}
	_, linuxTail, found := strings.Cut(string(body), "On Linux, start it with `")
	linuxLaunch, _, closed := strings.Cut(linuxTail, "`")
	if !found || !closed {
		t.Fatal("skill has no Linux launch line")
	}
	forms := map[string]string{
		"macos": blocks[1],
		"linux": strings.Replace(blocks[1], macLaunch, linuxLaunch, 1),
	}

	stubs := t.TempDir()
	got := filepath.Join(stubs, "prompt.txt")
	writeStub := func(name, script string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(stubs, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeStub("claude", "#!/bin/sh\n[ \"$1\" = --cloud ] || exit 9\nprintf '%s' \"$2\" >\"$PROMPT_OUT\"\necho 'View: https://claude.ai/code/session_test'\nexec sleep 30\n")
	writeStub("script", "#!/bin/sh\n[ \"$1\" = -q ] || exit 9\nif [ \"$2\" = -c ]; then exec sh -c \"$3\"; fi\nshift 2\nexec \"$@\"\n")

	run := 0
	for _, shell := range []string{"/bin/bash", "/bin/sh", "zsh"} {
		path, err := exec.LookPath(shell)
		if err != nil {
			continue
		}
		for _, form := range []string{"macos", "linux"} {
			run++
			// A sleep length unique to this run lets the test find its own
			// leftover input process, even beside a parallel run.
			input := fmt.Sprintf("sleep %d", 1000+(os.Getpid()%5000)*10+run)
			launch := strings.ReplaceAll(forms[form], "sleep 150", input)
			t.Run(filepath.Base(shell)+"/"+form, func(t *testing.T) {
				_ = os.Remove(got)
				cmd := exec.Command(path, "-c", setPrompt+launch)
				cmd.Env = append(os.Environ(), "PATH="+stubs+":/usr/bin:/bin", "PROMPT_OUT="+got)
				var buf bytes.Buffer
				cmd.Stdout, cmd.Stderr = &buf, &buf
				// A process left behind by the snippet holds its output open.
				cmd.WaitDelay = 5 * time.Second
				err := cmd.Run()
				out := buf.Bytes()
				if left, _ := exec.Command("pgrep", "-f", "^"+input+"$").Output(); len(left) > 0 {
					_ = exec.Command("pkill", "-f", "^"+input+"$").Run()
					t.Fatalf("the input sleep is still running after the snippet: %s", left)
				}
				if err != nil {
					t.Fatalf("snippet failed: %v\n%s", err, out)
				}
				if !strings.Contains(string(out), "View: https://claude.ai/code/session_test") {
					t.Errorf("snippet output lacks the View line:\n%s", out)
				}
				sent, err := os.ReadFile(got)
				if err != nil {
					t.Fatalf("the stand-in claude never ran: %v\n%s", err, out)
				}
				if string(sent) != prompt {
					t.Errorf("prompt arrived as\n%q\nwant\n%q", sent, prompt)
				}
			})
		}
	}
}

func bashBlocks(body string) []string {
	var blocks []string
	for _, part := range strings.Split(body, "```bash\n")[1:] {
		if end := strings.Index(part, "```"); end >= 0 {
			blocks = append(blocks, part[:end])
		}
	}
	return blocks
}
