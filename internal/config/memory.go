package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/gitquery"
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

func inheritPersonalMemory(cfg *Config, root string) (string, error) {
	layout, err := gitquery.LayoutAt(root)
	if err != nil {
		return "", nil
	}
	gitDir, common := strings.TrimSpace(layout.GitDir), strings.TrimSpace(layout.CommonDir)
	if gitDir == common || filepath.Base(common) != ".git" {
		return "", nil
	}
	path := filepath.Join(filepath.Dir(common), filepath.FromSlash(strings.TrimSpace(layout.Prefix)), LocalOverrideFileName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	doc, err := readYAMLMap(path)
	if err != nil {
		return "", err
	}
	memory, _ := doc["memory"].(map[string]any)
	value, set := memory["personal"]
	if !set {
		return "", nil
	}
	mode, ok := value.(string)
	if !ok {
		return "", errs.Coded(errs.CodeConfigDecode, "%s: memory.personal must be a string", path)
	}
	if err := validateMemory(MemoryConfig{Personal: mode}, nil, "", path); err != nil {
		return "", err
	}
	cfg.Memory.Personal = mode
	return path, nil
}
