package cli

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
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
