package cli

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type cloudVariableSetting struct {
	Name     string `json:"name"`
	Secret   bool   `json:"secret"`
	Location string `json:"location"`
}

type cloudDomainSetting struct {
	Domain   string `json:"domain"`
	Location string `json:"location"`
}

type cloudRequirementLocations struct {
	variable string
	secret   string
	domain   string
}

var cloudRequirementSettings = map[string]cloudRequirementLocations{
	"claude": {
		variable: "Edit cloud environment > Environment variables",
		secret:   "Edit cloud environment > Network secrets (HTTPS requests)",
		domain:   "Edit cloud environment > Network access > Allowed domains",
	},
	"cursor": {
		variable: "Cloud Agents > environment > Secrets > Environment Variable",
		secret:   "Cloud Agents > environment > Secrets > Runtime Secret",
		domain:   "Cloud Agents > environment > Network settings",
	},
	"copilot": {
		variable: "Settings > Secrets and variables > Agents > Variables",
		secret:   "Settings > Secrets and variables > Agents > Secrets",
		domain:   "Settings > Copilot > Internet access > Copilot in the cloud > Custom allowlist",
	},
	"codex": {
		variable: "Settings > Codex Cloud > Environments > Environment variables",
		secret:   "Settings > Codex Cloud > Environments > HTTP secrets (HTTPS requests)",
		domain:   "Settings > Codex Cloud > Environments > Public internet access > Additional allowed domains",
	},
	"openhands": {
		variable: "Settings > Secrets",
		secret:   "Settings > Secrets",
		domain:   "The sandbox's network settings",
	},
	"amp": {
		variable: "Project settings > Secrets and Environment Variables",
		secret:   "Project settings > Secrets and Environment Variables",
		domain:   "The orb runner's network settings",
	},
	"ellipsis": {
		variable: "Environments > variables",
		secret:   "Dashboard > Secrets, then list the name under x-ellipsis.environment.variables",
		domain:   "The sandbox's network settings",
	},
}

func resolvedCloudRequirementMeta(envs []spec.Entry, target string) map[string]any {
	meta := map[string]any{}
	for _, env := range envs {
		if !env.EmitsTo(target) {
			continue
		}
		resolved := adapters.ResolveMeta(env.Meta, target)
		for _, field := range []string{spec.EnvironmentVariablesField, spec.EnvironmentAllowedDomainsField, "egressAllowlist"} {
			if value, ok := resolved[field]; ok {
				meta[field] = value
			}
		}
	}
	return meta
}

func cloudEnvironmentRequirements(envs []spec.Entry, target string) ([]cloudVariableSetting, []cloudDomainSetting, bool) {
	meta := resolvedCloudRequirementMeta(envs, target)
	requirements, issues := spec.ReadEnvironmentRequirements(meta)
	locations := cloudRequirementSettings[target]
	var variables []cloudVariableSetting
	for _, variable := range requirements.Variables {
		location := locations.variable
		if variable.Secret {
			location = locations.secret
		}
		if target == "copilot" {
			upper := strings.ToUpper(variable.Name)
			switch {
			case strings.HasPrefix(upper, "GITHUB_"):
				location += " (choose a name without the reserved GITHUB_ prefix)"
			case upper != variable.Name:
				location += " (GitHub stores it as " + upper + ")"
			}
		}
		variables = append(variables, cloudVariableSetting{variable.Name, variable.Secret, location})
	}
	domainLocation := locations.domain
	if _, native := meta["egressAllowlist"]; target == "cursor" && !native {
		domainLocation = "egressAllowlist in the repository environment file, or " + domainLocation
	}
	var domains []cloudDomainSetting
	for _, domain := range requirements.AllowedDomains {
		location := domainLocation
		if target == "copilot" && strings.HasPrefix(domain, "*.") {
			location += " (enter the domain without *.)"
		}
		domains = append(domains, cloudDomainSetting{domain, location})
	}
	return variables, domains, len(issues) != 0
}
