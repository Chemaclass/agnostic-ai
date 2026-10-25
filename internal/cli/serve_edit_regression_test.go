package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/authority"
)

func TestEditBuiltinRoundTripKeepsUnrelatedBytes(t *testing.T) {
	for _, original := range []string{
		"# mine\ntargets:\n    - claude # keep\n",
		"targets: [claude] # keep\nbuiltins: [\"memory\", 'rtk'] # list\n",
		"targets: [claude]\nbuiltins:\n    - memory # keep item\n    - rtk\n",
		"targets: [claude]\nbuiltins: [] # empty\n",
		"{targets: [claude], builtins: [rtk]}\n",
	} {
		t.Run(original, func(t *testing.T) {
			source := editHome(t)
			file := filepath.Join(source, "agnostic-ai.yaml")
			mustWriteGlobalTest(t, file, original)
			if err := os.Chmod(file, 0o600); err != nil {
				t.Fatal(err)
			}
			h := &editHandler{root: source, global: true}
			for _, enabled := range []bool{true, false} {
				_, err := h.applyValidated(file, func(data []byte) ([]byte, error) { return setBuiltin(data, "handoff", enabled) })
				if err != nil {
					t.Fatal(err)
				}
			}
			got, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != original {
				t.Errorf("round trip:\n%q\nwant:\n%q", got, original)
			}
			info, err := os.Stat(file)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
				t.Errorf("mode=%v", info.Mode())
			}
		})
	}
}

func TestEditBoolChangesOnlyItsScalar(t *testing.T) {
	for _, original := range []string{
		"# keys\nx-claude:\n    verbose: false # keep\n    text: 'false'\n",
		"x-claude: {verbose: false, text: 'keep'} # trailing\n",
		"x-claude:\r\n  verbose: false # keep\r\n",
	} {
		got, err := setBool([]byte(original), "x-claude.verbose", true)
		if err != nil {
			t.Fatal(err)
		}
		if want := strings.Replace(original, "verbose: false", "verbose: true", 1); string(got) != want {
			t.Errorf("got %q want %q", got, want)
		}
		restored, err := setBool(got, "x-claude.verbose", false)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(restored, []byte(original)) {
			t.Errorf("round trip=%q", restored)
		}
	}
}

