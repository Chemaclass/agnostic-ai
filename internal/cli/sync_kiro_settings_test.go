package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const kiroCLIJSON = ".kiro/settings/cli.json"

func setupKiroSettingsProject(t *testing.T, model string) string {
	t.Helper()
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	dir := setupFixture(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	writeKiroSettingsSpec(t, dir, model)
	testutil.Chdir(t, dir)
	silence(t)
	return dir
}

func writeKiroSettingsSpec(t *testing.T, dir, model string) {
	t.Helper()
	body := "permissions:\n  allow: [\"Read\"]\n"
	if model != "" {
		body = "model: " + model + "\n"
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "settings", "project-defaults.yaml"), body)
}

func readKiroCLIJSON(t *testing.T, dir string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, kiroCLIJSON))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v\n%s", kiroCLIJSON, err, raw)
	}
	return doc
}

func TestSync_KiroSettingsModelAddThenRemoveRestoresCLIJSON(t *testing.T) {
	t.Run("no file before", func(t *testing.T) {
		dir := setupKiroSettingsProject(t, "claude-sonnet-4.6")
		if err := runSync(t); err != nil {
			t.Fatal(err)
		}
		if got := readKiroCLIJSON(t, dir); len(got) != 1 || got["chat.defaultModel"] != "claude-sonnet-4.6" {
			t.Fatalf("cli.json = %#v", got)
		}
		if err := runSync(t, "--check"); err != nil {
			t.Errorf("sync --check after sync: %v", err)
		}

		writeKiroSettingsSpec(t, dir, "")
		if err := runSync(t); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join(dir, kiroCLIJSON))
		if err == nil {
			var doc map[string]any
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatal(err)
			}
			if len(doc) != 0 {
				t.Errorf("dropping the model must leave no key behind, got %s", raw)
			}
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	})

	t.Run("file with a hand-set key", func(t *testing.T) {
		dir := setupKiroSettingsProject(t, "claude-sonnet-4.6")
		const original = "{\n  \"chat.defaultAgent\": \"mine\"\n}\n"
		mustWrite(t, filepath.Join(dir, kiroCLIJSON), original)
		if err := runSync(t); err != nil {
			t.Fatal(err)
		}
		got := readKiroCLIJSON(t, dir)
		if got["chat.defaultAgent"] != "mine" || got["chat.defaultModel"] != "claude-sonnet-4.6" {
			t.Fatalf("cli.json = %#v", got)
		}
		if err := runSync(t, "--check"); err != nil {
			t.Errorf("sync --check after sync: %v", err)
		}

		writeKiroSettingsSpec(t, dir, "")
		if err := runSync(t); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join(dir, kiroCLIJSON))
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != original {
			t.Errorf("cli.json after dropping the model:\n%s\nwant:\n%s", raw, original)
		}
	})
}

func TestSync_KiroOverwritesHandSetDefaultModel(t *testing.T) {
	dir := setupKiroSettingsProject(t, "claude-sonnet-4.6")
	mustWrite(t, filepath.Join(dir, kiroCLIJSON), "{\"chat.defaultModel\": \"auto\"}\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	if got := readKiroCLIJSON(t, dir)["chat.defaultModel"]; got != "claude-sonnet-4.6" {
		t.Errorf("chat.defaultModel = %v, want the spec value", got)
	}
}
