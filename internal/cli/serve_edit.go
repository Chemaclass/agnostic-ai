package cli

import (
	"bytes"
	"cmp"
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

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/builtins"
	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Source-only edit endpoints.
const (
	editBuiltinPath     = "/api/edit/builtin"
	editValuePath       = "/api/edit/value"
	editFieldPath       = "/api/edit/field"
	editPermissionsPath = "/api/edit/permissions"
	editProtectedPath   = "/api/edit/protected"
)

// editHandler switches a built-in in the config file, or a boolean field
// in a settings spec, then validates the configuration and restores the
// file when the edit introduced a problem. root is the project root, or
// the user source root with user set.
type editHandler struct {
	root      string
	userLevel bool
	mu        sync.Mutex
	validate  func() ([]string, error)
	baselines map[string]editBaseline
}

// level names the served level in messages: project, or global.
func (h *editHandler) level() string {
	if h.userLevel {
		return "global"
	}
	return "project"
}

// levelLabel is level as the page's selector shows it.
func (h *editHandler) levelLabel() string {
	if h.userLevel {
		return "Global"
	}
	return "Project"
}

func newEditHandler(root string, userLevel bool) http.Handler {
	return &editHandler{root: root, userLevel: userLevel}
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

// editPlan is what an edit endpoint asks ServeHTTP to apply: the file,
// the change to its bytes, a check that its sources did not move, and
// who the file is for.
type editPlan struct {
	file        string
	edit        func([]byte) ([]byte, error)
	sourceGuard func() error
	audience    editAudience
}

// fileEdit builds the edit of one file.
type fileEdit func() (string, func([]byte) ([]byte, error), error)

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
	var plan editPlan
	var ok bool
	switch r.URL.Path {
	case editPermissionsPath:
		h.servePermissions(w, body)
		return
	case editProtectedPath:
		h.serveProtected(w, body)
		return
	case editMCPPath:
		h.serveMCP(w, body)
		return
	case editHookPath:
		h.serveHook(w, body)
		return
	case editBuiltinPath:
		plan, ok = h.serveBuiltinToggle(w, body)
	case editTargetsPath:
		plan, ok = h.serveTargetsToggle(w, body)
	case editValuePath:
		plan, ok = h.serveValue(w, body)
	case editFieldPath:
		plan, ok = h.serveField(w, body)
	default:
		writeEditJSON(w, http.StatusNotFound, editError{Error: "no edit at " + r.URL.Path})
		return
	}
	if ok {
		h.applyPlan(w, plan)
	}
}

// recheckedEdit runs the edit only while what the page showed still
// holds: it checks expected again, rebuilds the edit, and fails with
// staleToggleError when the edit now lands in another file.
func recheckedEdit(file, label string, recheck func() error, rebuild fileEdit) func([]byte) ([]byte, error) {
	return func(data []byte) ([]byte, error) {
		if err := recheck(); err != nil {
			return nil, err
		}
		currentFile, next, err := rebuild()
		if err != nil {
			return nil, err
		}
		if currentFile != file {
			return nil, staleToggleError{label}
		}
		return next(data)
	}
}

func (h *editHandler) serveBuiltinToggle(w http.ResponseWriter, body []byte) (editPlan, bool) {
	var req struct {
		audienceFields
		Target   string          `json:"target"`
		Name     string          `json:"name"`
		Enabled  *bool           `json:"enabled"`
		Reset    bool            `json:"reset"`
		Expected json.RawMessage `json:"expected"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Reset != (req.Name == "") || req.Reset && req.Enabled != nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "send a built-in name and enabled, or reset alone"})
		return editPlan{}, false
	}
	if newFieldAddress("", req.Target).target != "" {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "built-ins apply to every tool; send target all or leave it out"})
		return editPlan{}, false
	}
	if !req.Reset && !slices.Contains(builtins.Names(), req.Name) {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "name must be one of " + strings.Join(builtins.Names(), ", ")})
		return editPlan{}, false
	}
	if !req.Reset && req.Enabled == nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "enabled must be a boolean"})
		return editPlan{}, false
	}
	var plan editPlan
	var err error
	plan.audience, err = req.audience()
	audience, expected := plan.audience, len(req.Expected) > 0
	recheck := func() error { return h.checkBuiltinExpected(req.Name, req.Reset, req.Expected) }
	rebuild := func() (string, func([]byte) ([]byte, error), error) {
		return h.builtinsEdit(audience, builtinRequest{name: req.Name, enabled: req.Enabled != nil && *req.Enabled, reset: req.Reset})
	}
	if err == nil && expected {
		err = recheck()
	}
	if err == nil {
		plan.file, plan.edit, err = rebuild()
	}
	if err == nil && expected {
		team, local := h.builtinsConfigFiles()
		plan.sourceGuard, err = watchEditSources([]string{team, local}, plan.file, req.Name)
	}
	if err != nil {
		writeExpectedError(w, err)
		return editPlan{}, false
	}
	if expected {
		plan.edit = recheckedEdit(plan.file, req.Name, recheck, rebuild)
	}
	return plan, true
}

func (h *editHandler) serveTargetsToggle(w http.ResponseWriter, body []byte) (editPlan, bool) {
	var req struct {
		audienceFields
		Name     string          `json:"name"`
		Enabled  *bool           `json:"enabled"`
		Reset    bool            `json:"reset"`
		Expected json.RawMessage `json:"expected"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Reset != (req.Name == "") || req.Reset && req.Enabled != nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "send a coding tool name and enabled, or reset alone"})
		return editPlan{}, false
	}
	if !req.Reset && !slices.Contains(h.supportedTargetNames(), req.Name) {
		if h.userLevel && slices.Contains(adapters.Names(), req.Name) {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: fmt.Sprintf("sync --scope user cannot write %s; change it in a project", req.Name)})
			return editPlan{}, false
		}
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "name must be one of " + strings.Join(h.supportedTargetNames(), ", ")})
		return editPlan{}, false
	}
	if !req.Reset && req.Enabled == nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "enabled must be a boolean"})
		return editPlan{}, false
	}
	var plan editPlan
	var err error
	plan.audience, err = req.audience()
	audience, expected, label := plan.audience, len(req.Expected) > 0, cmp.Or(req.Name, "coding tools")
	change := targetsRequest{name: req.Name, enabled: req.Enabled != nil && *req.Enabled, reset: req.Reset}
	recheck := func() error { return h.checkTargetsExpected(req.Name, req.Reset, req.Expected) }
	rebuild := func() (string, func([]byte) ([]byte, error), error) { return h.targetsEdit(audience, change) }
	if err == nil && expected {
		err = recheck()
	}
	if err == nil {
		plan.file, plan.edit, err = rebuild()
	}
	if err == nil && expected {
		team, local := h.builtinsConfigFiles()
		plan.sourceGuard, err = watchEditSources([]string{team, local}, plan.file, label)
	}
	if err != nil {
		writeExpectedError(w, err)
		return editPlan{}, false
	}
	if expected {
		plan.edit = recheckedEdit(plan.file, label, recheck, rebuild)
	}
	return plan, true
}

