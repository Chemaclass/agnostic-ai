package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/builtins"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/overview"
)

// Edit endpoints serve mounts. They are the only API calls that write.
const (
	editBuiltinPath = "/api/edit/builtin"
	editValuePath   = "/api/edit/value"
)

// editHandler switches a built-in in the config file, or a boolean field
// in a settings spec, then validates the configuration and restores the
// file when the edit introduced a problem. root is the project root, or
// the global source root with global set.
type editHandler struct {
	root   string
	global bool
}

func newEditHandler(root string, global bool) http.Handler {
	return &editHandler{root: root, global: global}
}

type editError struct {
	Error    string   `json:"error"`
	Problems []string `json:"problems,omitempty"`
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
	switch r.URL.Path {
	case editBuiltinPath:
		var req struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		}
		if err := json.Unmarshal(body, &req); err != nil || !slices.Contains(builtins.Names(), req.Name) {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: fmt.Sprintf("name must be one of %s", strings.Join(builtins.Names(), ", "))})
			return
		}
		file = h.builtinsFile()
		edit = func(data []byte) ([]byte, error) { return setBuiltin(data, req.Name, req.Enabled) }
	case editValuePath:
		var req struct {
			Path  string `json:"path"`
			Key   string `json:"key"`
			Value bool   `json:"value"`
		}
		if err := json.Unmarshal(body, &req); err != nil || req.Path == "" || req.Key == "" {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: "path and key are required"})
			return
		}
		file, err = h.settingsFile(req.Path)
		if err != nil {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: err.Error()})
			return
		}
		edit = func(data []byte) ([]byte, error) { return setBool(data, req.Key, req.Value) }
	default:
		writeEditJSON(w, http.StatusNotFound, editError{Error: "no edit at " + r.URL.Path})
		return
	}
	if problems, err := h.applyValidated(file, edit); err != nil {
		writeEditJSON(w, http.StatusUnprocessableEntity, editError{Error: err.Error(), Problems: problems})
		return
	}
	writeEditJSON(w, http.StatusOK, map[string]any{"ok": true, "file": overviewDisplay(h.root, file)})
}

// editRequestGuard refuses any request a page on another site could send: it
// must be a POST with a JSON body, to a loopback Host, from a local
// Origin when one is given. A JSON body needs a CORS preflight, which
// serve answers only for local origins.
func editRequestGuard(r *http.Request) (int, string) {
	if r.Method != http.MethodPost {
		return http.StatusMethodNotAllowed, "use POST"
	}
	if mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mediaType != "application/json" {
		return http.StatusUnsupportedMediaType, "send Content-Type: application/json"
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return http.StatusMisdirectedRequest, "edits are served on loopback only"
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "http" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
			return http.StatusForbidden, "edits are accepted from local pages only"
		}
	}
	return 0, ""
}

// builtinsFile is the config file whose builtins list is in effect: the
// local layer's when it sets builtins, else the shared one.
func (h *editHandler) builtinsFile() string {
	var shared, local string
	if h.global {
		paths := globalConfigPaths(h.root)
		shared, local = paths[0], paths[1]
	} else {
		shared = filepath.Join(h.root, config.ConfigFileName)
		if path, _, err := config.ResolveConfigPath(h.root); err == nil {
			shared = path
		}
		local = filepath.Join(h.root, config.LocalOverrideFileName)
	}
	if data, err := os.ReadFile(local); err == nil && yamlHasKey(data, "builtins") {
		return local
	}
	return shared
}

// settingsFile resolves a settings spec path the overview showed to the
// file on disk, refusing anything that is not a loaded settings spec.
func (h *editHandler) settingsFile(shown string) (string, error) {
	scope, err := loadCheckScope(h.global)
	if err != nil {
		return "", err
	}
	for _, e := range scope.bundle.Settings {
		if overview.SpecPath(h.root, e) == shown && e.Layer != "builtin" {
			return e.Path, nil
		}
	}
	return "", fmt.Errorf("%s is not a settings spec of this configuration", shown)
}

// applyValidated edits file, then validates the configuration. When the
// edit fails to load, or adds a validation problem, the original bytes go
// back and the problems are returned.
func (h *editHandler) applyValidated(file string, edit func([]byte) ([]byte, error)) ([]string, error) {
	before, beforeErr := h.validationProblems()
	original, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	existed := err == nil
	edited, err := edit(original)
	if err != nil {
		return nil, err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(file); err == nil {
		mode = info.Mode().Perm()
	}
	if err := writeFileAtomic(file, edited, mode); err != nil {
		return nil, err
	}
	after, afterErr := h.validationProblems()
	var added []string
	if afterErr != nil && beforeErr == nil {
		added = append(added, afterErr.Error())
	}
	for _, p := range after {
		if !slices.Contains(before, p) {
			added = append(added, p)
		}
	}
	if len(added) == 0 {
		return nil, nil
	}
	if existed {
		_ = writeFileAtomic(file, original, mode)
	} else {
		_ = os.Remove(file)
	}
	return added, errors.New("the edit did not validate, so the file was restored")
}

// validationProblems runs what validate runs and names each issue.
func (h *editHandler) validationProblems() ([]string, error) {
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

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".agnostic-ai-edit"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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

// setBuiltin adds name to, or removes it from, the builtins list,
// keeping every other key and comment. An emptied list is removed.
func setBuiltin(data []byte, name string, enabled bool) ([]byte, error) {
	doc, m, err := yamlDocument(data)
	if err != nil {
		return nil, err
	}
	at, list := yamlMapValue(m, "builtins")
	if list != nil && list.Kind != yaml.SequenceNode {
		return nil, errors.New("builtins is not a list")
	}
	has := func() int {
		if list == nil {
			return -1
		}
		return slices.IndexFunc(list.Content, func(n *yaml.Node) bool { return n.Value == name })
	}
	switch {
	case enabled && has() < 0:
		if list == nil {
			list = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "builtins"}, list)
		}
		list.Content = append(list.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name})
	case !enabled && has() >= 0:
		list.Content = slices.Delete(list.Content, has(), has()+1)
		if len(list.Content) == 0 {
			m.Content = slices.Delete(m.Content, at, at+2)
		}
	default:
		return data, nil
	}
	return encodeYAML(doc)
}

// setBool sets the boolean at a dotted key, which must exist and hold a
// boolean, keeping every other key and comment.
func setBool(data []byte, key string, value bool) ([]byte, error) {
	doc, node, err := yamlDocument(data)
	if err != nil {
		return nil, err
	}
	for _, part := range strings.Split(key, ".") {
		if node.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s is not a field of the file", key)
		}
		_, node = yamlMapValue(node, part)
		if node == nil {
			return nil, fmt.Errorf("%s is not a field of the file", key)
		}
	}
	if node.Kind != yaml.ScalarNode || node.ShortTag() != "!!bool" {
		return nil, fmt.Errorf("%s is not a boolean", key)
	}
	node.Value = fmt.Sprint(value)
	return encodeYAML(doc)
}
