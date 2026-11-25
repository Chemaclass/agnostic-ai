package spec

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	EnvironmentVariablesField      = "variables"
	EnvironmentAllowedDomainsField = "allowed-domains"
)

type EnvironmentVariable struct {
	Name        string
	Description string
	Secret      bool
}

type EnvironmentRequirements struct {
	Variables      []EnvironmentVariable
	AllowedDomains []string
}

// ReadEnvironmentRequirements reads names from resolved environment fields.
// An invalid list yields issues that identify fields without echoing input.
func ReadEnvironmentRequirements(meta map[string]any) (EnvironmentRequirements, []string) {
	var requirements EnvironmentRequirements
	var issues []string
	if raw, ok := meta[EnvironmentVariablesField]; ok {
		requirements.Variables, issues = readEnvironmentVariables(raw)
	}
	if raw, ok := meta[EnvironmentAllowedDomainsField]; ok {
		domains, domainIssues := readEnvironmentDomains(raw)
		requirements.AllowedDomains = domains
		issues = append(issues, domainIssues...)
	}
	return requirements, issues
}

func readEnvironmentVariables(raw any) ([]EnvironmentVariable, []string) {
	items, ok := raw.([]any)
	if !ok {
		return nil, []string{"variables must be a list of name mappings"}
	}
	variables := make([]EnvironmentVariable, 0, len(items))
	seen := make(map[string]bool, len(items))
	var issues []string
	for i, item := range items {
		variable, issue := readEnvironmentVariable(item)
		if issue == "" && seen[variable.Name] {
			issue = "duplicates an earlier name"
		}
		if issue != "" {
			issues = append(issues, fmt.Sprintf("variables entry %d %s", i+1, issue))
			continue
		}
		variables = append(variables, variable)
		seen[variable.Name] = true
	}
	if len(issues) > 0 {
		return nil, issues
	}
	return variables, nil
}

func readEnvironmentVariable(raw any) (EnvironmentVariable, string) {
	var variable EnvironmentVariable
	m, ok := raw.(map[string]any)
	if !ok {
		return variable, "must be a mapping with a name"
	}
	if _, hasValue := m["value"]; hasValue {
		return variable, "sets a value. Keep the name and any description or secret flag"
	}
	name, ok := m["name"].(string)
	if !ok || !envRefPlainPattern.MatchString(name) {
		return variable, "must have a name that starts with a letter or underscore and contains only letters, digits, and underscores"
	}
	variable.Name = name
	if raw, present := m["description"]; present {
		variable.Description, ok = raw.(string)
		if !ok {
			return variable, "must have a text description"
		}
	}
	if raw, present := m["secret"]; present {
		variable.Secret, ok = raw.(bool)
		if !ok {
			return variable, "must set secret to true or false"
		}
	}
	for key := range m {
		switch key {
		case "name", "description", "secret":
		default:
			return variable, "must use only the name, description, and secret fields"
		}
	}
	return variable, ""
}

var environmentDomainLabel = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

func readEnvironmentDomains(raw any) ([]string, []string) {
	items, ok := raw.([]any)
	if !ok {
		return nil, []string{"allowed-domains must be a list of domain names"}
	}
	domains := make([]string, 0, len(items))
	var issues []string
	for i, item := range items {
		domain, ok := item.(string)
		if !ok || !isEnvironmentDomain(domain) {
			issues = append(issues, fmt.Sprintf("allowed-domains entry %d must be a domain name with an optional *. prefix", i+1))
			continue
		}
		domains = append(domains, domain)
	}
	if len(issues) > 0 {
		return nil, issues
	}
	return domains, nil
}

func isEnvironmentDomain(domain string) bool {
	host := strings.TrimSuffix(strings.TrimPrefix(domain, "*."), ".")
	if len(host) > 253 {
		return false
	}
	for label := range strings.SplitSeq(host, ".") {
		if !environmentDomainLabel.MatchString(label) {
			return false
		}
	}
	return true
}
