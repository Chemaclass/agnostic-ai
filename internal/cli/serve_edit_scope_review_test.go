package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEditField_JustMeRefusesALocalFolderLinkedIntoTeamSources(t *testing.T) {
	project := scopeProject(t)
	shared := ".agnostic-ai/settings/defaults.yaml"
	before := readOptional(t, shared)
	if err := os.MkdirAll(".agnostic-ai/local", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../settings", ".agnostic-ai/local/settings"); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	rec := postEdit(t, newEditHandler(project, false), editFieldPath, `{"scope":"local","path":"`+shared+`","key":"model","value":"opus"}`, nil)
	if rec.Code != 400 {
		t.Errorf("save = %d %s, want a refusal", rec.Code, rec.Body)
	}
	if got := readOptional(t, shared); got != before {
		t.Errorf("team spec changed through the link:\n%s", got)
	}
}

func TestEditBuiltin_JustMeRefusesAPersonalListFromAYAMLMerge(t *testing.T) {
	project := scopeProject(t)
	local := "<<: {builtins: [memory, rtk]}\n"
	mustWrite(t, "agnostic-ai.local.yaml", local)
	h := newEditHandler(project, false)
	for _, body := range []string{`{"scope":"local","name":"handoff","enabled":true}`, `{"scope":"team","name":"handoff","enabled":true}`} {
		if rec := postEdit(t, h, editBuiltinPath, body, nil); rec.Code < 400 {
			t.Errorf("%s = %d %s, want a refusal", body, rec.Code, rec.Body)
		}
	}
	if got := readOptional(t, "agnostic-ai.local.yaml"); got != local {
		t.Errorf("local = %q", got)
	}
}

func TestEditBuiltin_JustMeInALinkedWorktreeKeepsTheInheritedMemoryMode(t *testing.T) {
	silence(t)
	isolateGit(t)
	main := t.TempDir()
	git(t, main, "init", "-q")
	mustWrite(t, filepath.Join(main, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\nbuiltins: [memory]\n")
	mustWrite(t, filepath.Join(main, "agnostic-ai.local.yaml"), "memory:\n  personal: repo\n")
	git(t, main, "add", "agnostic-ai.yaml")
	git(t, main, "commit", "-qm", "base")
	linked := filepath.Join(t.TempDir(), "linked")
	git(t, main, "worktree", "add", "-q", "--detach", linked)
	testutil.Chdir(t, linked)
	rec := postEdit(t, newEditHandler(linked, false), editBuiltinPath, `{"scope":"local","name":"handoff","enabled":true}`, nil)
	if rec.Code != 200 {
		t.Fatalf("turn on = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, "agnostic-ai.local.yaml"); !strings.Contains(got, "personal: repo") || !strings.Contains(got, "handoff") {
		t.Errorf("local = %q, want the inherited memory mode kept", got)
	}
}

func TestEditValue_JustMeResetRemovesAPersonalBoolean(t *testing.T) {
	project := scopeProject(t)
	shared := ".agnostic-ai/settings/native.yaml"
	mustWrite(t, shared, "name: native\nx-claude:\n  verbose: false\n")
	overlay := ".agnostic-ai/local/settings/native.yaml"
	mustWrite(t, overlay, "name: native\nx-claude:\n  verbose: true\n  model: mine\n")
	h := newEditHandler(project, false)
	rec := postEdit(t, h, editValuePath, `{"scope":"local","path":"`+shared+`","key":"x-claude.verbose","value":null}`, nil)
	if rec.Code != 200 {
		t.Fatalf("reset = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, overlay); got != "name: native\nx-claude:\n  model: mine\n" {
		t.Errorf("overlay = %q", got)
	}

	if err := os.Remove(overlay); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"scope":"local","path":"` + shared + `","key":"x-claude.verbose","value":true}`,
		`{"scope":"local","path":"` + shared + `","key":"x-claude.verbose","value":null}`,
	} {
		if rec := postEdit(t, h, editValuePath, body, nil); rec.Code != 200 {
			t.Fatalf("%s = %d %s", body, rec.Code, rec.Body)
		}
	}
	if _, err := os.Stat(overlay); !os.IsNotExist(err) {
		t.Errorf("reset kept the created overlay: %q", readOptional(t, overlay))
	}
	if rec := postEdit(t, h, editValuePath, `{"scope":"team","path":"`+shared+`","key":"x-claude.verbose","value":null}`, nil); rec.Code != 400 {
		t.Errorf("team null = %d %s, want a refusal", rec.Code, rec.Body)
	}
}

func TestEditField_JustMeOverlayKeepsASpecNameWithAHash(t *testing.T) {
	project := scopeProject(t)
	if err := os.Remove(".agnostic-ai/settings/defaults.yaml"); err != nil {
		t.Fatal(err)
	}
	shared := ".agnostic-ai/settings/team.yaml"
	mustWrite(t, shared, "name: \"a # b\"\ndescription: Team.\nmodel: sonnet\n")
	rec := postEdit(t, newEditHandler(project, false), editFieldPath, `{"scope":"local","path":"`+shared+`","key":"model","value":"opus"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	scope, err := loadCheckScope(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.bundle.Settings) != 1 || scope.bundle.Settings[0].Name != "a # b" || scope.bundle.Settings[0].Meta["model"] != "opus" {
		t.Errorf("settings = %+v", scope.bundle.Settings)
	}
}

func TestEditBuiltin_ResetRemovesTheWholePersonalList(t *testing.T) {
	project := scopeProject(t)
	h := newEditHandler(project, false)
	if rec := postEdit(t, h, editBuiltinPath, `{"scope":"team","reset":true}`, nil); rec.Code != 400 {
		t.Errorf("team reset = %d %s, want a refusal", rec.Code, rec.Body)
	}
	mustWrite(t, "agnostic-ai.local.yaml", "builtins:\n  - memory\nmemory:\n  personal: checkout\n")
	if rec := postEdit(t, h, editBuiltinPath, `{"scope":"local","reset":true}`, nil); rec.Code != 200 {
		t.Fatalf("reset = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, "agnostic-ai.local.yaml"); got != "memory:\n  personal: checkout\n" {
		t.Errorf("local = %q", got)
	}
}

func TestEditField_TeamSaveCreatesTheSharedSpecBesideAPersonalOne(t *testing.T) {
	project := scopeProject(t)
	if err := os.Remove(".agnostic-ai/settings/defaults.yaml"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, ".agnostic-ai/local/settings/defaults.yaml", "name: defaults\ndescription: Mine.\neffort: high\n")
	rec := postEdit(t, newEditHandler(project, false), editFieldPath, `{"scope":"team","path":".agnostic-ai/local/settings/defaults.yaml","key":"model","value":"opus"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("team save = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, ".agnostic-ai/settings/defaults.yaml"); !strings.Contains(got, "model: opus") {
		t.Errorf("shared = %q", got)
	}
	if got := readOptional(t, ".agnostic-ai/local/settings/defaults.yaml"); got != "name: defaults\ndescription: Mine.\neffort: high\n" {
		t.Errorf("personal spec changed: %q", got)
	}
}

func TestBuiltinChanges_PreviewNamesItsScopeAndResetsAPersonalList(t *testing.T) {
	root := builtinChangeProject(t)
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinScopedProposal(t, h, scopeLocal, "caveman", true)
	if proposal.Scope != "local" {
		t.Errorf("scope = %q, want local", proposal.Scope)
	}
	builtinAction(t, h, builtinApplyPath, proposal.ID)
	rec := postEdit(t, h, builtinPreviewPath, `{"scope":"local","reset":true}`, nil)
	if rec.Code != 200 {
		t.Fatalf("reset preview = %d %s", rec.Code, rec.Body)
	}
	var reset builtinChangeView
	if err := json.Unmarshal(rec.Body.Bytes(), &reset); err != nil {
		t.Fatal(err)
	}
	builtinAction(t, h, builtinApplyPath, reset.ID)
	if _, err := os.Stat("agnostic-ai.local.yaml"); !os.IsNotExist(err) {
		t.Errorf("reset kept the local config: %q", readOptional(t, "agnostic-ai.local.yaml"))
	}
}

func TestEditBuiltin_ResetInALinkedWorktreeKeepsTheMemoryModeInEffect(t *testing.T) {
	for _, body := range []string{`{"scope":"local","name":"handoff","enabled":false}`, `{"scope":"local","reset":true}`} {
		t.Run(body, func(t *testing.T) {
			silence(t)
			isolateGit(t)
			main := t.TempDir()
			git(t, main, "init", "-q")
			mustWrite(t, filepath.Join(main, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\nbuiltins: [memory]\n")
			mustWrite(t, filepath.Join(main, "agnostic-ai.local.yaml"), "memory:\n  personal: repo\n")
			git(t, main, "add", "agnostic-ai.yaml")
			git(t, main, "commit", "-qm", "base")
			linked := filepath.Join(t.TempDir(), "linked")
			git(t, main, "worktree", "add", "-q", "--detach", linked)
			testutil.Chdir(t, linked)
			mustWrite(t, "agnostic-ai.local.yaml", "builtins:\n  - memory\n  - handoff\n")
			if rec := postEdit(t, newEditHandler(linked, false), editBuiltinPath, body, nil); rec.Code != 200 {
				t.Fatalf("reset = %d %s", rec.Code, rec.Body)
			}
			cfg, err := config.Load(".")
			if err != nil {
				t.Fatal(err)
			}
			if cfg.RepoPersonalMemory() || len(cfg.Builtins) != 1 {
				t.Errorf("after reset: repo memory = %v, builtins = %v, local = %q", cfg.RepoPersonalMemory(), cfg.Builtins, readOptional(t, "agnostic-ai.local.yaml"))
			}
		})
	}
}

func TestEditBuiltin_RefusesAMergedBuiltinsNull(t *testing.T) {
	project := scopeProject(t)
	local := "<<: {builtins: null}\n"
	mustWrite(t, "agnostic-ai.local.yaml", local)
	if rec := postEdit(t, newEditHandler(project, false), editBuiltinPath, `{"scope":"local","name":"handoff","enabled":true}`, nil); rec.Code < 400 {
		t.Errorf("save = %d %s, want a refusal", rec.Code, rec.Body)
	}
	if got := readOptional(t, "agnostic-ai.local.yaml"); got != local {
		t.Errorf("local = %q", got)
	}
}
