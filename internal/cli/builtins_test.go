package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func builtinProject(t *testing.T, names string) string {
	t.Helper()
	t.Cleanup(func() {
		if err := cleanupBuiltinLayers(); err != nil {
			t.Error(err)
		}
	})
	dir := testutil.TempCwd(t)
	t.Setenv(envUserGlobalRoot, t.TempDir())
	writeFile(t, filepath.Join(dir, config.ConfigFileName), "version: 1\ntargets: [claude, codex]\nbuiltins: ["+names+"]\n")
	return dir
}

func builtinCache(t *testing.T, usable bool) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("LocalAppData", filepath.Join(home, "cache"))
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if usable {
		if err := os.MkdirAll(cache, 0o755); err != nil {
			t.Fatal(err)
		}
	} else {
		writeFile(t, cache, "not a directory\n")
	}
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	t.Setenv("TEMP", temp)
	t.Setenv("TMP", temp)
}

func runBuiltinCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd("test")
	cmd.SetArgs(args)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	return out.String(), err
}

func TestBuiltins_ProjectLoadsOnlyEnabledSpecs(t *testing.T) {
	dir := builtinProject(t, "handoff")
	_, b, err := loadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Skills) != 1 || b.Skills[0].Name != "handoff" || b.Skills[0].Layer != "builtin" {
		t.Errorf("loaded skills = %+v", b.Skills)
	}
	writeFile(t, filepath.Join(dir, config.ConfigFileName), "version: 1\ntargets: [claude]\n")
	_, b, err = loadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Skills) != 0 {
		t.Errorf("opt-out loaded skills = %+v", b.Skills)
	}
}

func TestBuiltins_ProjectOverridesWithoutLintFindings(t *testing.T) {
	dir := builtinProject(t, "handoff")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "handoff.md"), "---\nname: handoff\ndescription: Project handoff\n---\nProject body.\n")
	_, b, err := loadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Skills) != 1 || b.Skills[0].Layer != "project" || !strings.Contains(b.Skills[0].Body, "Project body.") {
		t.Errorf("winning skills = %+v", b.Skills)
	}
	if len(b.Shadowed) != 0 {
		t.Errorf("override reported as duplicate: %+v", b.Shadowed)
	}
	out, err := runBuiltinCLI(t, "list")
	if err != nil || !strings.Contains(out, "skill\thandoff\tproject") {
		t.Errorf("list = %q, %v", out, err)
	}
}

func TestBuiltins_UnknownNameListsValidNames(t *testing.T) {
	dir := builtinProject(t, "nope")
	_, _, err := loadProject(dir)
	if err == nil {
		t.Fatal("unknown builtin accepted")
	}
	for _, want := range []string{"agnostic-ai.yaml", "builtins", "nope", "handoff"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("config error missing %q: %v", want, err)
		}
	}
}

func TestBuiltins_ProvenanceNeverReportsCachePaths(t *testing.T) {
	builtinProject(t, "handoff")
	if out, err := runBuiltinCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	for _, args := range [][]string{
		{"list"}, {"explain", "builtin:handoff"}, {"why", ".claude/skills/handoff/SKILL.md"}, {"status"},
	} {
		out, err := runBuiltinCLI(t, args...)
		if err != nil {
			t.Errorf("%v: %v\n%s", args, err, out)
			continue
		}
		if !strings.Contains(out, "builtin:handoff (agnostic-ai dev)") {
			t.Errorf("%v missing builtin provenance:\n%s", args, out)
		}
		if strings.Contains(out, "agnostic-ai/builtins/") || strings.Contains(out, "agnostic-ai-builtin-") {
			t.Errorf("%v leaked a cache path:\n%s", args, out)
		}
		out, err = runBuiltinCLI(t, append(args, "--json")...)
		if err != nil || !json.Valid([]byte(out)) {
			t.Errorf("%v --json: %v\n%s", args, err, out)
			continue
		}
		var value any
		if err := json.Unmarshal([]byte(out), &value); err != nil {
			t.Fatal(err)
		}
		if !containsBuiltinJSON(value) {
			t.Errorf("%v --json missing empty path, builtin layer, name, version:\n%s", args, out)
		}
	}
}

func containsBuiltinJSON(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		if b, ok := v["builtin"].(map[string]any); ok && b["name"] == "handoff" && b["version"] == "dev" && v["path"] == "" && v["layer"] == "builtin" {
			return true
		}
		for _, child := range v {
			if containsBuiltinJSON(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if containsBuiltinJSON(child) {
				return true
			}
		}
	}
	return false
}

