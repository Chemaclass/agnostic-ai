package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctor_SharedSettingsAfterTargetRemoval(t *testing.T) {
	bin := handoffBuildCLI(t)
	dir := t.TempDir()
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	write := func(path, content string) {
		t.Helper()
		path = filepath.Join(dir, path)
		must(t, os.MkdirAll(filepath.Dir(path), 0755))
		must(t, os.WriteFile(path, []byte(content), 0644))
	}
	run := func(args ...string) (string, error) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	write("agnostic-ai.yaml", "version: 1\ntargets: [gemini, claude]\n")
	write(".agnostic-ai/hooks/fmt.yaml", "name: fmt\nevent: AfterTool\ncommand: echo hi\n")
	if out, err := run("sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	settings := filepath.Join(dir, ".gemini/settings.json")
	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	doc["myKey"] = 1
	data, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	write(".gemini/settings.json", string(data))
	write("agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	write(".dockerignore", "")
	ledger := filepath.Join(dir, ".agnostic-ai/.sync-state")
	ledgerBefore, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatal(err)
	}
	out, err := run("doctor")
	if err == nil || !strings.Contains(out, "removes unchanged managed keys") || !strings.Contains(out, "keeps your keys") {
		t.Errorf("doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "to remove") || !strings.Contains(out, "GEMINI.md") {
		t.Errorf("doctor must still report a removable file: %s", out)
	}
	out, err = run("doctor", "--json")
	if err == nil {
		t.Error("doctor JSON must report drift")
	}
	var result struct {
		PackagingIgnore []struct {
			Path      string   `json:"path"`
			Uncovered []string `json:"uncovered"`
		} `json:"packaging_ignore"`
		Writes []struct {
			Path   string `json:"path"`
			Action string `json:"action"`
		} `json:"writes"`
	}
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&result); err != nil {
		t.Fatalf("doctor JSON: %v\n%s", err, out)
	}
	found := false
	for _, f := range result.Writes {
		if f.Path == ".gemini/settings.json" && f.Action == "stale" {
			found = true
		}
	}
	if !found {
		t.Errorf("missing stale settings: %s", out)
	}
	generatedFound := false
	for _, finding := range result.PackagingIgnore {
		if finding.Path != ".dockerignore" {
			continue
		}
		for _, path := range finding.Uncovered {
			if path == ".gemini/settings.json" {
				t.Errorf("retained user settings listed as generated: %s", out)
			}
			if path == "CLAUDE.md" {
				generatedFound = true
			}
		}
	}
	if !generatedFound {
		t.Errorf("missing generated file in packaging finding: %s", out)
	}
	after, err := os.ReadFile(settings)
	if err != nil || string(after) != string(data) {
		t.Errorf("doctor changed settings: %v", err)
	}
	ledgerAfter, err := os.ReadFile(ledger)
	if err != nil || string(ledgerAfter) != string(ledgerBefore) {
		t.Errorf("doctor changed ledger: %v", err)
	}
	if _, err := os.Stat(settings + ".bak"); !os.IsNotExist(err) {
		t.Errorf("doctor created backup: %v", err)
	}
	if out, err := run("doctor", "--fix"); err != nil {
		t.Fatalf("doctor --fix: %v\n%s", err, out)
	}
	after, err = os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	doc = nil
	if err := json.Unmarshal(after, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc) != 1 || doc["myKey"] != float64(1) {
		t.Errorf("settings after fix: %s", after)
	}
	if out, err := run("sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}
