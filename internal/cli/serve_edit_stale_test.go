package cli

import (
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEditValue_RejectsStaleBoolean(t *testing.T) {
	silence(t)
	source := editHome(t)
	file := filepath.Join(source, "settings", "native.yaml")
	mustWriteGlobalTest(t, file, "x-claude:\n  verbose: true\n")
	rec := postEdit(t, newEditHandler(source, true), editValuePath, `{"scope":"team","path":"settings/native.yaml","key":"x-claude.verbose","value":true,"expected":false}`, nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409: %s", rec.Code, rec.Body)
	}
	data, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(data), "verbose: true") {
		t.Errorf("changed file: %q, %v", data, err)
	}
}

func TestEditBuiltin_RejectsStaleToggle(t *testing.T) {
	silence(t)
	source := editHome(t)
	file := filepath.Join(source, "agnostic-ai.yaml")
	mustWriteGlobalTest(t, file, "targets:\n  - claude\nbuiltins:\n  - memory\n")
	rec := postEdit(t, newEditHandler(source, true), editBuiltinPath, `{"scope":"team","name":"memory","enabled":true,"expected":false}`, nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409: %s", rec.Code, rec.Body)
	}
	data, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(data), "  - memory") {
		t.Errorf("changed file: %q, %v", data, err)
	}
}

func TestEditBuiltin_RejectsStalePersonalListReset(t *testing.T) {
	silence(t)
	source := editHome(t)
	file := filepath.Join(source, "local", "agnostic-ai.yaml")
	mustWriteGlobalTest(t, file, "builtins:\n  - memory\n  - handoff\n")
	rec := postEdit(t, newEditHandler(source, true), editBuiltinPath, `{"scope":"local","reset":true,"expected":["memory"]}`, nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409: %s", rec.Code, rec.Body)
	}
	data, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(data), "  - handoff") {
		t.Errorf("changed file: %q, %v", data, err)
	}
}

func TestEditValue_RejectsStalePersonalReset(t *testing.T) {
	silence(t)
	source := editHome(t)
	file := filepath.Join(source, "local", "settings", "native.yaml")
	mustWriteGlobalTest(t, file, "name: native\nx-claude:\n  verbose: true\n")
	rec := postEdit(t, newEditHandler(source, true), editValuePath, `{"scope":"local","path":"settings/native.yaml","key":"x-claude.verbose","value":null,"expected":false}`, nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409: %s", rec.Code, rec.Body)
	}
	data, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(data), "verbose: true") {
		t.Errorf("changed file: %q, %v", data, err)
	}
}

func TestEditExpected_AcceptsMatchingValuesAndRejectsInvalidInput(t *testing.T) {
	silence(t)
	source := editHome(t)
	h := newEditHandler(source, true)
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{editValuePath, `{"scope":"team","path":"settings/native.yaml","key":"x-claude.verbose","value":true,"expected":false}`, http.StatusOK},
		{editValuePath, `{"scope":"team","path":"settings/native.yaml","key":"x-claude.verbose","value":true,"expected":"false"}`, http.StatusBadRequest},
		{editBuiltinPath, `{"scope":"team","name":"memory","enabled":true,"expected":false}`, http.StatusOK},
		{editBuiltinPath, `{"scope":"team","name":"memory","enabled":false,"expected":null}`, http.StatusBadRequest},
	} {
		rec := postEdit(t, h, tc.path, tc.body, nil)
		if rec.Code != tc.status {
			t.Errorf("%s: status = %d, want %d: %s", tc.path, rec.Code, tc.status, rec.Body)
		}
	}
}

func TestEditPersonal_RejectsTeamChangesBeforeReplacement(t *testing.T) {
	for _, kind := range []string{"builtin", "boolean"} {
		t.Run(kind, func(t *testing.T) {
			silence(t)
			source := editHome(t)
			h := &editHandler{root: source, global: true}
			var file, team string
			var edit func([]byte) ([]byte, error)
			var err error
			if kind == "builtin" {
				team, _ = h.builtinsConfigFiles()
				file, edit, err = h.builtinsEdit(scopeLocal, builtinRequest{name: "memory", enabled: true})
			} else {
				team = filepath.Join(source, "settings", "native.yaml")
				value := true
				file, edit, err = h.settingsEdit(scopeLocal, "settings/native.yaml", "x-claude.verbose", &value)
			}
			if err != nil {
				t.Fatal(err)
			}
			guard, err := watchEditSources([]string{team}, file, kind)
			if err != nil {
				t.Fatal(err)
			}
			next := edit
			edit = func(data []byte) ([]byte, error) {
				out, err := next(data)
				if err == nil {
					mustWriteGlobalTest(t, team, "# changed while editing\n")
				}
				return out, err
			}
			_, err = h.applyValidated(file, edit, guard)
			var stale staleToggleError
			if !errors.As(err, &stale) {
				t.Errorf("error = %v, want stale conflict", err)
			}
			if _, err := os.Stat(file); !os.IsNotExist(err) {
				t.Errorf("personal file exists or stat failed: %v", err)
			}
		})
	}
}