func TestEditInheritedBooleanWritesOnlyLocalOverride(t *testing.T) {
	silence(t)
	source := editHome(t)
	shared := filepath.Join(source, "settings", "native.yaml")
	before, _ := os.ReadFile(shared)
	local := filepath.Join(source, "local", "settings", "native.yaml")
	original := "# personal\nx-claude:\n    alwaysThinkingEnabled: true # keep\n"
	mustWriteGlobalTest(t, local, original)
	w := postEdit(t, newEditHandler(source, true), editValuePath, `{"scope":"local","path":"local/settings/native.yaml","key":"x-claude.verbose","value":true}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	got, _ := os.ReadFile(shared)
	if !bytes.Equal(got, before) {
		t.Errorf("shared bytes changed: %q", got)
	}
	got, _ = os.ReadFile(local)
	if !strings.Contains(string(got), "    alwaysThinkingEnabled: true # keep\n") || !strings.Contains(string(got), "verbose: true") {
		t.Errorf("local=%q", got)
	}
	scope, err := loadCheckScope(true)
	if err != nil {
		t.Fatal(err)
	}
	if scope.bundle.Settings[0].Meta["x-claude"].(map[string]any)["verbose"] != true {
		t.Fatal("override not effective")
	}
}

func TestEditRollbackPreservesConcurrentReplacement(t *testing.T) {
	silence(t)
	source := editHome(t)
	file := filepath.Join(source, "agnostic-ai.yaml")
	h := &editHandler{root: source, global: true}
	calls := 0
	concurrent := "targets: [claude, codex]\n# concurrent save\n"
	h.validate = func() ([]string, error) {
		calls++
		if calls == 2 {
			if err := os.WriteFile(file, []byte(concurrent), 0o600); err != nil {
				t.Fatal(err)
			}
			return nil, errors.New("invalid edit")
		}
		return nil, nil
	}
	_, err := h.applyValidated(file, func(data []byte) ([]byte, error) { return setBuiltin(data, "memory", true) })
	if err == nil || strings.Contains(err.Error(), "file was restored") {
		t.Fatalf("untruthful rollback: %v", err)
	}
	got, _ := os.ReadFile(file)
	if string(got) != concurrent {
		t.Fatalf("concurrent edit lost: %q", got)
	}
}

func TestEditInterleavedBooleansKeepOtherEdits(t *testing.T) {
	source := editHome(t)
	file := filepath.Join(source, "settings", "native.yaml")
	mustWriteGlobalTest(t, file, "# mine\nx-claude:\n    verbose: false # first\n    alwaysThinkingEnabled: false # second\n")
	h := &editHandler{root: source, global: true, validate: func() ([]string, error) { return nil, nil }}
	for _, change := range []struct {
		key   string
		value bool
	}{{"verbose", true}, {"alwaysThinkingEnabled", true}, {"verbose", false}} {
		_, err := h.applyValidated(file, func(data []byte) ([]byte, error) { return setBool(data, "x-claude."+change.key, change.value) })
		if err != nil {
			t.Fatal(err)
		}
	}
	got, _ := os.ReadFile(file)
	if want := "# mine\nx-claude:\n    verbose: false # first\n    alwaysThinkingEnabled: true # second\n"; string(got) != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestEditRoundTripKeepsExternalUnrelatedSave(t *testing.T) {
	source := editHome(t)
	file := filepath.Join(source, "agnostic-ai.yaml")
	h := &editHandler{root: source, global: true}
	if _, err := h.applyValidated(file, func(data []byte) ([]byte, error) { return setBuiltin(data, "memory", true) }); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(file)
	external := append([]byte("# editor saved this\n"), got...)
	if err := os.WriteFile(file, external, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.applyValidated(file, func(data []byte) ([]byte, error) { return setBuiltin(data, "memory", false) }); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(file)
	if !bytes.HasPrefix(got, []byte("# editor saved this\n")) {
		t.Fatalf("external bytes lost: %q", got)
	}
}

func TestEditBuiltinEmptyOverrideSurvivesNewHandler(t *testing.T) {
	source := editHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude]\nbuiltins: [memory]\n")
	local := filepath.Join(source, "local", "agnostic-ai.yaml")
	mustWriteGlobalTest(t, local, "# local\nbuiltins:\n    - memory # chosen here\n")
	for _, enabled := range []string{"false", "true", "false"} {
		w := postEdit(t, newEditHandler(source, true), editBuiltinPath, `{"scope":"local","name":"memory","enabled":`+enabled+`}`, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
	names, err := loadGlobalBuiltins(source, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Fatalf("inherited builtin reenabled: %v", names)
	}
	data, _ := os.ReadFile(local)
	if !strings.Contains(string(data), "builtins: []") || !strings.Contains(string(data), "# local") {
		t.Errorf("local=%q", data)
	}
}

func TestEditRejectsIdenticalBytesInReplacementFile(t *testing.T) {
	source := editHome(t)
	file := filepath.Join(source, "agnostic-ai.yaml")
	h := &editHandler{root: source, global: true}
	_, err := h.applyValidated(file, func(data []byte) ([]byte, error) {
		other := filepath.Join(source, "editor-save.yaml")
		if err := os.WriteFile(other, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(other, file); err != nil {
			t.Fatal(err)
		}
		return setBuiltin(data, "memory", true)
	})
	if err == nil || !strings.Contains(err.Error(), "changed since") {
		t.Fatalf("replacement ignored: %v", err)
	}
	data, _ := os.ReadFile(file)
	if strings.Contains(string(data), "builtins") {
		t.Fatalf("replacement overwritten: %q", data)
	}
}

func TestEditRollsBackNewFileToAbsence(t *testing.T) {
	source := editHome(t)
	file := filepath.Join(source, "local", "agnostic-ai.yaml")
	calls := 0
	h := &editHandler{root: source, global: true, validate: func() ([]string, error) {
		calls++
		if calls == 2 {
			return []string{"conflict"}, nil
		}
		return nil, nil
	}}
	_, err := h.applyValidated(file, func(data []byte) ([]byte, error) { return setBuiltin(data, "memory", true) })
	if err == nil || !strings.Contains(err.Error(), "file was restored") {
		t.Fatalf("rollback=%v", err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new file remains: %v", err)
	}
}

func TestEditRollbackReportsRemovedParent(t *testing.T) {
	source := editHome(t)
	dir := filepath.Join(source, "isolated")
	file := filepath.Join(dir, "config.yaml")
	mustWriteGlobalTest(t, file, "targets: [claude]\n")
	calls := 0
	h := &editHandler{root: source, global: true, validate: func() ([]string, error) {
		calls++
		if calls == 2 {
			if err := os.Remove(file); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(dir); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dir, []byte("external replacement"), 0o600); err != nil {
				t.Fatal(err)
			}
			return []string{"conflict"}, nil
		}
		return nil, nil
	}}
	_, err := h.applyValidated(file, func(data []byte) ([]byte, error) { return setBuiltin(data, "memory", true) })
	if err == nil || strings.Contains(err.Error(), "file was restored") || !strings.Contains(err.Error(), "could not restore") {
		t.Fatalf("rollback=%v", err)
	}
	got, _ := os.ReadFile(dir)
	if string(got) != "external replacement" {
		t.Fatalf("external replacement lost: %q", got)
	}
}

func TestEditYAMLLayoutsKeepCommentsAndDocumentBoundaries(t *testing.T) {
	for _, original := range []string{
		"---\n# mine\ntargets: [claude]\n...\n",
		"\ufefftargets: [claude]\n",
		"targets: [claude]\nbuiltins: [rtk, # keep separator comment\n memory,] # keep end\n",
		"targets: [claude]\nbuiltins:\n  -   memory # spaces after dash\n",
		"targets: [claude]\nx-extra: |\n  keep this text\n  and this text\n# final comment\n",
	} {
		t.Run(original, func(t *testing.T) {
			source := editHome(t)
			file := filepath.Join(source, "agnostic-ai.yaml")
			mustWriteGlobalTest(t, file, original)
			h := &editHandler{root: source, global: true, validate: func() ([]string, error) { return nil, nil }}
			for _, enabled := range []bool{true, false} {
				if _, err := h.applyValidated(file, func(data []byte) ([]byte, error) { return setBuiltin(data, "handoff", enabled) }); err != nil {
					t.Fatal(err)
				}
			}
			got, _ := os.ReadFile(file)
			if string(got) != original {
				t.Errorf("got %q want %q", got, original)
			}
		})
	}
}

func TestEditInheritedBooleanAfterLiteralKeepsNextMapping(t *testing.T) {
	original := "x-claude:\n  text: |\n    keep this\n    and this\nx-codex: {} # keep next\n"
	got, err := setBoolOverride([]byte(original), "x-claude.verbose", true)
	if err != nil {
		t.Fatal(err)
	}
	want := "x-claude:\n  text: |\n    keep this\n    and this\n  verbose: true\nx-codex: {} # keep next\n"
	if string(got) != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestEditBuiltinNewFileRoundTripRestoresAbsence(t *testing.T) {
	source := editHome(t)
	file := filepath.Join(source, "local", "agnostic-ai.yaml")
	h := &editHandler{root: source, global: true, validate: func() ([]string, error) { return nil, nil }}
	for _, enabled := range []bool{true, false} {
		target, edit, err := h.builtinsEdit(scopeLocal, "memory", enabled)
		if err != nil || target != file {
			t.Fatalf("target %s: %v", target, err)
		}
		if _, err := h.applyValidated(file, edit); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new file remains: %v", err)
	}
}

func TestEditRollbackKeepsOriginalPrivateMode(t *testing.T) {
	source := editHome(t)
	file := filepath.Join(source, "settings", "native.yaml")
	original, _ := os.ReadFile(file)
	if err := os.Chmod(file, 0o600); err != nil {
		t.Fatal(err)
	}
	h := &editHandler{root: source, global: true}
	_, err := h.applyValidated(file, func([]byte) ([]byte, error) { return []byte("x-claude: [invalid\n"), nil })
	if err == nil {
		t.Fatal("invalid edit accepted")
	}
	got, _ := os.ReadFile(file)
	if !bytes.Equal(got, original) {
		t.Errorf("rollback=%q", got)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("mode=%v", info.Mode())
	}
}

func TestEditBuiltinDirectRoundTripKeepsFlowSpacing(t *testing.T) {
	for _, original := range []string{"builtins: [rtk]\n", "builtins: []\n", "builtins: [ ]\n", "builtins: [rtk,]\n", "builtins: [rtk  ] # keep\n"} {
		enabled, err := setBuiltin([]byte(original), "memory", true)
		if err != nil {
			t.Fatal(err)
		}
		disabled, err := setBuiltin(enabled, "memory", false)
		if err != nil {
			t.Fatal(err)
		}
		if string(disabled) != original {
			t.Errorf("got %q want %q", disabled, original)
		}
	}
}

func TestEditBuiltinChangesOnlyListBytes(t *testing.T) {
	original := "# keep\ntargets:\n    - claude # exact indent\nbuiltins: ['rtk'] # list\n"
	got, err := setBuiltin([]byte(original), "memory", true)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(original, "['rtk']", "['rtk', memory]", 1)
	if string(got) != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestEditInheritedFalseRemainsAfterSharedValueChanges(t *testing.T) {
	source := editHome(t)
	local := filepath.Join(source, "local", "settings", "native.yaml")
	original := "# my choice\nx-claude:\n    alwaysThinkingEnabled: true # keep\n"
	mustWriteGlobalTest(t, local, original)
	w := postEdit(t, newEditHandler(source, true), editValuePath, `{"scope":"local","path":"local/settings/native.yaml","key":"x-claude.verbose","value":false}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "native.yaml"), "x-claude:\n  verbose: true\n")
	scope, err := loadCheckScope(true)
	if err != nil {
		t.Fatal(err)
	}
	if scope.bundle.Settings[0].Meta["x-claude"].(map[string]any)["verbose"] != false {
		t.Fatal("shared change undid local false")
	}
	got, _ := os.ReadFile(local)
	if !strings.HasPrefix(string(got), original) || !strings.Contains(string(got), "verbose: false") {
		t.Errorf("local=%q", got)
	}
}

