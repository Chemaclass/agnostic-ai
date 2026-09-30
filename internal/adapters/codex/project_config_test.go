package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func captureNotes(t *testing.T) *strings.Builder {
	t.Helper()
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	return buf
}

func writeConfigOverlay(t *testing.T, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(configOverlayPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configOverlayPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func decodeTopLevel(t *testing.T, body string) map[string]any {
	t.Helper()
	doc := map[string]any{}
	if _, err := toml.Decode(body, &doc); err != nil {
		t.Fatalf("emitted config.toml does not parse: %v\n%s", err, body)
	}
	return doc
}

// Codex drops notify, profiles, and model_providers from a project
// config.toml with a startup warning, so sync does not write them and
// says where they work instead (#1511).
func TestEmit_CodexConfigSkipsKeysCodexIgnoresInProject(t *testing.T) {
	dir := testutil.TempCwd(t)
	notes := captureNotes(t)
	cfg := &config.Config{Outputs: map[string]config.Output{target: {Config: &config.CodexConfig{
		Model:          "gpt-5",
		Notify:         []string{"python3", "/etc/codex/notify.py"},
		Profiles:       map[string]config.CodexProfile{"oss": {Model: "gpt-oss-20b", ModelProvider: "ollama"}},
		ModelProviders: map[string]config.CodexModelProvider{"ollama": {Name: "Ollama", BaseURL: "http://localhost:11434/v1"}},
	}}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()

	got := readFile(t, filepath.Join(dir, defaultConfigFile))
	doc := decodeTopLevel(t, got)
	if doc["model"] != "gpt-5" {
		t.Errorf("model = %v, want gpt-5:\n%s", doc["model"], got)
	}
	for _, key := range []string{"notify", "profiles", "model_providers"} {
		if _, ok := doc[key]; ok {
			t.Errorf("%s written to the project config.toml:\n%s", key, got)
		}
	}
	for _, want := range []string{
		"outputs.codex.config.notify",
		"outputs.codex.config.model-providers",
		"outputs.codex.config.profiles",
		"~/.codex/config.toml",
		"~/.codex/<name>.config.toml",
		"--profile <name>",
	} {
		if !strings.Contains(notes.String(), want) {
			t.Errorf("notes miss %q:\n%s", want, notes.String())
		}
	}
}

// With only ignored keys set there is nothing for Codex to read, so no
// project config.toml is written.
func TestEmit_CodexConfigWithOnlyIgnoredKeysWritesNoFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	captureNotes(t)
	cfg := &config.Config{Outputs: map[string]config.Output{target: {Config: &config.CodexConfig{
		Notify:   []string{"notify-send"},
		Profiles: map[string]config.CodexProfile{"work": {Model: "o4-mini"}},
	}}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, defaultConfigFile)); !os.IsNotExist(err) {
		t.Errorf("config.toml written for ignored keys alone: %v", err)
	}
}

// The captured overlay keeps its copy of an ignored key, but the key does
// not reach the project config.toml. Every other byte of the overlay,
// comments included, still does.
func TestEmit_CodexOverlayDropsKeysCodexIgnoresInProject(t *testing.T) {
	dir := testutil.TempCwd(t)
	notes := captureNotes(t)
	overlay := `# project defaults
sandbox_mode = "workspace-write"
profile = "work"
notify = [
  "terminal-notifier",
  "-title", "Codex",
]
otel.environment = "dev"
openai_base_url = "https://proxy.example/v1"

[history]
persistence = "none"

[profiles.work]
model = "gpt-5"

[model_providers.ollama]
name = "Ollama"

[otel.exporter.otlp-http]
endpoint = "https://otel.example"

[tui]
notifications = true
`
	writeConfigOverlay(t, overlay)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()

	got := readFile(t, filepath.Join(dir, defaultConfigFile))
	doc := decodeTopLevel(t, got)
	for _, key := range []string{"profile", "notify", "otel", "openai_base_url", "profiles", "model_providers"} {
		if _, ok := doc[key]; ok {
			t.Errorf("%s written to the project config.toml:\n%s", key, got)
		}
	}
	for _, want := range []string{"# project defaults", `sandbox_mode = "workspace-write"`, "[history]", "[tui]", "notifications = true"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if kept := readFile(t, configOverlayPath); kept != overlay {
		t.Errorf("sync rewrote the overlay:\n%s", kept)
	}
	for _, want := range []string{
		configOverlayPath + " sets model_providers, notify, openai_base_url, otel,",
		"~/.codex/config.toml",
		configOverlayPath + " sets profile, profiles",
		"~/.codex/<name>.config.toml",
	} {
		if !strings.Contains(notes.String(), want) {
			t.Errorf("notes miss %q:\n%s", want, notes.String())
		}
	}
}

// An overlay layout the line filter cannot cut cleanly, here a multi-line
// string inside the dropped value, still drops the ignored key: the
// filtered table is re-encoded instead.
func TestEmit_CodexOverlayReencodesWhenLinesCannotBeCut(t *testing.T) {
	dir := testutil.TempCwd(t)
	captureNotes(t)
	writeConfigOverlay(t, "model = \"gpt-5\"\nnotify = [\"\"\"\n]\n\"\"\", \"b\"]\n")
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, defaultConfigFile))
	doc := decodeTopLevel(t, got)
	if _, ok := doc["notify"]; ok {
		t.Errorf("notify written to the project config.toml:\n%s", got)
	}
	if doc["model"] != "gpt-5" {
		t.Errorf("model = %v, want gpt-5:\n%s", doc["model"], got)
	}
}

// An overlay that holds nothing but ignored keys leaves Codex nothing to
// read, so no project config.toml is written.
func TestEmit_CodexOverlayWithOnlyIgnoredKeysWritesNoFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	captureNotes(t)
	writeConfigOverlay(t, "[profiles.review]\nmodel_reasoning_effort = \"xhigh\"\n")
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, defaultConfigFile)); !os.IsNotExist(err) {
		t.Errorf("config.toml written for ignored keys alone: %v", err)
	}
}
