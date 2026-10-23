package cli

import (
	"fmt"
	"strings"
)

const grokTOMLFormat = "grok-toml"
const grokModelsHeaderNewline = "header-newline"
const grokModelsContainer = "\x00grok-models-separator"

func editGrokTOML(path string, data []byte, order []string, set map[string]any, remove []string, previous map[string]any) ([]byte, any, error) {
	for _, key := range append(append([]string{}, order...), remove...) {
		if key != "models.default" {
			return nil, nil, fmt.Errorf("%s: global Grok settings support models.default only", path)
		}
	}
	lines := strings.SplitAfter(string(data), "\n")
	scan := make([]string, len(lines))
	for i, line := range lines {
		scan[i] = strings.TrimSuffix(line, "\n")
	}
	headers, err := scanTOMLHeaders(scan)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	start, end := -1, len(lines)
	for i, h := range headers {
		if len(h.key) == 1 && h.key[0] == "models" {
			start = h.line
			if i+1 < len(headers) {
				end = headers[i+1].line
			}
			break
		}
	}
	sep, owned := previous[grokModelsContainer].(string)
	if start >= 0 {
		body := strings.Join(lines[start+1:end], "")
		rootSet := map[string]any{}
		var rootOrder, rootRemove []string
		if v, ok := set["models.default"]; ok {
			rootSet["default"] = v
			rootOrder = []string{"default"}
		}
		if len(remove) > 0 {
			rootRemove = []string{"default"}
		}
		updated, err := editTOMLRoot(path, []byte(body), rootOrder, rootSet, rootRemove)
		if err != nil {
			return nil, nil, err
		}
		if sep == grokModelsHeaderNewline && len(rootRemove) > 0 && strings.TrimSpace(string(updated)) == "" {
			prefix := strings.Join(lines[:start+1], "")
			prefix = strings.TrimSuffix(prefix, "\n")
			return []byte(prefix + strings.Join(lines[end:], "")), nil, nil
		}
		if owned && sep != grokModelsHeaderNewline && len(rootRemove) > 0 && strings.TrimSpace(string(updated)) == "" {
			prefix := strings.Join(lines[:start], "")
			prefix = strings.TrimSuffix(prefix, sep)
			return []byte(prefix + strings.Join(lines[end:], "")), nil, nil
		}
		prefix := strings.Join(lines[:start+1], "")
		if len(rootSet) > 0 && !strings.HasSuffix(prefix, "\n") {
			prefix += "\n"
			sep = grokModelsHeaderNewline
			owned = true
		}
		next := prefix + string(updated) + strings.Join(lines[end:], "")
		if owned {
			return []byte(next), sep, nil
		}
		return []byte(next), nil, nil
	}
	if len(set) == 0 {
		return data, nil, nil
	}
	values, err := settingsValues(path, "toml", data)
	if err != nil {
		return nil, nil, err
	}
	if _, ok := values["models"]; ok {
		return nil, nil, fmt.Errorf("%s: models uses a dotted or inline table; rewrite it as [models] before syncing its default", path)
	}
	v := set["models.default"]
	literal, err := tomlValueText(v)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: models.default: %w", path, err)
	}
	sep = ""
	if len(data) > 0 {
		sep = "\n"
		if !strings.HasSuffix(string(data), "\n") {
			sep = "\n\n"
		}
	}
	return []byte(string(data) + sep + "[models]\ndefault = " + literal + "\n"), sep, nil
}