func TestEditUsesExplicitOuterOriginAllowance(t *testing.T) {
	source := editHome(t)
	h := authority.CORS(newEditHandler(source, true), "http://localhost:5173")
	w := postEdit(t, h, editBuiltinPath, `{"scope":"team","name":"memory","enabled":true}`, func(r *http.Request) { r.Header.Set("Origin", "http://localhost:5173") })
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	data, _ := os.ReadFile(filepath.Join(source, "agnostic-ai.yaml"))
	if !strings.Contains(string(data), "memory") {
		t.Fatalf("allowed request did not persist: %q", data)
	}
}

func TestEditBuiltinRefusesAnImplicitYAMLMerge(t *testing.T) {
	original := "defaults: &defaults\n  builtins: [rtk]\n<<: *defaults\n"
	if _, err := setBuiltin([]byte(original), "memory", true); err == nil {
		t.Fatal("editing merged list could discard inherited names")
	}
}

func TestEditInheritedBooleanInFlowMapWithTrailingComma(t *testing.T) {
	for _, original := range []string{
		"x-claude: {alwaysThinkingEnabled: true,}\n",
		"x-claude: {alwaysThinkingEnabled: true, } # keep end\n",
		"x-claude: {alwaysThinkingEnabled: true, # keep comment\n} # keep end\n",
	} {
		got, err := setBoolOverride([]byte(original), "x-claude.verbose", true)
		if err != nil {
			t.Errorf("%q: %v", original, err)
			continue
		}
		fragment := `"verbose": true,`
		closing := strings.IndexByte(original, '}')
		if original[closing-1] == ',' {
			fragment = " " + fragment
		}
		want := original[:closing] + fragment + original[closing:]
		if string(got) != want {
			t.Errorf("got %q want %q", got, want)
		}
	}
}

