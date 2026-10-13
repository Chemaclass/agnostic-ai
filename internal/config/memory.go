package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

func inheritPersonalMemory(cfg *Config, root string) (string, error) {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir", "--show-prefix").Output()
	if err != nil {
		return "", nil
	}
	parts := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(parts) != 3 || strings.TrimSpace(parts[0]) == strings.TrimSpace(parts[1]) || filepath.Base(strings.TrimSpace(parts[1])) != ".git" {
		return "", nil
	}
	path := filepath.Join(filepath.Dir(strings.TrimSpace(parts[1])), filepath.FromSlash(strings.TrimSpace(parts[2])), LocalOverrideFileName)
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