func (h *editHandler) serveValue(w http.ResponseWriter, body []byte) (editPlan, bool) {
	var req struct {
		audienceFields
		Target   string          `json:"target"`
		Path     string          `json:"path"`
		Key      string          `json:"key"`
		Value    json.RawMessage `json:"value"`
		Expected json.RawMessage `json:"expected"`
	}
	var value *bool
	if err := json.Unmarshal(body, &req); err != nil || req.Path == "" || req.Key == "" || len(req.Value) == 0 || json.Unmarshal(req.Value, &value) != nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "path, key, and a boolean or null value are required"})
		return editPlan{}, false
	}
	if newFieldAddress(req.Key, req.Target).target != "" {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "the key names the tool already; send target all or leave it out"})
		return editPlan{}, false
	}
	var plan editPlan
	var err error
	plan.audience, err = req.audience()
	audience, expected := plan.audience, len(req.Expected) > 0
	recheck := func() error { return h.checkBooleanExpected(req.Path, req.Key, req.Expected) }
	rebuild := func() (string, func([]byte) ([]byte, error), error) {
		return h.settingsEdit(audience, req.Path, req.Key, value)
	}
	if err == nil && expected {
		err = recheck()
	}
	if err == nil {
		plan.file, plan.edit, err = rebuild()
	}
	if err == nil && expected {
		var paths []string
		paths, err = h.booleanSourcePaths(req.Path)
		if err == nil {
			plan.sourceGuard, err = watchEditSources(paths, plan.file, req.Key)
		}
	}
	if err != nil {
		writeExpectedError(w, err)
		return editPlan{}, false
	}
	if expected {
		plan.edit = recheckedEdit(plan.file, req.Key, recheck, rebuild)
	}
	return plan, true
}