func TestEditRequiresAnExplicitBoolean(t *testing.T) {
	for _, request := range []struct{ path, body string }{
		{editBuiltinPath, `{"scope":"team","name":"memory"}`},
		{editBuiltinPath, `{"scope":"team","name":"memory","enabled":null}`},
		{editValuePath, `{"scope":"team","path":"settings/native.yaml","key":"x-claude.verbose"}`},
		{editValuePath, `{"scope":"team","path":"settings/native.yaml","key":"x-claude.verbose","value":null}`},
	} {
		t.Run(request.body, func(t *testing.T) {
			source := editHome(t)
			config := filepath.Join(source, "agnostic-ai.yaml")
			settings := filepath.Join(source, "settings", "native.yaml")
			mustWriteGlobalTest(t, config, "targets: [claude]\nbuiltins: [memory]\n")
			mustWriteGlobalTest(t, settings, "x-claude:\n  verbose: true\n")
			beforeConfig, _ := os.ReadFile(config)
			beforeSettings, _ := os.ReadFile(settings)
			w := postEdit(t, newEditHandler(source, true), request.path, request.body, nil)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status=%d body=%s", w.Code, w.Body)
			}
			afterConfig, _ := os.ReadFile(config)
			afterSettings, _ := os.ReadFile(settings)
			if !bytes.Equal(beforeConfig, afterConfig) || !bytes.Equal(beforeSettings, afterSettings) {
				t.Errorf("missing boolean changed files: config=%q settings=%q", afterConfig, afterSettings)
			}
		})
	}
}