func TestEditPersonal_StaleWriteDoesNotAlterGitExclude(t *testing.T) {
	for _, kind := range []string{"builtin", "boolean"} {
		t.Run(kind, func(t *testing.T) {
			project := gitScopeProject(t)
			h := &editHandler{root: project}
			var file, team string
			var edit func([]byte) ([]byte, error)
			var err error
			if kind == "builtin" {
				team, _ = h.builtinsConfigFiles()
				file, edit, err = h.builtinsEdit(scopeLocal, builtinRequest{name: "handoff", enabled: true})
			} else {
				team = filepath.Join(project, ".agnostic-ai", "settings", "native.yaml")
				value := true
				file, edit, err = h.settingsEdit(scopeLocal, ".agnostic-ai/settings/native.yaml", "x-claude.verbose", &value)
			}
			if err != nil {
				t.Fatal(err)
			}
			before := excludeOf(t, project)
			guard, err := watchEditSources([]string{team}, file, kind)
			if err != nil {
				t.Fatal(err)
			}
			next := edit
			edit = func(data []byte) ([]byte, error) {
				out, err := next(data)
				if err == nil {
					mustWriteGlobalTest(t, team, "# changed while editing\n")
				}
				return out, err
			}
			_, err = h.applyValidatedWithPrepare(file, edit, func() (func(bool) error, error) { return h.preparePersonalOutOfGit(file) }, guard)
			var stale staleToggleError
			if !errors.As(err, &stale) {
				t.Errorf("error = %v, want stale conflict", err)
			}
			if got := excludeOf(t, project); got != before {
				t.Errorf("Git exclude changed on stale edit: before %q, after %q", before, got)
			}
			if _, err := os.Stat(file); !os.IsNotExist(err) {
				t.Errorf("personal file exists or stat failed: %v", err)
			}
		})
	}
}

