package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// settingRequest is a scalar settings edit the web flow previews: the
// same path, key, value, and expected text /api/edit/field takes.
type settingRequest struct {
	path     string
	key      string
	value    json.RawMessage
	expected json.RawMessage
}

func (r settingRequest) clearing() bool {
	return bytes.Equal(bytes.TrimSpace(r.value), []byte("null"))
}

func (r settingRequest) title(where editScope) string {
	var value string
	switch {
	case r.clearing() && where == scopeLocal:
		return "personal " + r.key + " reset to the team value"
	case r.clearing():
		return r.key + " cleared"
	case json.Unmarshal(r.value, &value) != nil:
		return r.key + " changed"
	case where == scopeLocal:
		return preview.RedactText(fmt.Sprintf("%s set to %s for you", r.key, value))
	}
	return preview.RedactText(fmt.Sprintf("%s set to %s", r.key, value))
}

// settingSourceEdit resolves the settings file and edit exactly as a field
// save does, with the same stale-value check when the page sent one.
func settingSourceEdit(root string, where editScope, req settingRequest) (string, func([]byte) ([]byte, error), error) {
	var chosen spec.Entry
	h := &editHandler{root: root}
	file, edit, err := h.fieldEdit(where, req.path, req.key, req.value, &chosen)
	var source fieldSourceError
	var nothing noPersonalValueError
	if err != nil && len(req.expected) > 0 && (errors.As(err, &source) || errors.As(err, &nothing)) {
		return "", nil, builtinChanged(fmt.Errorf("%w. This changed after the page loaded; reload to review it", err))
	}
	if err != nil {
		return "", nil, err
	}
	if len(req.expected) > 0 {
		var expected *string
		if err := json.Unmarshal(req.expected, &expected); err != nil {
			return "", nil, errors.New("expected must be the value the page showed, as text, or null when it was not set")
		}
		edit = expectFieldValue(req.key, expected, req.clearing(), inheritedFieldText(where, chosen, file, req.key), edit)
	}
	return file, edit, nil
}

// plannedSourceLayers loads planned settings bytes in place of the files
// on disk, adding the personal layer a first personal file would start.
func plannedSourceLayers(layers []spec.Layer, planned map[string][]byte) []spec.Layer {
	layers = slices.Clone(layers)
	if !slices.ContainsFunc(layers, func(l spec.Layer) bool { return l.Name == layerNameProjectUser }) {
		for path, data := range planned {
			if data != nil && strings.HasPrefix(path, filepath.Clean(defaultProjectUser)+string(filepath.Separator)) {
				layers = append(layers, spec.Layer{Name: layerNameProjectUser, Root: defaultProjectUser, Sources: defaultLayerSources(), Extends: true, IncludeRoot: "."})
				break
			}
		}
	}
	for i := range layers {
		layers[i].Files = planned
	}
	return layers
}

func plannedRead(planned map[string][]byte) func(string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		data, ok := planned[filepath.Clean(path)]
		switch {
		case !ok:
			return os.ReadFile(path)
		case data == nil:
			return nil, fmt.Errorf("%s: %w", path, fs.ErrNotExist)
		}
		return data, nil
	}
}
