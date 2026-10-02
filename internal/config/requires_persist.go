package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type requiresEdit struct {
	path, target  string
	before, after []byte
	mode          fs.FileMode
}

func PersistRequires(root, version, schema string) ([]string, error) {
	return persistRequiresWith(root, version, schema, writeRequiresFile)
}

func persistRequiresWith(root, version, schema string, write func(string, []byte, fs.FileMode) error) ([]string, error) {
	base, _, err := ResolveConfigPath(root)
	if err != nil {
		return nil, err
	}
	paths := []string{base}
	local := filepath.Join(root, LocalOverrideFileName)
	if _, err := os.Stat(local); err == nil {
		paths = append(paths, local)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s: %w", local, err)
	}
	var edits []requiresEdit
	for i, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out, err := rewriteRequires(data, version, schema, i == 0)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if bytes.Equal(data, out) {
			continue
		}
		target, err := ResolveSourceAlias(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		info, err := os.Stat(target)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for _, prior := range edits {
			priorInfo, err := os.Stat(prior.target)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", prior.path, err)
			}
			if os.SameFile(priorInfo, info) {
				return nil, fmt.Errorf("%s: shares a config target with %s", path, prior.path)
			}
		}
		edits = append(edits, requiresEdit{path: path, target: target, before: data, after: out, mode: info.Mode().Perm()})
	}
	var changed []string
	for i, edit := range edits {
		if err := write(edit.target, edit.after, edit.mode); err != nil {
			failure := fmt.Errorf("persist %s: %w", edit.path, err)
			for j := i - 1; j >= 0; j-- {
				prior := edits[j]
				if err := write(prior.target, prior.before, prior.mode); err != nil {
					failure = errors.Join(failure, fmt.Errorf("restore %s: %w", prior.path, err))
				}
			}
			return nil, failure
		}
		changed = append(changed, edit.path)
	}
	return changed, nil
}

var requiresLine = regexp.MustCompile(`^([ \t]*(?:requires|"requires"|'requires')[ \t]*:[ \t]*)(.*)$`)
var requiresSchema = regexp.MustCompile(`\$schema=(?:"[^"]*"|'[^']*'|[^ \t\r]+)`)

func rewriteRequires(data []byte, version, schema string, base bool) ([]byte, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
		return nil, errors.New("cannot update a multi-document config")
	}
	if len(doc.Content) == 0 {
		if !base {
			return rewriteRequiresSchema(data, schema, false), nil
		}
		return nil, errors.New("cannot update an empty config")
	}
	root := doc.Content[0]
	if !base && root.Kind == yaml.ScalarNode && root.Tag == "!!null" {
		return rewriteRequiresSchema(data, schema, false), nil
	}
	if root.Kind != yaml.MappingNode {
		return nil, errors.New("config must be a mapping")
	}
	var key, value *yaml.Node
	for i := 0; i < len(root.Content); i += 2 {
		candidate := root.Content[i]
		if candidate.Tag == "!!merge" {
			return nil, errors.New("cannot safely update a config with merged root keys")
		}
		if candidate.Value == "requires" {
			if key != nil {
				return nil, errors.New("cannot update duplicate requires keys")
			}
			key, value = candidate, root.Content[i+1]
		}
	}
	if key == nil && !base {
		return rewriteRequiresSchema(data, schema, false), nil
	}
	if root.Style&yaml.FlowStyle != 0 {
		return nil, errors.New("cannot safely update a flow-style config")
	}
	newline := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		newline = "\r\n"
	}
	lines := strings.Split(string(data), "\n")
	if key != nil {
		if value.Kind != yaml.ScalarNode || value.Anchor != "" || key.Anchor != "" || value.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
			return nil, errors.New("cannot safely update an anchored or non-scalar requires value")
		}
		if value.Line != key.Line && value.Tag != "!!null" {
			return nil, errors.New("cannot safely update a multiline requires scalar")
		}
		index := key.Line - 1
		if index < 0 || index >= len(lines) {
			return nil, errors.New("cannot safely update this config newline style")
		}
		line := strings.TrimSuffix(lines[index], "\r")
		match := requiresLine.FindStringSubmatchIndex(line)
		if match == nil {
			return nil, errors.New("cannot locate the requires scalar")
		}
		start := match[3]
		end, quote, err := requiresScalarEnd(line, start)
		if err != nil {
			return nil, err
		}
		if value.Tag == "!!null" {
			quote = '"'
		}
		replacement := version
		if quote != 0 {
			replacement = string(quote) + version + string(quote)
		}
		if start > 0 && line[start-1] == ':' {
			replacement = " " + replacement
		}
		if start == end && end < len(line) && line[end] == '#' {
			replacement += " "
		}
		lines[index] = line[:start] + replacement + line[end:] + strings.TrimPrefix(lines[index], line)
	} else {
		index := root.Content[0].Line - 1
		if index < 0 || index >= len(lines) {
			return nil, errors.New("cannot safely update this config newline style")
		}
		addition := "requires: \"" + version + "\"" + strings.TrimSuffix(newline, "\n")
		lines = append(lines[:index], append([]string{addition}, lines[index:]...)...)
	}
	out := rewriteRequiresSchema([]byte(strings.Join(lines, "\n")), schema, base)
	var checked struct {
		Requires string `yaml:"requires"`
	}
	if err := yaml.Unmarshal(out, &checked); err != nil {
		return nil, fmt.Errorf("parse rewritten config: %w", err)
	}
	if checked.Requires != version {
		return nil, errors.New("rewritten config does not contain the installed requires version")
	}
	return out, nil
}