func TestBuiltins_LintAndLSPKeepUserFindings(t *testing.T) {
	b := spec.Bundle{Skills: []spec.Entry{
		{Kind: spec.KindSkill, Name: "handoff", Layer: "builtin", Path: "builtin-source", Meta: map[string]any{}},
		{Kind: spec.KindSkill, Name: "mine", Layer: "project", Path: "user-source", Meta: map[string]any{}},
	}}
	findings := collectLintFindings([]string{"claude"}, targetsSupportingKind, b)
	foundUser := false
	for _, f := range findings {
		if f.Path == "builtin-source" {
			t.Errorf("builtin lint finding: %+v", f)
		}
		foundUser = foundUser || f.Path == "user-source"
	}
	if !foundUser {
		t.Error("user lint findings were lost")
	}
	dir := builtinProject(t, "handoff")
	path := filepath.Join(dir, ".agnostic-ai", "skills", "mine.md")
	writeFile(t, path, "---\nname: mine\n---\n")
	diagnostics := lspLinter(dir)
	if len(diagnostics[path]) == 0 {
		t.Error("LSP lost user diagnostics")
	}
	for path := range diagnostics {
		if !strings.HasPrefix(path, dir+string(filepath.Separator)) {
			t.Errorf("LSP reported builtin diagnostics on %s", path)
		}
	}
}

func TestBuiltins_InitOptsIn(t *testing.T) {
	got := renderConfig("", []string{"claude"}, false, "dev")
	if !strings.Contains(got, "builtins: [handoff]") {
		t.Errorf("init config omitted handoff:\n%s", got)
	}
}

func TestBuiltins_GlobalConfigLocalListReplacesSharedList(t *testing.T) {
	t.Cleanup(func() { _ = cleanupBuiltinLayers() })
	source := t.TempDir()
	t.Setenv(envUserGlobalRoot, source)
	writeFile(t, filepath.Join(source, config.ConfigFileName), "version: 1\nbuiltins: [handoff]\n")
	scope, err := loadSpecScope(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.bundle.Skills) != 1 || scope.bundle.Skills[0].Layer != "builtin" {
		t.Errorf("global skills = %+v", scope.bundle.Skills)
	}
	writeFile(t, filepath.Join(source, "local", config.ConfigFileName), "builtins: []\n")
	scope, err = loadSpecScope(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.bundle.Skills) != 0 {
		t.Errorf("global local opt-out skills = %+v", scope.bundle.Skills)
	}
}

func TestBuiltins_HandoffFilesAreNotSpecs(t *testing.T) {
	dir := builtinProject(t, "handoff")
	for _, name := range []string{"HANDOFF.md", "HANDOFF.auto.md"} {
		writeFile(t, filepath.Join(dir, defaultProjectUser, name), "---\nnot valid: [\n")
	}
	_, b, err := loadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.All()) != 1 {
		t.Errorf("handoff files were parsed: %+v", b.All())
	}
}

func TestBuiltins_OptOutRemovesLedgerOwnedSkills(t *testing.T) {
	dir := builtinProject(t, "handoff")
	if out, err := runBuiltinCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	writeFile(t, filepath.Join(dir, config.ConfigFileName), "version: 1\ntargets: [claude, codex]\n")
	if out, err := runBuiltinCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync after opt-out: %v\n%s", err, out)
	}
	for _, path := range []string{".claude/skills/handoff/SKILL.md", ".agents/skills/handoff/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dir, path)); !os.IsNotExist(err) {
			t.Errorf("orphan %s survived: %v", path, err)
		}
	}
}

func TestBuiltins_CommandCleansFallbackOnSuccessAndError(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "error"}[fail], func(t *testing.T) {
			dir := builtinProject(t, "handoff")
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			blocked := filepath.Join(home, "cache-file")
			t.Setenv("XDG_CACHE_HOME", blocked)
			t.Setenv("LocalAppData", blocked)
			if runtime.GOOS == "darwin" {
				blocked = filepath.Join(home, "Library", "Caches")
			}
			writeFile(t, blocked, "not a directory\n")
			cfg, err := config.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			layers, err := resolveLayers(dir, cfg)
			if err != nil || len(layers) == 0 {
				t.Fatalf("resolve layers: %v", err)
			}
			tempRoot := layers[0].Root
			if fail {
				writeFile(t, filepath.Join(dir, ".claude"), "not a directory\n")
			}
			_, err = runBuiltinCLI(t, "sync", "--gitignore=off")
			if (err != nil) != fail {
				t.Errorf("sync error = %v, want failure %v", err, fail)
			}
			if _, err := os.Stat(tempRoot); !os.IsNotExist(err) {
				t.Errorf("fallback root survived command: %s (%v)", tempRoot, err)
			}
		})
	}
}

