package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const editHookPath = "/api/edit/hook"

// hookFormMatches are the tool kinds the form offers; leaving match out
// means every tool, so any is not needed.
var hookFormMatches = []string{"shell", "edit", "read", "web"}

// shellVariable matches a `$NAME` or `$1` the shell fills in at run time;
// the credential detector alone knows only `${NAME}`.
var shellVariable = regexp.MustCompile(`\$(?:[A-Za-z_][A-Za-z0-9_]*|[0-9@*#?])`)

// hookCommandCredential reports whether command holds a literal
// credential. The text of each `$(...)`, `<(...)`, `>(...)`, or backtick
// substitution is checked on its own, nested ones included; one with no
// credential then counts as a reference, as `$NAME` does.
func hookCommandCredential(command string) bool {
	var out strings.Builder
	for i := 0; i < len(command); i++ {
		c := command[i]
		if c == '`' {
			end := strings.IndexByte(command[i+1:], '`')
			if end < 0 {
				out.WriteString(command[i:])
				break
			}
			if hookCommandCredential(command[i+1 : i+1+end]) {
				return true
			}
			out.WriteString("${REF}")
			i += end + 1
			continue
		}
		if (c == '$' || c == '<' || c == '>') && i+1 < len(command) && command[i+1] == '(' {
			depth, end := 0, -1
			for j := i + 1; j < len(command); j++ {
				if command[j] == '(' {
					depth++
				} else if command[j] == ')' {
					depth--
					if depth == 0 {
						end = j
						break
					}
				}
			}
			if end < 0 {
				out.WriteString(command[i:])
				break
			}
			if hookCommandCredential(command[i+2 : end]) {
				return true
			}
			out.WriteString("${REF}")
			i = end
			continue
		}
		out.WriteByte(c)
	}
	return preview.CommandCredentialDetected(shellVariable.ReplaceAllString(out.String(), "$${REF}"))
}

