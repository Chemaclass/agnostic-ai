package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const editHookPath = "/api/edit/hook"

type hookSpecRequest struct {
	Action      string `json:"action"`
	Audience    string `json:"audience"`
	Name        string `json:"name"`
	On          string `json:"on"`
	Match       string `json:"match,omitempty"`
	Command     string `json:"command"`
	Timeout     *int   `json:"timeout,omitempty"`
	Description string `json:"description,omitempty"`
	Expected    string `json:"expected,omitempty"`
}

type hookSpecPreview struct {
	File        string `json:"file"`
	Spec        string `json:"spec"`
	SyncCommand string `json:"syncCommand"`
	Expected    string `json:"expected"`
	OK          bool   `json:"ok,omitempty"`
}

type hookEditConflict struct{ error }

func (e hookEditConflict) Unwrap() error { return e.error }

type hookSpecProblems []string

func (p hookSpecProblems) Error() string {
	return "the hook does not validate: " + strings.Join(p, "; ")
}

func (h *editHandler) serveHook(w http.ResponseWriter, body []byte) {
	var req hookSpecRequest
	if json.Unmarshal(body, &req) != nil || (req.Action != "preview" && req.Action != "apply") || req.Name == "" || req.On == "" {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "send action preview or apply, audience, name, on, and command; timeout is whole seconds"})
		return
	}
	where, err := (audienceFields{Audience: req.Audience}).audience()
	if err != nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: err.Error()})
		return
	}
	if err := req.check(); err != nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: err.Error()})
		return
	}
	proposed, file, err := h.hookPreview(req, where)
	if err != nil {
		writeHookError(w, err)
		return
	}
	if req.Action == "preview" {
		writeEditJSON(w, http.StatusOK, proposed)
		return
	}
	if req.Expected == "" || req.Expected != proposed.Expected {
		writeEditJSON(w, http.StatusConflict, editError{Error: "The hook or its destination changed; review a fresh preview before applying"})
		return
	}
	var prepare func() (func(bool) error, error)
	if where == audiencePersonal {
		prepare = func() (func(bool) error, error) { return h.preparePersonalOutOfGit(file) }
	}
	guard := func() error {
		now, next, err := h.hookPreview(req, where)
		if err != nil {
			return err
		}
		if next != file || now.Expected != req.Expected {
			return hookEditConflict{errors.New("the hook or its destination changed; review a fresh preview before applying")}
		}
		return nil
	}
	problems, err := h.applyValidatedWithPrepare(file, func(data []byte) ([]byte, error) {
		if len(data) > 0 {
			return nil, hookEditConflict{hookExists(req.Name)}
		}
		return []byte(proposed.Spec), nil
	}, prepare, guard)
	if err != nil {
		if len(problems) > 0 {
			writeEditJSON(w, http.StatusUnprocessableEntity, editError{Error: err.Error(), Problems: problems})
			return
		}
		writeHookError(w, err)
		return
	}
	proposed.OK = true
	writeEditJSON(w, http.StatusOK, proposed)
}

func (req hookSpecRequest) check() error {
	if !slugRe.MatchString(req.Name) || spec.ValidateName(spec.KindHook, req.Name) != nil {
		return fmt.Errorf("name %q must use lowercase letters, digits, and hyphens, such as format-on-save", req.Name)
	}
	if !slices.Contains(spec.PortableHookEvents, req.On) {
		return fmt.Errorf("on must be one of %s", strings.Join(spec.PortableHookEvents, ", "))
	}
	if strings.TrimSpace(req.Command) == "" {
		return errors.New("command is required")
	}
	if req.Match != "" {
		if req.On != "before-tool" && req.On != "after-tool" {
			return errors.New("match applies only to before-tool and after-tool")
		}
		if !slices.Contains(spec.HookToolKinds, req.Match) {
			return fmt.Errorf("match must be one of %s", strings.Join(spec.HookToolKinds, ", "))
		}
	}
	if req.Timeout != nil && *req.Timeout <= 0 {
		return errors.New("timeout must be a whole number of seconds above zero")
	}
	return nil
}

type hookExists string

func (e hookExists) Error() string {
	return fmt.Sprintf("a hook named %s already exists", string(e))
}

func writeHookError(w http.ResponseWriter, err error) {
	var exists hookExists
	if errors.As(err, &exists) {
		writeEditJSON(w, http.StatusConflict, editError{Error: fmt.Sprintf("A hook named %s already exists", string(exists))})
		return
	}
	var conflict hookEditConflict
	if errors.As(err, &conflict) {
		writeEditJSON(w, http.StatusConflict, editError{Error: conflict.Error()})
		return
	}
	var problems hookSpecProblems
	if errors.As(err, &problems) {
		writeEditJSON(w, http.StatusUnprocessableEntity, editError{Error: "The hook does not validate", Problems: problems})
		return
	}
	writeEditJSON(w, http.StatusUnprocessableEntity, editError{Error: err.Error()})
}