func TestBuiltins_WatchReloadRepairsDamagedCache(t *testing.T) {
	for _, damage := range []string{"edited skill", "deleted skill", "extra file"} {
		t.Run(damage, func(t *testing.T) {
			dir := builtinProject(t, "handoff")
			builtinCache(t, true)
			silence(t)
			rule := filepath.Join(dir, ".agnostic-ai", "rules", "check.md")
			writeFile(t, rule, "---\nname: check\n---\nOriginal rule.\n")
			if err := resyncForChanges(dir, []string{"claude"}, []string{rule}, false, false, "off", 1); err != nil {
				t.Fatal(err)
			}
			_, b, err := loadProject(dir)
			if err != nil || len(b.Skills) != 1 {
				t.Fatalf("load builtin: %v, skills: %+v", err, b.Skills)
			}
			skill := b.Skills[0].Path
			original, err := os.ReadFile(skill)
			if err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(dir, ".claude", "skills", "handoff", "SKILL.md")
			emitted, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Dir(filepath.Dir(filepath.Dir(skill)))
			extra := filepath.Join(root, "unexpected.md")
			switch damage {
			case "edited skill":
				if err := os.Chmod(skill, 0o644); err != nil {
					t.Fatal(err)
				}
				writeFile(t, skill, "---\nname: handoff\ndescription: Corrupted cache\n---\nChanged body.\n")
			case "deleted skill":
				if err := os.Remove(skill); err != nil {
					t.Fatal(err)
				}
			case "extra file":
				writeFile(t, extra, "unexpected\n")
			}
			writeFile(t, rule, "---\nname: check\n---\nUpdated rule.\n")
			if err := resyncForChanges(dir, []string{"claude"}, []string{rule}, false, false, "off", 1); err != nil {
				t.Fatal(err)
			}
			out, err := os.ReadFile(output)
			if err != nil || !bytes.Equal(out, emitted) {
				t.Errorf("reload lost embedded handoff: %v\n%s", err, out)
			}
			cached, err := os.ReadFile(skill)
			if err != nil || !bytes.Equal(cached, original) {
				t.Errorf("reload did not repair cached skill: %v", err)
			}
			if _, err := os.Stat(extra); !os.IsNotExist(err) {
				t.Errorf("extra cache file survived reload: %v", err)
			}
		})
	}
}

func TestBuiltins_ReplacedFallbackIsCleaned(t *testing.T) {
	builtinProject(t, "handoff")
	builtinCache(t, false)
	first, err := materializeBuiltin("handoff", builtinOptions(nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(first, ".complete")); err != nil {
		t.Fatal(err)
	}
	second, err := materializeBuiltin("handoff", builtinOptions(nil))
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Error("damaged fallback was reused")
	}
	if err := cleanupBuiltinLayers(); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{first, second} {
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Errorf("fallback survived cleanup: %s (%v)", root, err)
		}
	}
}

func TestBuiltins_ProjectAndGlobalShareOriginWithoutNameClash(t *testing.T) {
	dir := builtinProject(t, "handoff")
	source := t.TempDir()
	t.Setenv(envUserGlobalRoot, source)
	writeFile(t, filepath.Join(source, config.ConfigFileName), "version: 1\ntargets: [claude]\nbuiltins: [handoff]\n")
	_, b, err := loadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if clashes := globalNameClashes(b, []string{"claude"}); len(clashes) != 0 {
		t.Errorf("same builtin origin reported as name clash: %+v", clashes)
	}
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "handoff.md"), "---\nname: handoff\ndescription: Custom handoff\n---\nMine.\n")
	_, b, err = loadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	clashes := globalNameClashes(b, []string{"claude"})
	if len(clashes) != 1 {
		t.Errorf("custom project override did not clash with global builtin: %+v", clashes)
		return
	}
	for _, message := range []string{clashes[0].String(), clashes[0].silenced("ignored by sync.global-name-clash")} {
		if !strings.Contains(message, "builtin:handoff (agnostic-ai dev)") || strings.Contains(filepath.ToSlash(message), "agnostic-ai/builtins/") {
			t.Errorf("clash reported a cache source: %s", message)
		}
	}
	if message := clashes[0].String(); !strings.Contains(message, "builtins") || strings.Contains(message, "rename the global one") || strings.Contains(message, "delete it") {
		t.Errorf("clash recommends editing the bundled source: %s", message)
	}
	if err := os.Remove(filepath.Join(dir, ".agnostic-ai", "skills", "handoff.md")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(source, "skills", "handoff.md"), "---\nname: handoff\ndescription: Global custom handoff\n---\nGlobal body.\n")
	_, b, err = loadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	clashes = globalNameClashes(b, []string{"claude"})
	if len(clashes) != 1 {
		t.Fatalf("project builtin did not clash with global override: %+v", clashes)
	}
	if message := clashes[0].String(); !strings.Contains(message, "builtin:handoff (agnostic-ai dev)") || strings.Contains(filepath.ToSlash(message), "agnostic-ai/builtins/") {
		t.Errorf("project builtin clash reported a cache source: %s", message)
	}
}