func (h *editHandler) serveField(w http.ResponseWriter, body []byte) (editPlan, bool) {
	var req struct {
		audienceFields
		Target   string          `json:"target"`
		Path     string          `json:"path"`
		Key      string          `json:"key"`
		Value    json.RawMessage `json:"value"`
		Expected json.RawMessage `json:"expected"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Key == "" || len(req.Value) == 0 {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "key and value are required (null clears the field)"})
		return editPlan{}, false
	}
	var expected *string
	if len(req.Expected) > 0 && json.Unmarshal(req.Expected, &expected) != nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "expected must be the value the page showed, as text, or null when it was not set"})
		return editPlan{}, false
	}
	var plan editPlan
	var chosen spec.Entry
	addr := newFieldAddress(req.Key, req.Target)
	var err error
	plan.audience, err = req.audience()
	if err == nil {
		plan.file, plan.edit, err = h.fieldEdit(plan.audience, req.Path, addr, req.Value, &chosen)
	}
	var source fieldSourceError
	var nothing noPersonalValueError
	var tier tierEffortError
	var shadowed shadowedFieldError
	switch {
	case errors.As(err, &shadowed):
		writeEditJSON(w, http.StatusConflict, editError{Error: err.Error()})
		return editPlan{}, false
	case errors.As(err, &tier):
		writeEditJSON(w, http.StatusUnprocessableEntity, editError{Error: err.Error()})
		return editPlan{}, false
	case err != nil && len(req.Expected) > 0 && errors.As(err, &source):
		writeEditJSON(w, http.StatusConflict, editError{Error: err.Error() + ". This changed after the page loaded; reload to review it"})
		return editPlan{}, false
	case err != nil && len(req.Expected) > 0 && errors.As(err, &nothing):
		writeEditJSON(w, http.StatusOK, map[string]any{"ok": true})
		return editPlan{}, false
	case err != nil:
		writeEditJSON(w, http.StatusBadRequest, editError{Error: err.Error()})
		return editPlan{}, false
	}
	if len(req.Expected) > 0 {
		plan.edit = expectFieldValue(addr, expected, bytes.Equal(bytes.TrimSpace(req.Value), []byte("null")), inheritedFieldText(plan.audience, chosen, plan.file, addr), plan.edit)
	}
	return plan, true
}

// applyPlan writes the planned edit, keeping it only when the
// configuration still validates, and answers with the outcome.
func (h *editHandler) applyPlan(w http.ResponseWriter, plan editPlan) {
	var prepare func() (func(bool) error, error)
	if plan.audience == audiencePersonal {
		prepare = func() (func(bool) error, error) {
			finish, err := h.preparePersonalOutOfGit(plan.file)
			if err != nil {
				return nil, personalGitError{err}
			}
			return finish, nil
		}
	}
	sourceGuard := plan.sourceGuard
	if sourceGuard == nil {
		sourceGuard = func() error { return nil }
	}
	if problems, err := h.applyValidatedWithPrepare(plan.file, plan.edit, prepare, sourceGuard); err != nil {
		if _, ok := errors.AsType[personalGitError](err); ok {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: err.Error()})
			return
		}
		if _, ok := errors.AsType[staleToggleError](err); ok {
			writeEditJSON(w, http.StatusConflict, editError{Error: err.Error()})
			return
		}
		if stale, ok := errors.AsType[staleFieldError](err); ok {
			writeEditJSON(w, http.StatusConflict, editError{Error: overviewDisplay(h.root, plan.file) + ": " + stale.Error()})
			return
		}
		if errors.Is(err, errLastTarget) {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: err.Error()})
			return
		}
		writeEditJSON(w, http.StatusUnprocessableEntity, editError{Error: err.Error(), Problems: problems})
		return
	}
	writeEditJSON(w, http.StatusOK, map[string]any{"ok": true, "file": overviewDisplay(h.root, plan.file)})
}

// allowGetOnly answers 405 and is false for a request that is not a GET
// or a HEAD.
func allowGetOnly(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	w.Header().Set("Allow", "GET, HEAD")
	writeEditJSON(w, http.StatusMethodNotAllowed, editError{Error: r.Method + " is not supported here; use GET"})
	return false
}

// Origin checks also honor origins explicitly allowed by the outer server.
func editRequestGuard(r *http.Request) (int, string) {
	if r.Method != http.MethodPost {
		return http.StatusMethodNotAllowed, "use POST"
	}
	if mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mediaType != "application/json" {
		return http.StatusUnsupportedMediaType, "send Content-Type: application/json"
	}
	switch requestOriginStatus(r) {
	case http.StatusMisdirectedRequest:
		return http.StatusMisdirectedRequest, "edits are served on loopback only"
	case http.StatusForbidden:
		return http.StatusForbidden, "edits are accepted from the server origin only"
	}
	return 0, ""
}

// requestOriginStatus is 0 for a request from the server's own origin,
// 421 for one that names no browser origin and came off loopback, and
// 403 for one from another origin.
func requestOriginStatus(r *http.Request) int {
	if authority.RequestAllowed(r) {
		return 0
	}
	if r.Header.Get("Origin") == "" && r.Header.Get("Sec-Fetch-Site") != "cross-site" {
		return http.StatusMisdirectedRequest
	}
	return http.StatusForbidden
}

// Only a boolean present in the loaded settings can gain a local override.
// A nil value resets the personal override, and removes a personal file
// that then only names its spec.
func (h *editHandler) settingsEdit(where editAudience, shown, key string, value *bool) (string, func([]byte) ([]byte, error), error) {
	if voiceEditField(key) && !h.userLevel {
		return "", nil, errors.New("voice is editable in the user-level view only")
	}
	scope, err := loadCheckScope(h.userLevel)
	if err != nil {
		return "", nil, err
	}
	for _, e := range scope.bundle.Settings {
		if !h.matchesShownSpec(e, shown) || e.Layer == "builtin" {
			continue
		}
		if voiceEditField(key) {
			if reason := overview.VoiceEditDisabledReason(e, h.root); reason != "" {
				return "", nil, errors.New(reason)
			}
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
		if value == nil && (where != audiencePersonal || !overview.LocalSets(e, key)) {
			return "", nil, fmt.Errorf("%s has no local value to reset; send audience personal for a key your local file sets", key)
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
	scope, err := loadCheckScope(h.userLevel)
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
	if rel, inside := pathBelow(root, path); inside {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// yamlDocument parses data into a document whose root is a mapping and
// returns that mapping; an empty file becomes an empty mapping.
func yamlDocument(data []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if len(bytes.TrimSpace(data)) > 0 {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse: %w", err)
		}
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("the file is not a YAML mapping")
	}
	return doc.Content[0], nil
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
	m, err := yamlDocument(data)
	if err != nil {
		return false
	}
	_, v := yamlMapValue(m, key)
	return v != nil
}