func TestEditInheritedBooleanInTrailingCommaFlowMapIsPersisted(t *testing.T) {
	source := editHome(t)
	shared := filepath.Join(source, "settings", "native.yaml")
	before, _ := os.ReadFile(shared)
	local := filepath.Join(source, "local", "settings", "native.yaml")
	mustWriteGlobalTest(t, local, "x-claude: {alwaysThinkingEnabled: true,} # keep\n")
	w := postEdit(t, newEditHandler(source, true), editValuePath, `{"scope":"local","path":"local/settings/native.yaml","key":"x-claude.verbose","value":true}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	got, _ := os.ReadFile(shared)
	if !bytes.Equal(before, got) {
		t.Errorf("shared bytes changed: %q", got)
	}
	got, _ = os.ReadFile(local)
	if want := "x-claude: {alwaysThinkingEnabled: true, \"verbose\": true,} # keep\n"; string(got) != want {
		t.Errorf("got %q want %q", got, want)
	}
	scope, err := loadCheckScope(true)
	if err != nil {
		t.Fatal(err)
	}
	if scope.bundle.Settings[0].Meta["x-claude"].(map[string]any)["verbose"] != true {
		t.Fatal("override not effective after reload")
	}
}

func TestEditErrorsRedactInvalidConfigCredentials(t *testing.T) {
	source := editHome(t)
	token := "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	file := filepath.Join(source, "agnostic-ai.yaml")
	original := "targets: [claude]\nbuiltins: [" + token + "]\n"
	mustWriteGlobalTest(t, file, original)
	w := postEdit(t, newEditHandler(source, true), editBuiltinPath, `{"scope":"team","name":"memory","enabled":true}`, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var response editError
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error == "" || strings.Contains(w.Body.String(), token) {
		t.Errorf("credential in error response: %s", w.Body)
	}
	got, _ := os.ReadFile(file)
	if string(got) != original {
		t.Errorf("invalid source changed: %q", got)
	}
}

func TestEditErrorsRedactPathsAndValidationProblems(t *testing.T) {
	token := "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	source := editHome(t)
	path := token + "/settings.yaml"
	w := postEdit(t, newEditHandler(source, true), editValuePath, `{"scope":"team","path":"`+path+`","key":"x-claude.verbose","value":true}`, nil)
	if w.Code != http.StatusBadRequest || strings.Contains(w.Body.String(), token) {
		t.Errorf("path response=%d %s", w.Code, w.Body)
	}
	calls := 0
	h := &editHandler{root: source, global: true, validate: func() ([]string, error) {
		calls++
		if calls == 2 {
			return []string{"invalid rule \"shell(echo " + token + ")\""}, nil
		}
		return nil, nil
	}}
	w = postEdit(t, h, editValuePath, `{"scope":"team","path":"settings/native.yaml","key":"x-claude.verbose","value":true}`, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var response editError
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Problems) != 1 || strings.Contains(w.Body.String(), token) {
		t.Errorf("validation response=%s", w.Body)
	}
	got, _ := os.ReadFile(filepath.Join(source, "settings", "native.yaml"))
	if !bytes.Contains(got, []byte("verbose: false")) {
		t.Errorf("rollback did not restore: %q", got)
	}
}
