package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/builtins"
	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Source-only edit endpoints.
const (
	editBuiltinPath = "/api/edit/builtin"
	editValuePath   = "/api/edit/value"
	editFieldPath   = "/api/edit/field"
)

// editHandler switches a built-in in the config file, or a boolean field
// in a settings spec, then validates the configuration and restores the
// file when the edit introduced a problem. root is the project root, or
// the global source root with global set.
type editHandler struct {
	root      string
	global    bool
	mu        sync.Mutex
	validate  func() ([]string, error)
	baselines map[string]editBaseline
}

func newEditHandler(root string, global bool) http.Handler {
	return &editHandler{root: root, global: global}
}

type editError struct {
	Error    string   `json:"error"`
	Problems []string `json:"problems,omitempty"`
}

func (e editError) MarshalJSON() ([]byte, error) {
	type response editError
	e.Error = preview.RedactText(e.Error)
	e.Problems = slices.Clone(e.Problems)
	for i, problem := range e.Problems {
		e.Problems[i] = preview.RedactText(problem)
	}
	return json.Marshal(response(e))
}

func writeEditJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *editHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if status, msg := editRequestGuard(r); status != 0 {
		writeEditJSON(w, status, editError{Error: msg})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "read request: " + err.Error()})
		return
	}
	var file string
	var edit func([]byte) ([]byte, error)
	var sourceGuard func() error
	var scope editScope
	switch r.URL.Path {
	case editBuiltinPath:
		var req struct {
			Scope    string          `json:"scope"`
			Target   string          `json:"target"`
			Name     string          `json:"name"`
			Enabled  *bool           `json:"enabled"`
			Reset    bool            `json:"reset"`
			Expected json.RawMessage `json:"expected"`
		}
		if err := json.Unmarshal(body, &req); err != nil || req.Reset != (req.Name == "") || req.Reset && req.Enabled != nil {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: "send a built-in name and enabled, or reset alone"})
			return
		}
		if newFieldAddress("", req.Target).target != "" {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: "built-ins apply to every tool; send target all or leave it out"})
			return
		}
		if !req.Reset && !slices.Contains(builtins.Names(), req.Name) {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: fmt.Sprintf("name must be one of %s", strings.Join(builtins.Names(), ", "))})
			return
		}
		if !req.Reset && req.Enabled == nil {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: "enabled must be a boolean"})
			return
		}
		scope, err = parseEditScope(req.Scope)
		if err == nil && len(req.Expected) > 0 {
			err = h.checkBuiltinExpected(req.Name, req.Reset, req.Expected)
		}
		if err == nil {
			file, edit, err = h.builtinsEdit(scope, builtinRequest{name: req.Name, enabled: req.Enabled != nil && *req.Enabled, reset: req.Reset})
		}
		if err == nil && len(req.Expected) > 0 {
			team, local := h.builtinsConfigFiles()
			sourceGuard, err = watchEditSources([]string{team, local}, file, req.Name)
		}
		if err == nil && len(req.Expected) > 0 {
			edit = func(data []byte) ([]byte, error) {
				if err := h.checkBuiltinExpected(req.Name, req.Reset, req.Expected); err != nil {
					return nil, err
				}
				currentFile, next, err := h.builtinsEdit(scope, builtinRequest{name: req.Name, enabled: req.Enabled != nil && *req.Enabled, reset: req.Reset})
				if err != nil {
					return nil, err
				}
				if currentFile != file {
					return nil, staleToggleError{req.Name}
				}
				return next(data)
			}
		}
		if err != nil {
			writeExpectedError(w, err)
			return
		}
	case editValuePath:
		var req struct {
			Scope    string          `json:"scope"`
			Target   string          `json:"target"`
			Path     string          `json:"path"`
			Key      string          `json:"key"`
			Value    json.RawMessage `json:"value"`
			Expected json.RawMessage `json:"expected"`
		}
		var value *bool
		if err := json.Unmarshal(body, &req); err != nil || req.Path == "" || req.Key == "" || len(req.Value) == 0 || json.Unmarshal(req.Value, &value) != nil {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: "path, key, and a boolean or null value are required"})
			return
		}
		if newFieldAddress(req.Key, req.Target).target != "" {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: "the key names the tool already; send target all or leave it out"})
			return
		}
		scope, err = parseEditScope(req.Scope)
		if err == nil && len(req.Expected) > 0 {
			err = h.checkBooleanExpected(req.Path, req.Key, req.Expected)
		}
		if err == nil {
			file, edit, err = h.settingsEdit(scope, req.Path, req.Key, value)
		}
		if err == nil && len(req.Expected) > 0 {
			var paths []string
			paths, err = h.booleanSourcePaths(req.Path)
			if err == nil {
				sourceGuard, err = watchEditSources(paths, file, req.Key)
			}
		}
		if err == nil && len(req.Expected) > 0 {
			edit = func(data []byte) ([]byte, error) {
				if err := h.checkBooleanExpected(req.Path, req.Key, req.Expected); err != nil {
					return nil, err
				}
				currentFile, next, err := h.settingsEdit(scope, req.Path, req.Key, value)
				if err != nil {
					return nil, err
				}
				if currentFile != file {
					return nil, staleToggleError{req.Key}
				}
				return next(data)
			}
		}
		if err != nil {
			writeExpectedError(w, err)
			return
		}
	case editFieldPath:
		var req struct {
			Scope    string          `json:"scope"`
			Target   string          `json:"target"`
			Path     string          `json:"path"`
			Key      string          `json:"key"`
			Value    json.RawMessage `json:"value"`
			Expected json.RawMessage `json:"expected"`
		}
		if err := json.Unmarshal(body, &req); err != nil || req.Key == "" || len(req.Value) == 0 {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: "key and value are required (null clears the field)"})
			return
		}
		var expected *string
		if len(req.Expected) > 0 && json.Unmarshal(req.Expected, &expected) != nil {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: "expected must be the value the page showed, as text, or null when it was not set"})
			return
		}
		var chosen spec.Entry
		addr := newFieldAddress(req.Key, req.Target)
		scope, err = parseEditScope(req.Scope)
		if err == nil {
			file, edit, err = h.fieldEdit(scope, req.Path, addr, req.Value, &chosen)
		}
		var source fieldSourceError
		var nothing noPersonalValueError
		var tier tierEffortError
		switch {
		case errors.As(err, &tier):
			writeEditJSON(w, http.StatusUnprocessableEntity, editError{Error: err.Error()})
			return
		case err != nil && len(req.Expected) > 0 && errors.As(err, &source):
			writeEditJSON(w, http.StatusConflict, editError{Error: err.Error() + ". This changed after the page loaded; reload to review it"})
			return
		case err != nil && len(req.Expected) > 0 && errors.As(err, &nothing):
			writeEditJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		case err != nil:
			writeEditJSON(w, http.StatusBadRequest, editError{Error: err.Error()})
			return
		}
		if len(req.Expected) > 0 {
			edit = expectFieldValue(addr, expected, bytes.Equal(bytes.TrimSpace(req.Value), []byte("null")), inheritedFieldText(scope, chosen, file, addr), edit)
		}
	default:
		writeEditJSON(w, http.StatusNotFound, editError{Error: "no edit at " + r.URL.Path})
		return
	}
	var prepare func() (func(bool) error, error)
	if scope == scopeLocal {
		prepare = func() (func(bool) error, error) {
			finish, err := h.preparePersonalOutOfGit(file)
			if err != nil {
				return nil, personalGitError{err}
			}
			return finish, nil
		}
	}
	if sourceGuard == nil {
		sourceGuard = func() error { return nil }
	}
	if problems, err := h.applyValidatedWithPrepare(file, edit, prepare, sourceGuard); err != nil {
		var protection personalGitError
		if errors.As(err, &protection) {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: err.Error()})
			return
		}
		var stale staleFieldError
		var staleValue staleToggleError
		if errors.As(err, &staleValue) {
			writeEditJSON(w, http.StatusConflict, editError{Error: err.Error()})
			return
		}
		if errors.As(err, &stale) {
			writeEditJSON(w, http.StatusConflict, editError{Error: overviewDisplay(h.root, file) + ": " + stale.Error()})
			return
		}
		writeEditJSON(w, http.StatusUnprocessableEntity, editError{Error: err.Error(), Problems: problems})
		return
	}
	writeEditJSON(w, http.StatusOK, map[string]any{"ok": true, "file": overviewDisplay(h.root, file)})
}

