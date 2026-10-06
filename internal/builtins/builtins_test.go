package builtins_test

import (
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

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
	want := []string{"handoff"}
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
		"tool: codex | date: 2026-10-05T14:02Z | branch: feat/x | head: abc1234\n\n" +
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
		"nearest ancestor", "agnostic-ai.yaml", "agnostic.config.yaml",
	} {
		if !strings.Contains(skill.Body, rule) {
			t.Errorf("skill is missing rule %q", rule)
		}
	}
	for _, path := range []string{"rules/learnings.md", "learnings-local.md", "hooks/"} {
		if strings.Contains(skill.Body, path) {
			t.Errorf("Slice 1 skill unexpectedly names %q", path)
		}
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
		"Compare the manual header",
		"cannot hide drift",
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