func TestBuiltins_NativeImportKeepsBundledSkillRegenerable(t *testing.T) {
	for _, target := range []string{"claude", "codex", "gemini", "opencode"} {
		t.Run(target, func(t *testing.T) {
			dir := builtinProject(t, "handoff")
			writeFile(t, filepath.Join(dir, config.ConfigFileName), "version: 1\ntargets: ["+target+"]\nbuiltins: [handoff]\noutputs:\n  "+target+":\n    emit-skills-as-commands: true\n")
			for _, args := range [][]string{{"sync", "--gitignore=off"}, {"import", target}} {
				if out, err := runBuiltinCLI(t, args...); err != nil {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
			}
			_, b, err := loadProject(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(b.Skills) != 1 || b.Skills[0].Layer != "builtin" {
				t.Errorf("import made builtin a user override: %+v", b.Skills)
			}
			if len(b.Commands) != 0 {
				t.Errorf("import copied a bundled skill's command mirror: %+v", b.Commands)
			}
			for _, path := range []string{"handoff.md", "handoff/SKILL.md"} {
				if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "skills", path)); !os.IsNotExist(err) {
					t.Errorf("import copied bundled source %s: %v", path, err)
				}
			}
			writeFile(t, filepath.Join(dir, config.ConfigFileName), "version: 1\ntargets: ["+target+"]\nbuiltins: []\n")
			if out, err := runBuiltinCLI(t, "sync", "--gitignore=off"); err != nil {
				t.Fatalf("opt out: %v\n%s", err, out)
			}
			_, b, err = loadProject(dir)
			if err != nil || len(b.Skills) != 0 || len(b.Commands) != 0 {
				t.Errorf("builtin survived opt-out after import: %v, skills=%+v commands=%+v", err, b.Skills, b.Commands)
			}
		})
	}
}

func TestBuiltins_NativeImportKeepsEditsToProjectOverrides(t *testing.T) {
	for _, target := range []string{"claude", "codex", "gemini"} {
		t.Run(target, func(t *testing.T) {
			dir := builtinProject(t, "handoff")
			writeFile(t, filepath.Join(dir, config.ConfigFileName), "version: 1\ntargets: ["+target+"]\nbuiltins: [handoff]\n")
			source := filepath.Join(dir, ".agnostic-ai", "skills", "handoff", "SKILL.md")
			writeFile(t, source, "---\nname: handoff\ndescription: My handoff\n---\nMy original body.\n")
			if out, err := runBuiltinCLI(t, "sync", "--gitignore=off"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			nativeDir := map[string]string{"claude": ".claude/skills", "codex": ".agents/skills", "gemini": ".gemini/skills"}[target]
			native := filepath.Join(dir, filepath.FromSlash(nativeDir), "handoff", "SKILL.md")
			data, err := os.ReadFile(native)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, native, strings.Replace(string(data), "My original body.", "My edited body.", 1))
			if out, err := runBuiltinCLI(t, "import", target); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			data, err = os.ReadFile(source)
			if err != nil || !strings.Contains(string(data), "My edited body.") {
				t.Errorf("import suppressed an actual project override: %v\n%s", err, data)
			}
		})
	}
}