func rewriteRequiresSchema(data []byte, schema string, base bool) []byte {
	newline := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		newline = "\r\n"
	}
	lines := strings.Split(string(data), "\n")
	foundSchema := false
	for i, line := range lines {
		if !strings.HasPrefix(line, "# yaml-language-server:") {
			continue
		}
		foundSchema = true
		if token := requiresSchema.FindString(line); token != "" {
			replacement := "$schema=" + schema
			old := strings.TrimPrefix(token, "$schema=")
			if old[0] == '"' || old[0] == '\'' {
				replacement = "$schema=" + string(old[0]) + schema + string(old[0])
			}
			lines[i] = strings.Replace(line, token, replacement, 1)
		} else {
			lines[i] = strings.TrimSuffix(line, "\r") + " $schema=" + schema + strings.TrimPrefix(line, strings.TrimSuffix(line, "\r"))
		}
	}
	if !foundSchema && base {
		lines = append([]string{"# yaml-language-server: $schema=" + schema + strings.TrimSuffix(newline, "\n")}, lines...)
	}
	return []byte(strings.Join(lines, "\n"))
}

func requiresScalarEnd(line string, start int) (int, byte, error) {
	if start == len(line) || line[start] == '#' {
		return start, 0, nil
	}
	quote := line[start]
	if quote == '"' || quote == '\'' {
		for i := start + 1; i < len(line); i++ {
			if quote == '"' && line[i] == '\\' {
				i++
				continue
			}
			if line[i] == quote {
				if quote == '\'' && i+1 < len(line) && line[i+1] == '\'' {
					i++
					continue
				}
				return i + 1, quote, nil
			}
		}
		return 0, 0, errors.New("cannot safely update a multiline requires scalar")
	}
	if strings.ContainsRune("!&*", rune(quote)) {
		return 0, 0, errors.New("cannot safely update a tagged or aliased requires scalar")
	}
	end := len(line)
	for i := start; i < len(line); i++ {
		if line[i] == '#' && i > start && (line[i-1] == ' ' || line[i-1] == '\t') {
			end = i
			break
		}
	}
	for end > start && (line[end-1] == ' ' || line[end-1] == '\t') {
		end--
	}
	return end, 0, nil
}

func writeRequiresFile(path string, data []byte, mode fs.FileMode) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".agnostic-ai-requires-*")
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	defer func() { _ = os.Remove(temp.Name()) }()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
