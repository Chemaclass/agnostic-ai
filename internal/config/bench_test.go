package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

const benchConfig = `version: 1
requires: ">=0.95.0"
builtins: [handoff, memory]

targets:
  - codebuddy
  - claude
  - codex
  - gemini
  - qwen
  - pi
  - cursor
  - copilot
  - aider
  - cline
  - windsurf
  - continue
  - zed
  - opencode
  - antigravity
  - junie
  - kiro
  - crush
  - trae
  - qoder
  - vibe
  - kimi
  - openhands
  - factory
  - kilo
  - jules
  - goose
  - augment
  - amp
  - warp

sources:
  rules: .agnostic-ai/rules
  agents: .agnostic-ai/agents
  skills: .agnostic-ai/skills

outputs:
  aider:
    file: CONVENTIONS.md
    conf-file: .aider.conf.yml
    rules-file: CONVENTIONS.md
  gemini:
    emit-skills-as-commands: true
  opencode:
    emit-skills-as-commands: true
  codex:
    nested-glob-rules: false
  zed:
    tasks-file: .zed/tasks.json

gitignore:
  enabled: true

on-unsupported: warn

sync:
  unmanaged:
    - tests/integration/fixtures/scope-union/
    - tests/integration/fixtures/rule-heading-context/
    - tests/integration/fixtures/codex-glob-context/
    - tests/integration/fixtures/windsurf-nested-globs/
`

const benchLocalConfig = `targets:
  - claude
  - codex
  - cursor
memory:
  personal: repo
outputs:
  codex:
    nested-glob-rules: true
`

func writeBenchConfig(b *testing.B, local bool) string {
	b.Helper()
	root := b.TempDir()
	if err := os.WriteFile(filepath.Join(root, ConfigFileName), []byte(benchConfig), 0o644); err != nil {
		b.Fatal(err)
	}
	if local {
		if err := os.WriteFile(filepath.Join(root, LocalOverrideFileName), []byte(benchLocalConfig), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	return root
}

// BenchmarkLoad measures one config.Load. A project without a local file
// asks Git for an inherited one; the benchmark answers for Git so it
// times the config work, not a git process.
func BenchmarkLoad(b *testing.B) {
	for _, tc := range []struct {
		name  string
		local bool
	}{{"base", false}, {"base+local", true}} {
		b.Run(tc.name, func(b *testing.B) {
			root := writeBenchConfig(b, tc.local)
			b.Cleanup(UseProjectGit(root, ProjectGit{}))
			if _, err := Load(root); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				if _, err := Load(root); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkLoadDecodeOnly is the lower bound for Load: one read and one
// typed decode, with no checks.
func BenchmarkLoadDecodeOnly(b *testing.B) {
	path := filepath.Join(writeBenchConfig(b, false), ConfigFileName)
	b.ReportAllocs()
	for b.Loop() {
		data, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		cfg := configForDecode()
		if err := yaml.Unmarshal(data, cfg); err != nil {
			b.Fatal(err)
		}
		cfg.applyDefaults()
	}
}
