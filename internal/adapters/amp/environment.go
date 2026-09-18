package amp

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

var ampServiceName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// emitEnvironment maps the portable setup and long-running process fields to
// Amp's two distinct orb lifecycle files. `.agents/resume` is deliberately not
// written: it runs after activation and every wake with thread credentials, so
// neither a one-time install nor a supervised service has equivalent semantics.
func emitEnvironment(sess *emit.Session, envs []spec.Entry, cfg *config.Config, dryRun bool) error {
	install, terminals, err := resolveEnvironment(envs)
	if err != nil {
		return err
	}
	services, err := buildServices(terminals)
	if err != nil {
		return err
	}
	if install != "" {
		path := emit.OutputSetupFile(cfg, target, defaultSetupFile)
		if err := sess.WriteExecutableFile(path, renderSetupScript(install), dryRun); err != nil {
			return err
		}
	}

	if len(services) == 0 {
		return nil
	}
	raw, err := yaml.Marshal(map[string]any{"services": services})
	if err != nil {
		return fmt.Errorf("amp services: %w", err)
	}
	path := emit.OutputEnvironmentFile(cfg, target, defaultEnvFile)
	return sess.WriteFile(path, emit.WithHeader(string(raw), emit.FormatYAML), dryRun)
}

// resolveEnvironment follows the Environment kind's established top-level
// merge rule: each declared field replaces the earlier value, including an
// explicit empty value. Target metadata is resolved before the merge.
func resolveEnvironment(envs []spec.Entry) (string, []any, error) {
	var install string
	var terminals []any
	for _, environment := range envs {
		meta := emit.ResolveMeta(environment.Meta, target)
		if value, ok := meta["install"]; ok {
			if value == nil {
				install = ""
			} else if text, valid := value.(string); valid {
				install = text
			} else {
				return "", nil, fmt.Errorf("amp environment %q: install must be a string", environment.Name)
			}
		}
		if value, ok := meta["terminals"]; ok {
			if value == nil {
				terminals = nil
			} else if entries, valid := value.([]any); valid {
				terminals = entries
			} else {
				return "", nil, fmt.Errorf("amp environment %q: terminals must be a list", environment.Name)
			}
		}
	}
	return install, terminals, nil
}

func renderSetupScript(install string) string {
	var script strings.Builder
	script.WriteString("#!/usr/bin/env bash\n")
	script.WriteString(emit.HeaderBlock(emit.FormatShell))
	script.WriteString(strings.TrimRight(install, "\n"))
	script.WriteString("\n")
	return script.String()
}

// buildServices turns each named terminal into an Amp supervised service. The
// remaining keys pass through so x-amp.terminals can use Amp's documented cwd,
// port, env, health, portal, portals, review, agent, and platforms controls
// without adding them to the portable Environment contract.
func buildServices(terminals []any) (map[string]any, error) {
	services := make(map[string]any, len(terminals))
	for index, raw := range terminals {
		terminal, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("amp environment: terminals[%d] must be a mapping", index)
		}
		name, _ := terminal["name"].(string)
		if !ampServiceName.MatchString(name) {
			return nil, fmt.Errorf("amp environment: terminals[%d].name %q must match %s", index, name, ampServiceName.String())
		}
		command, _ := terminal["command"].(string)
		if command == "" {
			return nil, fmt.Errorf("amp environment: terminal %q requires command", name)
		}
		if _, exists := services[name]; exists {
			return nil, fmt.Errorf("amp environment: duplicate terminal name %q", name)
		}
		service := make(map[string]any, len(terminal)-1)
		for key, value := range terminal {
			if key != "name" {
				service[key] = value
			}
		}
		services[name] = service
	}
	return services, nil
}
