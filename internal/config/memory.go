package config

import (
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// Values memory.personal accepts.
const (
	PersonalMemoryCheckout = "checkout"
	PersonalMemoryRepo     = "repo"
)

// PersonalMemoryModes are the values memory.personal accepts.
var PersonalMemoryModes = []string{PersonalMemoryCheckout, PersonalMemoryRepo}

// MemoryConfig tunes the memory built-in for one user.
type MemoryConfig struct {
	Personal string `yaml:"personal,omitempty" json:"personal,omitempty" jsonschema:"enum=checkout,enum=repo" jsonschema_description:"Where personal memory lives: checkout (default) keeps it in .agnostic-ai/local/memory/ of each checkout; repo keeps one store per repository under $AGNOSTIC_AI_HOME/local/memory/, shared by every worktree. Set it in agnostic-ai.local.yaml only."`
}

// RepoPersonalMemory reports whether personal memory lives in the
// per-repository store rather than in the checkout. Nil-safe.
func (c *Config) RepoPersonalMemory() bool {
	return c != nil && c.Memory.Personal == PersonalMemoryRepo
}

// validateMemory rejects an unknown memory.personal value, and the key
// in the shared config: the store path is one user's choice.
func validateMemory(m MemoryConfig, base map[string]any, basePath, source string) error {
	if m.Personal != "" && !slices.Contains(PersonalMemoryModes, m.Personal) {
		return errs.Coded(errs.CodeConfigDecode, "%s: memory.personal: %q is not one of %s",
			source, m.Personal, strings.Join(PersonalMemoryModes, ", "))
	}
	if memory, ok := base["memory"].(map[string]any); ok {
		if _, set := memory["personal"]; set {
			return errs.Coded(errs.CodeConfigDecode, "%s: memory.personal is personal; move it to %s", basePath, LocalOverrideFileName)
		}
	}
	return nil
}