func TestEditPersonal_TeamChangeDuringExcludeUpdateKeepsGitExclude(t *testing.T) {
	project := gitScopeProject(t)
	h := &editHandler{root: project}
	team, _ := h.builtinsConfigFiles()
	file, edit, err := h.builtinsEdit(scopeLocal, builtinRequest{name: "handoff", enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	before := excludeOf(t, project)
	guard, err := watchEditSources([]string{team}, file, "handoff")
	if err != nil {
		t.Fatal(err)
	}
	prepare := func() (func(bool) error, error) {
		finish, err := h.preparePersonalOutOfGit(file)
		if err != nil {
			return nil, err
		}
		mustWriteGlobalTest(t, team, "# changed during exclude update\n")
		return finish, nil
	}
	_, err = h.applyValidatedWithPrepare(file, edit, prepare, guard)
	var stale staleToggleError
	if err == nil || !errors.As(err, &stale) || !strings.Contains(err.Error(), "kept the new Git exclusion") {
		t.Errorf("error = %v, want retained Git exclusion error", err)
	}
	if got := excludeOf(t, project); got == before || !strings.Contains(got, "/agnostic-ai.local.yaml\n") {
		t.Errorf("missing retained exclusion: %q", got)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("personal file exists or stat failed: %v", err)
	}
}

func TestEditPersonal_RollbackKeepsConcurrentGitExcludeEdits(t *testing.T) {
	project := gitScopeProject(t)
	h := &editHandler{root: project}
	team, _ := h.builtinsConfigFiles()
	file, edit, err := h.builtinsEdit(scopeLocal, builtinRequest{name: "handoff", enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	guard, err := watchEditSources([]string{team}, file, "handoff")
	if err != nil {
		t.Fatal(err)
	}
	prepare := func() (func(bool) error, error) {
		finish, err := h.preparePersonalOutOfGit(file)
		if err != nil {
			return nil, err
		}
		exclude := filepath.Join(project, ".git", "info", "exclude")
		f, err := os.OpenFile(exclude, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			return nil, err
		}
		if _, err := f.WriteString("# user edit\n"); err != nil {
			_ = f.Close()
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
		mustWriteGlobalTest(t, team, "# changed during exclude update\n")
		return finish, nil
	}
	_, err = h.applyValidatedWithPrepare(file, edit, prepare, guard)
	var stale staleToggleError
	if err == nil || !errors.As(err, &stale) {
		t.Errorf("error = %v, want rollback failure", err)
	}
	got := excludeOf(t, project)
	if !strings.Contains(got, "# user edit\n") || !strings.Contains(got, "/agnostic-ai.local.yaml\n") {
		t.Errorf("concurrent Git exclude edits lost: %q", got)
	}
}

func TestEditPersonal_RollbackKeepsExclusionWhenPersonalFileAppears(t *testing.T) {
	project := gitScopeProject(t)
	h := &editHandler{root: project}
	team, _ := h.builtinsConfigFiles()
	file, edit, err := h.builtinsEdit(scopeLocal, builtinRequest{name: "handoff", enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	guard, err := watchEditSources([]string{team}, file, "handoff")
	if err != nil {
		t.Fatal(err)
	}
	prepare := func() (func(bool) error, error) {
		finish, err := h.preparePersonalOutOfGit(file)
		if err != nil {
			return nil, err
		}
		mustWriteGlobalTest(t, file, "# user created this file\n")
		mustWriteGlobalTest(t, team, "# changed during exclude update\n")
		return finish, nil
	}
	_, err = h.applyValidatedWithPrepare(file, edit, prepare, guard)
	var stale staleToggleError
	if err == nil || errors.As(err, &stale) {
		t.Errorf("error = %v, want concurrent-file error", err)
	}
	if got := excludeOf(t, project); !strings.Contains(got, "/agnostic-ai.local.yaml\n") {
		t.Errorf("missing personal Git exclusion: %q", got)
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != "# user created this file\n" {
		t.Errorf("personal file changed: %q, %v", data, err)
	}
}

func TestEditPersonal_FailedGitExclusionKeepsExclude(t *testing.T) {
	project := gitScopeProject(t)
	mustWrite(t, ".gitignore", "!/agnostic-ai.local.yaml\n")
	before := excludeOf(t, project)
	rec := postEdit(t, newEditHandler(project, false), editBuiltinPath, `{"scope":"local","name":"handoff","enabled":true,"expected":false}`, nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Git would still track") {
		t.Errorf("save = %d %s, want Git refusal", rec.Code, rec.Body)
	}
	if got := excludeOf(t, project); got == before || !strings.Contains(got, "/agnostic-ai.local.yaml\n") {
		t.Errorf("missing retained Git exclusion: %q", got)
	}
	if !strings.Contains(rec.Body.String(), "kept the new Git exclusion") {
		t.Errorf("missing retained-exclusion reason: %s", rec.Body)
	}
	if _, err := os.Stat(filepath.Join(project, "agnostic-ai.local.yaml")); !os.IsNotExist(err) {
		t.Errorf("personal file exists or stat failed: %v", err)
	}
}

func TestEditPersonal_RejectedValidationKeepsGitExclude(t *testing.T) {
	project := gitScopeProject(t)
	before := excludeOf(t, project)
	calls := 0
	h := &editHandler{root: project, validate: func() ([]string, error) {
		calls++
		if calls > 1 {
			return []string{"new validation failure"}, nil
		}
		return nil, nil
	}}
	rec := postEdit(t, h, editBuiltinPath, `{"scope":"local","name":"handoff","enabled":true,"expected":false}`, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("save = %d %s, want validation rejection", rec.Code, rec.Body)
	}
	if got := excludeOf(t, project); got == before || !strings.Contains(got, "/agnostic-ai.local.yaml\n") {
		t.Errorf("missing retained Git exclusion: %q", got)
	}
	if !strings.Contains(rec.Body.String(), "kept the new Git exclusion") {
		t.Errorf("missing retained-exclusion reason: %s", rec.Body)
	}
	if _, err := os.Stat(filepath.Join(project, "agnostic-ai.local.yaml")); !os.IsNotExist(err) {
		t.Errorf("personal file exists or stat failed: %v", err)
	}
}

func TestEditPersonal_RollbackKeepsExclusionWhenSiblingAppears(t *testing.T) {
	project := gitScopeProject(t)
	h := &editHandler{root: project}
	team := filepath.Join(project, ".agnostic-ai", "settings", "native.yaml")
	value := true
	file, edit, err := h.settingsEdit(scopeLocal, ".agnostic-ai/settings/native.yaml", "x-claude.verbose", &value)
	if err != nil {
		t.Fatal(err)
	}
	guard, err := watchEditSources([]string{team}, file, "x-claude.verbose")
	if err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(project, ".agnostic-ai", "local", "notes.md")
	prepare := func() (func(bool) error, error) {
		finish, err := h.preparePersonalOutOfGit(file)
		if err != nil {
			return nil, err
		}
		mustWriteGlobalTest(t, sibling, "private sibling\n")
		mustWriteGlobalTest(t, team, "# changed during exclude update\n")
		return finish, nil
	}
	_, err = h.applyValidatedWithPrepare(file, edit, prepare, guard)
	var stale staleToggleError
	if err == nil || !errors.As(err, &stale) {
		t.Errorf("error = %v, want rollback refusal", err)
	}
	if got := excludeOf(t, project); !strings.Contains(got, "/.agnostic-ai/local/\n") {
		t.Errorf("missing personal folder Git exclusion: %q", got)
	}
	if got, err := os.ReadFile(sibling); err != nil || string(got) != "private sibling\n" {
		t.Errorf("sibling changed: %q, %v", got, err)
	}
	if !gitIgnores(project, sibling) {
		t.Error("sibling is not ignored")
	}
}

func TestEditPersonal_HTTPConflictAfterGitExclusion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Git interception uses a POSIX executable")
	}
	for _, kind := range []string{"builtin", "boolean"} {
		t.Run(kind, func(t *testing.T) {
			project := gitScopeProject(t)
			gitPath, err := exec.LookPath("git")
			if err != nil {
				t.Fatal(err)
			}
			var endpoint, body, team, file, pattern string
			if kind == "builtin" {
				endpoint, body = editBuiltinPath, `{"scope":"local","name":"handoff","enabled":true,"expected":false}`
				team = filepath.Join(project, "agnostic-ai.yaml")
				file = filepath.Join(project, "agnostic-ai.local.yaml")
				pattern = "/agnostic-ai.local.yaml\n"
			} else {
				endpoint, body = editValuePath, `{"scope":"local","path":".agnostic-ai/settings/native.yaml","key":"x-claude.verbose","value":true,"expected":false}`
				team = filepath.Join(project, ".agnostic-ai", "settings", "native.yaml")
				file = filepath.Join(project, ".agnostic-ai", "local", "settings", "native.yaml")
				pattern = "/.agnostic-ai/local/\n"
			}
			dir := t.TempDir()
			script := `#!/bin/sh
"$AGNOSTIC_TEST_REAL_GIT" "$@"
result=$?
if [ "$2" = "check-ignore" ] && [ "$result" -eq 0 ] && [ ! -e "$AGNOSTIC_TEST_MARKER" ]; then
  printf '# changed during Git protection\n' > "$AGNOSTIC_TEST_SOURCE"
  : > "$AGNOSTIC_TEST_MARKER"
fi
exit "$result"
`
			if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("AGNOSTIC_TEST_REAL_GIT", gitPath)
			t.Setenv("AGNOSTIC_TEST_SOURCE", team)
			t.Setenv("AGNOSTIC_TEST_MARKER", filepath.Join(dir, "changed"))
			rec := postEdit(t, newEditHandler(project, false), endpoint, body, nil)
			if rec.Code != http.StatusConflict {
				t.Errorf("status = %d, want 409: %s", rec.Code, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), "changed after this page loaded") || !strings.Contains(rec.Body.String(), "kept the new Git exclusion") {
				t.Errorf("missing conflict or retained-exclusion reason: %s", rec.Body)
			}
			if got := excludeOf(t, project); !strings.Contains(got, pattern) {
				t.Errorf("missing retained Git exclusion: %q", got)
			}
			if _, err := os.Stat(file); !os.IsNotExist(err) {
				t.Errorf("personal file exists or stat failed: %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, "changed")); err != nil {
				t.Errorf("the Git interception did not run: %v", err)
			}
		})
	}
}