// hookSpecYAML writes the fields in the order the hook reference lists them.
// The command keeps every byte, since trailing spaces and backslashes can
// change what the shell runs; the first style that reads back unchanged wins.
func hookSpecYAML(req hookSpecRequest) ([]byte, error) {
	styles := []yaml.Style{0, yaml.DoubleQuotedStyle}
	if strings.Contains(req.Command, "\n") {
		styles = append([]yaml.Style{yaml.LiteralStyle}, styles...)
	}
	for _, style := range styles {
		m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		add := func(key, value string, style yaml.Style, tag string) {
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value, Style: style})
		}
		add("name", req.Name, 0, "!!str")
		if description := strings.TrimSpace(req.Description); description != "" {
			add("description", description, 0, "!!str")
		}
		add("on", req.On, 0, "!!str")
		if req.Match != "" {
			add("match", req.Match, 0, "!!str")
		}
		add("command", req.Command, style, "!!str")
		if req.Timeout != nil {
			add("timeout", strconv.Itoa(*req.Timeout), 0, "!!int")
		}
		data, err := encodeYAML(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{m}})
		if err != nil {
			continue
		}
		back, err := spec.ParseYAMLBytes(spec.KindHook, data)
		if err == nil && back.Meta["command"] == req.Command && back.Meta["on"] == req.On && back.Name == req.Name {
			return data, nil
		}
	}
	return nil, errors.New("the command cannot be written as YAML unchanged")
}

func (h *editHandler) hookPreview(req hookSpecRequest, where editAudience) (hookSpecPreview, string, error) {
	scope, err := loadCheckScope(h.global)
	if err != nil {
		return hookSpecPreview{}, "", err
	}
	for _, e := range scope.bundle.Hooks {
		stem := strings.TrimSuffix(filepath.Base(e.Path), filepath.Ext(e.Path))
		if strings.EqualFold(e.Name, req.Name) || e.Path != "" && strings.EqualFold(stem, req.Name) {
			return hookSpecPreview{}, "", hookEditConflict{hookExists(req.Name)}
		}
	}
	dir := filepath.Join(h.root, "hooks")
	if !h.global {
		dir = config.ResolveSourcePath(h.root, scope.cfg.Sources.Hooks)
	}
	if where == audiencePersonal {
		local, _ := h.personalRoot("")
		dir = filepath.Join(local, "hooks")
	}
	file := filepath.Join(dir, req.Name+".yaml")
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return hookSpecPreview{}, "", fmt.Errorf("%s: %w", dir, err)
	}
	for _, entry := range entries {
		ext := filepath.Ext(entry.Name())
		if (ext == ".yaml" || ext == ".yml") && strings.EqualFold(strings.TrimSuffix(entry.Name(), ext), req.Name) {
			return hookSpecPreview{}, "", hookEditConflict{hookExists(req.Name)}
		}
	}
	if where == audiencePersonal {
		if err := h.checkPersonalTarget(file); err != nil {
			return hookSpecPreview{}, "", err
		}
	}
	data, err := hookSpecYAML(req)
	if err != nil {
		return hookSpecPreview{}, "", err
	}
	entry, err := spec.ParseYAMLBytes(spec.KindHook, data)
	if err != nil {
		return hookSpecPreview{}, "", err
	}
	entry.Path = file
	issues := append(lintEntries([]spec.Entry{entry}), lintHookEvents([]spec.Entry{entry}, scope.hookTargets)...)
	if len(issues) > 0 {
		problems := make(hookSpecProblems, 0, len(issues))
		for _, issue := range issues {
			problems = append(problems, issue.Message)
		}
		return hookSpecPreview{}, "", problems
	}
	command := "agnostic-ai sync"
	if h.global {
		command += " --global"
	}
	proposed := hookSpecPreview{File: overviewDisplay(h.root, file), Spec: string(data), SyncCommand: command}
	identity := req
	identity.Action, identity.Expected = "", ""
	fingerprint, err := json.Marshal(struct {
		Request hookSpecRequest
		Preview hookSpecPreview
		File    string
	}{identity, proposed, file})
	if err != nil {
		return hookSpecPreview{}, "", fmt.Errorf("prepare hook preview: %w", err)
	}
	sum := sha256.Sum256(fingerprint)
	proposed.Expected = hex.EncodeToString(sum[:])
	return proposed, file, nil
}
