package config

import (
	"strings"
	"testing"
)

func codexSandboxConfig(value string) string {
	return "version: 1\noutputs:\n  codex:\n    config:\n      sandbox: " + value + "\n"
}

func TestLoad_AcceptsEachCodexSandboxMode(t *testing.T) {
	for _, mode := range CodexSandboxModes {
		cfg, err := Load(writeConfigFiles(t, codexSandboxConfig(mode), ""))
		if err != nil {
			t.Errorf("%s: %v", mode, err)
			continue
		}
		if got := cfg.Outputs["codex"].Config.Sandbox; got != mode {
			t.Errorf("sandbox = %q, want %q", got, mode)
		}
	}
}

// Codex reads sandbox_mode, so a value it does not take, such as the
// `workspace` the docs once showed, stops the load and names the valid
// values. A local override is checked the same way.
func TestLoad_RejectsAnUnknownCodexSandboxMode(t *testing.T) {
	for name, files := range map[string][2]string{
		"shared": {codexSandboxConfig("workspace"), ""},
		"local":  {"version: 1\n", "outputs:\n  codex:\n    config:\n      sandbox: workspace\n"},
	} {
		_, err := Load(writeConfigFiles(t, files[0], files[1]))
		if err == nil {
			t.Errorf("%s: loaded sandbox: workspace", name)
			continue
		}
		for _, want := range append([]string{"outputs.codex.config.sandbox", `"workspace"`}, CodexSandboxModes...) {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: err = %v, want it to name %s", name, err, want)
			}
		}
	}
}
