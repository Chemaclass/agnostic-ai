package approvals

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// KiroAskRuleFiles lists the Kiro files that may hold permission rules
// for a session in cwd: the user permissions file, every workspace
// permissions file in the Kiro config root, since the folder name is a hash of the
// workspace that the hook cannot compute, and the agent profiles in the
// user folder and in .kiro/agents of cwd and every folder above it
// (kiro.dev/docs/permissions.md).
func KiroAskRuleFiles(root, cwd string) ([]string, error) {
	files := []string{filepath.Join(root, "settings", "permissions.yaml")}
	workspaces := filepath.Join(root, "workspace-roots")
	entries, err := readKiroRuleDir(workspaces)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			files = append(files, filepath.Join(workspaces, entry.Name(), "permissions.yaml"))
		}
	}
	dirs := []string{filepath.Join(root, "agents")}
	if filepath.IsAbs(cwd) {
		for dir := filepath.Clean(cwd); ; dir = filepath.Dir(dir) {
			dirs = append(dirs, filepath.Join(dir, ".kiro", "agents"))
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	for _, dir := range dirs {
		entries, err := readKiroRuleDir(dir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() && slices.Contains([]string{".json", ".yaml", ".yml", ".md"}, filepath.Ext(entry.Name())) {
				files = append(files, filepath.Join(dir, entry.Name()))
			}
		}
	}
	return files, nil
}

func readKiroRuleDir(path string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return entries, nil
}

// KiroHasAskRule reports whether any of files holds a rule with effect
// ask or evaluate, or lists the approve-for-me preset. A hook allow may
// answer the prompt such a rule shows, or the one Kiro's reviewer shows
// when it is unsure, and the hook cannot match Kiro's rules, so any of
// them turns the hook off. A file that exists but does not read or
// parse is an error.
func KiroHasAskRule(files []string) (bool, error) {
	for _, path := range files {
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("read %s: %w", path, err)
		}
		docs, err := kiroDocuments(path, data)
		if err != nil {
			return false, fmt.Errorf("parse %s: %w", path, err)
		}
		if slices.ContainsFunc(docs, hasAskEffect) {
			return true, nil
		}
	}
	return false, nil
}

// frontMatter is the YAML block a Markdown agent profile opens with, or
// nothing when it has none.
func frontMatter(data []byte) []byte {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return nil
	}
	head, _, _ := strings.Cut(rest, "\n---")
	return []byte(head)
}

// kiroDocuments decodes every document in a rules file or agent profile.
func kiroDocuments(path string, data []byte) ([]any, error) {
	if strings.HasSuffix(path, ".json") {
		var doc any
		err := json.Unmarshal(data, &doc)
		return []any{doc}, err
	}
	if strings.HasSuffix(path, ".md") {
		data = frontMatter(data)
	}
	var docs []any
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var doc any
		err := decoder.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
}

// hasAskEffect finds, at any depth, an effect of ask or evaluate or a
// policies list that holds approve-for-me, so a rule list under rules,
// permissions or a profile's own key counts alike.
func hasAskEffect(node any) bool {
	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			if askEntry(key, child) || hasAskEffect(child) {
				return true
			}
		}
	case map[any]any:
		for key, child := range value {
			if text, ok := key.(string); ok && askEntry(text, child) || hasAskEffect(child) {
				return true
			}
		}
	case []any:
		return slices.ContainsFunc(value, hasAskEffect)
	}
	return false
}

func askEntry(key string, value any) bool {
	switch strings.ToLower(key) {
	case "effect":
		text, _ := value.(string)
		effect := strings.ToLower(strings.TrimSpace(text))
		return effect == "ask" || effect == "evaluate"
	case "policies", "policypreset":
		presets, _ := value.([]any)
		return slices.ContainsFunc(presets, func(p any) bool {
			text, _ := p.(string)
			return strings.EqualFold(strings.TrimSpace(text), "approve-for-me")
		})
	}
	return false
}