func TestBuiltins_MultiSourceImportKeepsCustomCommands(t *testing.T) {
	for _, sources := range [][]string{{"gemini", "claude"}, {"opencode", "claude"}, {"gemini", "opencode"}} {
		t.Run(strings.Join(sources, "-"), func(t *testing.T) {
			dir := builtinProject(t, "handoff")
			writeFile(t, filepath.Join(dir, config.ConfigFileName), "version: 1\ntargets: ["+strings.Join(sources, ", ")+"]\nbuiltins: [handoff]\noutputs:\n  "+sources[0]+":\n    emit-skills-as-commands: true\n")
			if out, err := runBuiltinCLI(t, "sync", "--gitignore=off"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			nativeDir := map[string]string{"claude": ".claude/commands", "opencode": opencodeCommandsDir}[sources[1]]
			writeFile(t, filepath.Join(dir, filepath.FromSlash(nativeDir), "skill-handoff.md"), "---\ndescription: Custom command\n---\nMy unrelated command body.\n")
			args := append([]string{"import"}, sources...)
			var previewErr error
			preview := captureStdout(t, func() {
				_, previewErr = runBuiltinCLI(t, append(args, "--dry-run")...)
			})
			if previewErr != nil || !strings.Contains(preview, "skill-handoff.md") {
				t.Errorf("preview lost custom command: %v\n%s", previewErr, preview)
			}
			if out, err := runBuiltinCLI(t, append(args, "--overwrite")...); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			data, err := os.ReadFile(filepath.Join(dir, ".agnostic-ai", "commands", "skill-handoff.md"))
			if err != nil || !strings.Contains(string(data), "My unrelated command body.") {
				t.Errorf("later source's command disappeared: %v\n%s", err, data)
			}
		})
	}
}

func TestBuiltins_PacksOverrideAndPersonalSpecsExtend(t *testing.T) {
	dir := builtinProject(t, "handoff")
	pack := filepath.Join(t.TempDir(), "handoff-pack")
	writeFile(t, filepath.Join(pack, "skills", "handoff.md"), "---\nname: handoff\ndescription: Pack handoff\n---\nPack body.\n")
	var output bytes.Buffer
	if err := runPacksAdd(dir, pack, "", &output); err != nil {
		t.Fatal(err)
	}
	_, b, err := loadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Skills) != 1 || b.Skills[0].Layer != "pack:handoff-pack" || !strings.Contains(b.Skills[0].Body, "Pack body.") {
		t.Errorf("pack override = %+v", b.Skills)
	}
	writeFile(t, filepath.Join(dir, defaultProjectUser, "skills", "handoff.md"), "---\nname: handoff\n---\n::parent\nPersonal addition.\n")
	_, b, err = loadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Skills) != 1 || b.Skills[0].Layer != "project-user" || !strings.Contains(b.Skills[0].Body, "Pack body.\nPersonal addition.") {
		t.Errorf("personal extension = %+v", b.Skills)
	}
}

func TestBuiltins_GlobalSpecsAndPersonalOverridesWin(t *testing.T) {
	t.Cleanup(func() { _ = cleanupBuiltinLayers() })
	source := t.TempDir()
	t.Setenv(envUserGlobalRoot, source)
	writeFile(t, filepath.Join(source, config.ConfigFileName), "version: 1\nbuiltins: [handoff]\n")
	writeFile(t, filepath.Join(source, "skills", "handoff.md"), "---\nname: handoff\ndescription: Global handoff\n---\nGlobal body.\n")
	writeFile(t, filepath.Join(source, "local", "skills", "handoff.md"), "---\nname: handoff\ndescription: Personal handoff\n---\n::parent\nPersonal addition.\n")
	scope, err := loadSpecScope(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.bundle.Skills) != 1 {
		t.Fatalf("global skills = %+v", scope.bundle.Skills)
	}
	e := scope.bundle.Skills[0]
	if e.Layer != "global-local" || e.Description() != "Personal handoff" || !strings.Contains(e.Body, "Global body.\nPersonal addition.") {
		t.Errorf("global override = %+v", e)
	}
}

func TestBuiltins_MigrationOmitsBuiltinHooks(t *testing.T) {
	dir := builtinProject(t, "handoff")
	cfg, b, err := loadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	b.Hooks = []spec.Entry{{Kind: spec.KindHook, Name: "snapshot", Layer: "builtin", Path: "builtin-hook.yaml", Meta: map[string]any{"event": "PreToolUse", "matcher": "Bash", "command": "true"}}}
	hs, err := projectHookMigrationSpecs(dir, cfg, b)
	if err != nil {
		t.Fatal(err)
	}
	changes, skips, err := planPortableHooks(migrationScope{root: dir}, hs)
	if err != nil || len(changes)+len(skips) != 0 {
		t.Errorf("builtin hook migration = %+v, %+v, %v", changes, skips, err)
	}
	for _, layer := range hs.layers {
		if layer.Name == "builtin" {
			t.Error("migration kept builtin disk layer")
		}
	}
}