type hookSpecRequest struct {
	Action      string `json:"action"`
	Audience    string `json:"audience"`
	Name        string `json:"name"`
	On          string `json:"on"`
	Match       string `json:"match,omitempty"`
	Command     string `json:"command"`
	Timeout     *int   `json:"timeout,omitempty"`
	Description string `json:"description,omitempty"`
	// TargetsExclude names the configured tools that have no event for
	// the stage, so the hook leaves them out instead of failing validate.
	TargetsExclude []string `json:"targetsExclude,omitempty"`
	Expected       string   `json:"expected,omitempty"`
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
	if decodeStrictJSON(bytes.NewReader(body), &req) != nil || (req.Action != "preview" && req.Action != "apply") || req.Name == "" || req.On == "" {
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
	if req.Expected == "" {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: errApplyWithoutPreview})
		return
	}
	if req.Expected != proposed.Expected {
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
	if preview.TokenIn(req.Name) {
		return errors.New("name contains a credential; choose a name that describes the hook")
	}
	if !slugRe.MatchString(req.Name) || spec.ValidateName(spec.KindHook, req.Name) != nil {
		return fmt.Errorf("name %q must use lowercase letters, digits, and hyphens, such as format-on-save", req.Name)
	}
	if !slices.Contains(spec.PortableHookEvents, req.On) {
		return fmt.Errorf("on must be one of %s", strings.Join(spec.PortableHookEvents, ", "))
	}
	if strings.TrimSpace(req.Command) == "" {
		return errors.New("command is required")
	}
	if hookCommandCredential(req.Command) {
		return errors.New("command contains a credential; read it at run time instead, as $GITHUB_TOKEN, ${GITHUB_TOKEN}, or $(gh auth token) do")
	}
	if preview.CredentialDetected(req.Description) {
		return errors.New("description contains a credential; leave it out")
	}
	if req.Match != "" {
		if req.On != "before-tool" && req.On != "after-tool" {
			return errors.New("match applies only to before-tool and after-tool")
		}
		if !slices.Contains(hookFormMatches, req.Match) {
			return fmt.Errorf("match must be one of %s, or left out for every tool", strings.Join(hookFormMatches, ", "))
		}
	}
	for _, target := range req.TargetsExclude {
		if !slugRe.MatchString(target) {
			return fmt.Errorf("targetsExclude: %q is not a tool name", target)
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
	if exists, ok := errors.AsType[hookExists](err); ok {
		writeEditJSON(w, http.StatusConflict, editError{Error: fmt.Sprintf("A hook named %s already exists", string(exists))})
		return
	}
	if conflict, ok := errors.AsType[hookEditConflict](err); ok {
		writeEditJSON(w, http.StatusConflict, editError{Error: conflict.Error()})
		return
	}
	if problems, ok := errors.AsType[hookSpecProblems](err); ok {
		writeEditJSON(w, http.StatusUnprocessableEntity, editError{Error: "The hook does not validate", Problems: problems})
		return
	}
	writeEditJSON(w, http.StatusUnprocessableEntity, editError{Error: err.Error()})
}

// checkTargetsExclude refuses a tool the setup does not configure, a
// repeat, and a list that leaves the hook no tool to run on. It returns
// the list without the tools that write no hooks in this scope, which the
// hook never reaches anyway.
func checkTargetsExclude(exclude, configured, hookTargets []string) ([]string, error) {
	writers := slices.DeleteFunc(slices.Clone(hookTargets), func(target string) bool {
		_, ok := targetsSupportingKind[spec.KindHook][target]
		return !ok
	})
	var kept []string
	for i, target := range exclude {
		if !slices.Contains(configured, target) {
			return nil, hookSpecProblems{fmt.Sprintf("targetsExclude: %s is not a configured tool", target)}
		}
		if slices.Contains(exclude[:i], target) {
			return nil, hookSpecProblems{fmt.Sprintf("targetsExclude: %s is listed twice", target)}
		}
		if slices.Contains(writers, target) {
			kept = append(kept, target)
		}
	}
	if len(kept) > 0 && !slices.ContainsFunc(writers, func(target string) bool { return !slices.Contains(kept, target) }) {
		return nil, hookSpecProblems{"targetsExclude leaves the hook no tool to run on"}
	}
	return kept, nil
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
		if len(req.TargetsExclude) > 0 {
			list := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			for _, target := range req.TargetsExclude {
				list.Content = append(list.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: target})
			}
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "targets-exclude"}, list)
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
	scope, hooks, err := loadCheckScopeAllHooks(h.userLevel)
	if err != nil {
		return hookSpecPreview{}, "", err
	}
	for _, e := range hooks {
		stem := strings.TrimSuffix(filepath.Base(e.Path), filepath.Ext(e.Path))
		if strings.EqualFold(e.Name, req.Name) || e.Path != "" && strings.EqualFold(stem, req.Name) {
			return hookSpecPreview{}, "", hookEditConflict{hookExists(req.Name)}
		}
	}
	kept, err := checkTargetsExclude(req.TargetsExclude, scope.targets, scope.hookTargets)
	if err != nil {
		return hookSpecPreview{}, "", err
	}
	req.TargetsExclude = kept
	dir := filepath.Join(h.root, "hooks")
	if !h.userLevel {
		dir = config.ResolveSourcePath(h.root, scope.cfg.Sources.Hooks)
	}
	if where == audiencePersonal {
		local, _ := h.personalRoot("")
		dir = filepath.Join(local, "hooks")
	}
	file, ok := pathUnder(dir, req.Name+".yaml")
	if !ok {
		return hookSpecPreview{}, "", fmt.Errorf("name %q is not a usable file name on this system", req.Name)
	}
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
	entry.Path, entry.Name = file, req.Name
	if problems := h.addedHookIssues(scope, entry); len(problems) > 0 {
		return hookSpecPreview{}, "", problems
	}
	command := "agnostic-ai sync"
	if h.userLevel {
		command += " --scope user"
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

// addedHookIssues runs what validate runs with the hook added, and names
// only the issues the hook brings, as apply compares before and after.
func (h *editHandler) addedHookIssues(scope checkScope, entry spec.Entry) hookSpecProblems {
	describe := func(issues []validationIssue) []string {
		out := make([]string, 0, len(issues))
		for _, i := range issues {
			out = append(out, overviewDisplay(h.root, i.Path)+": "+i.Message)
		}
		return out
	}
	before := describe(scopeValidationIssues(scope))
	scope.bundle.Hooks = append(slices.Clone(scope.bundle.Hooks), entry)
	var added hookSpecProblems
	for _, problem := range describe(scopeValidationIssues(scope)) {
		if !slices.Contains(before, problem) {
			added = append(added, problem)
		}
	}
	return added
}