// Origin checks also honor origins explicitly allowed by the outer server.
func editRequestGuard(r *http.Request) (int, string) {
	if r.Method != http.MethodPost {
		return http.StatusMethodNotAllowed, "use POST"
	}
	if mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mediaType != "application/json" {
		return http.StatusUnsupportedMediaType, "send Content-Type: application/json"
	}
	if !authority.RequestAllowed(r) {
		if r.Header.Get("Origin") == "" && r.Header.Get("Sec-Fetch-Site") != "cross-site" {
			return http.StatusMisdirectedRequest, "edits are served on loopback only"
		}
		return http.StatusForbidden, "edits are accepted from the server origin only"
	}
	return 0, ""
}

// Only a boolean present in the loaded settings can gain a local override.
// A nil value resets the personal override, and removes a personal file
// that then only names its spec.
func (h *editHandler) settingsEdit(where editScope, shown, key string, value *bool) (string, func([]byte) ([]byte, error), error) {
	scope, err := loadCheckScope(h.global)
	if err != nil {
		return "", nil, err
	}
	for _, e := range scope.bundle.Settings {
		if !h.matchesShownSpec(e, shown) || e.Layer == "builtin" {
			continue
		}
		var current any = e.Meta
		for _, part := range strings.Split(key, ".") {
			m, ok := current.(map[string]any)
			if !ok {
				return "", nil, fmt.Errorf("%s is not a boolean", key)
			}
			current = m[part]
		}
		if _, ok := current.(bool); !ok {
			return "", nil, fmt.Errorf("%s is not a boolean", key)
		}
		if value == nil && (where != scopeLocal || !overview.LocalSets(e, key)) {
			return "", nil, fmt.Errorf("%s has no personal value to reset; send scope local for a key your file sets", key)
		}
		file, scaffold, err := h.scopedSettingsFile(e, key, where)
		if err != nil {
			return "", nil, err
		}
		return file, func(data []byte) ([]byte, error) {
			if value == nil {
				out, err := removeYAMLPath(data, key)
				if err != nil || string(out) == scaffold {
					return nil, err
				}
				return out, nil
			}
			if len(data) == 0 {
				data = []byte(scaffold)
			}
			return setBoolOverride(data, key, *value)
		}, nil
	}
	return "", nil, fmt.Errorf("%s is not a settings spec of this configuration", shown)
}

// validationProblems runs what validate runs and names each issue.
func (h *editHandler) validationProblems() ([]string, error) {
	if h.validate != nil {
		return h.validate()
	}
	scope, err := loadCheckScope(h.global)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, i := range scopeValidationIssues(scope) {
		out = append(out, overviewDisplay(h.root, i.Path)+": "+i.Message)
	}
	return out, nil
}

func overviewDisplay(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// yamlDocument parses data into a document whose root is a mapping; an
// empty file becomes an empty mapping.
func yamlDocument(data []byte) (*yaml.Node, *yaml.Node, error) {
	var doc yaml.Node
	if len(bytes.TrimSpace(data)) > 0 {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, nil, fmt.Errorf("parse: %w", err)
		}
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil, errors.New("the file is not a YAML mapping")
	}
	return &doc, doc.Content[0], nil
}

func encodeYAML(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func yamlMapValue(m *yaml.Node, key string) (int, *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i, m.Content[i+1]
		}
	}
	return -1, nil
}

func yamlHasKey(data []byte, key string) bool {
	_, m, err := yamlDocument(data)
	if err != nil {
		return false
	}
	_, v := yamlMapValue(m, key)
	return v != nil
}
