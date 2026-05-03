package claude

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "claude" }

func (a *Adapter) Emit(entries []spec.Entry, cfg *config.Config, dryRun bool) error {
	out := outputDir(cfg)

	for _, e := range spec.Filter(entries, spec.KindAgent) {
		path := filepath.Join(out, "agents", e.Name+".md")
		if err := emit.WriteFile(path, emit.Frontmatter(e.Meta)+"\n"+e.Body, dryRun); err != nil {
			return err
		}
	}

	for _, e := range spec.Filter(entries, spec.KindSkill) {
		path := filepath.Join(out, "skills", e.Name, "SKILL.md")
		if err := emit.WriteFile(path, emit.Frontmatter(e.Meta)+"\n"+e.Body, dryRun); err != nil {
			return err
		}
	}

	rules := spec.Filter(entries, spec.KindRule)
	if len(rules) > 0 {
		var b strings.Builder
		for _, r := range rules {
			b.WriteString("## " + r.Name + "\n\n" + r.Body + "\n\n")
		}
		if err := emit.WriteFile(rulesFile(cfg), b.String(), dryRun); err != nil {
			return err
		}
	}

	hooks := spec.Filter(entries, spec.KindHook)
	if len(hooks) > 0 {
		settings := buildHookSettings(hooks)
		data, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return err
		}
		if err := emit.WriteFile(filepath.Join(out, "settings.json"), string(data)+"\n", dryRun); err != nil {
			return err
		}
	}

	return nil
}

func buildHookSettings(hooks []spec.Entry) map[string]any {
	byEvent := map[string][]map[string]any{}
	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		matcher, _ := h.Meta["matcher"].(string)
		command, _ := h.Meta["command"].(string)
		if event == "" {
			continue
		}
		byEvent[event] = append(byEvent[event], map[string]any{
			"matcher": matcher,
			"hooks": []map[string]any{
				{"type": "command", "command": command},
			},
		})
	}
	return map[string]any{"hooks": byEvent}
}

func outputDir(cfg *config.Config) string {
	if o, ok := cfg.Outputs["claude"]; ok && o.Dir != "" {
		return o.Dir
	}
	return ".claude"
}

func rulesFile(cfg *config.Config) string {
	if o, ok := cfg.Outputs["claude"]; ok && o.RulesFile != "" {
		return o.RulesFile
	}
	return "CLAUDE.md"
}
