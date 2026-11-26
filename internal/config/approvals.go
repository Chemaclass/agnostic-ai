package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// Values approvals.mode, approvals.tools and approvals.projects accept.
const (
	ApprovalsAsk    = "ask"
	ApprovalsReview = "review"
	ApprovalsYolo   = "yolo"
)

// ApprovalModes are the values an approvals mode accepts.
var ApprovalModes = []string{ApprovalsAsk, ApprovalsReview, ApprovalsYolo}

// ApprovalsBuiltin is the built-in that installs the approvals hooks.
// Only sync --scope user writes it: a project sync skips it.
const ApprovalsBuiltin = "approvals"

// AutosyncBuiltin is the built-in whose session-start hooks sync the
// project. Only a project sync writes it: sync --scope user skips it,
// so a personal list never rewrites files in every repository opened.
const AutosyncBuiltin = "autosync"

// EnvApprovals names the variable that turns approvals off for one
// session. Only ask is honored: the environment cannot loosen.
const EnvApprovals = "AGNOSTIC_AI_APPROVALS"

// ApprovalsFromDefault is the source of a mode no setting chose.
const ApprovalsFromDefault = "default"

// ApprovalsConfig is what happens when a tool wants permission for a
// tool call. Loosening is read only from the home config; a project file
// may say ask and nothing else.
type ApprovalsConfig struct {
	Mode      string            `yaml:"mode,omitempty"       json:"mode,omitempty"       jsonschema:"enum=ask,enum=review,enum=yolo" jsonschema_description:"What happens when a tool wants permission: ask (default) shows the tool's prompt; review lets the tool's own reviewer model decide; yolo approves one call at a time unless a fixed always-ask rule matches. review and yolo are read only from $AGNOSTIC_AI_HOME/agnostic-ai.yaml or its local/ file; a project file may set ask only."`
	Tools     map[string]string `yaml:"tools,omitempty"      json:"tools,omitempty"      jsonschema_description:"Mode per tool, by target name. Replaces mode. Home config only."`
	Projects  map[string]string `yaml:"projects,omitempty"   json:"projects,omitempty"   jsonschema_description:"Mode per project folder, an absolute path or one under ~. The deepest folder that holds the session wins. Replaces mode and tools. Home config only."`
	AlwaysAsk []string          `yaml:"always-ask,omitempty" json:"always-ask,omitempty" jsonschema_description:"Extra patterns that always show the prompt, added to the fixed list. The patterns of both home files add up. Home config only."`
	Companion bool              `yaml:"companion,omitempty"  json:"companion,omitempty"  jsonschema_description:"Show Approve and Deny buttons in the companion for a Claude Code or Codex request that approvals leave to you. The hook waits up to 300 seconds for Claude Code, whose prompt stays open, and up to 60 seconds for Codex, which shows its prompt after. Default false. Home config only."`
}

// Loosens reports whether any mode in the block is review or yolo.
func (a ApprovalsConfig) Loosens() bool {
	modes := []string{a.Mode}
	for _, m := range a.Tools {
		modes = append(modes, m)
	}
	for _, m := range a.Projects {
		modes = append(modes, m)
	}
	return slices.ContainsFunc(modes, func(m string) bool { return m == ApprovalsReview || m == ApprovalsYolo })
}

// Validate rejects a mode outside ApprovalModes and a projects key that
// is not an absolute path or one under ~.
func (a ApprovalsConfig) Validate(source string) error {
	if err := validApprovalMode(source, "approvals.mode", a.Mode); err != nil {
		return err
	}
	for key := range a.Projects {
		if _, ok := projectKeyPath(key, "/"); !ok {
			return errs.Coded(errs.CodeConfigDecode, "%s: approvals.projects: %q is not an absolute path or one under ~", source, key)
		}
	}
	for _, field := range []struct {
		name   string
		values map[string]string
	}{{"approvals.tools", a.Tools}, {"approvals.projects", a.Projects}} {
		keys := make([]string, 0, len(field.values))
		for key := range field.values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := validApprovalMode(source, field.name+"."+key, field.values[key]); err != nil {
				return err
			}
		}
	}
	return nil
}

func validApprovalMode(source, key, mode string) error {
	if mode == "" || slices.Contains(ApprovalModes, mode) {
		return nil
	}
	return errs.Coded(errs.CodeConfigDecode, "%s: %s: %q is not one of %s", source, key, mode, strings.Join(ApprovalModes, ", "))
}

// ApprovalsPolicy is every input that decides a mode, read once.
type ApprovalsPolicy struct {
	// Home is the approvals block of the home config.
	Home ApprovalsConfig
	// UserHome expands a leading ~ in a projects key.
	UserHome string
	// Env is the value of EnvApprovals.
	Env string
	// ProjectAsk is the project file that says approvals.mode: ask.
	ProjectAsk string
}

// ApprovalDecision is a resolved mode and the setting that chose it.
type ApprovalDecision struct {
	Mode string
	From string
}

// Resolve returns the mode for tool in dir, strictest source first: the
// environment, a project file, projects, tools, mode. A value outside
// ApprovalModes reads as ask. Any value of the environment variable asks:
// it can only tighten.
func (p ApprovalsPolicy) Resolve(tool, dir string) ApprovalDecision {
	if strings.TrimSpace(p.Env) != "" {
		return ApprovalDecision{ApprovalsAsk, EnvApprovals}
	}
	if p.ProjectAsk != "" {
		return ApprovalDecision{ApprovalsAsk, p.ProjectAsk}
	}
	if key, mode, ok := p.project(dir); ok {
		return checkedDecision(mode, "approvals.projects["+key+"]")
	}
	if mode, ok := p.Home.Tools[tool]; ok && mode != "" {
		return checkedDecision(mode, "approvals.tools."+tool)
	}
	if p.Home.Mode != "" {
		return checkedDecision(p.Home.Mode, "approvals.mode")
	}
	return ApprovalDecision{ApprovalsAsk, ApprovalsFromDefault}
}

func checkedDecision(mode, from string) ApprovalDecision {
	if !slices.Contains(ApprovalModes, mode) {
		return ApprovalDecision{ApprovalsAsk, from}
	}
	return ApprovalDecision{mode, from}
}

// ProjectFolders lists the folders named in approvals.projects with ~ expanded.
func (p ApprovalsPolicy) ProjectFolders() []string {
	var folders []string
	for key := range p.Home.Projects {
		if path, ok := projectKeyPath(key, p.UserHome); ok {
			folders = append(folders, path)
		}
	}
	return folders
}

// project returns the deepest projects key that holds dir. Two keys for
// one folder resolve to the stricter mode.
func (p ApprovalsPolicy) project(dir string) (key, mode string, ok bool) {
	dir = realPath(dir)
	best := -1
	for k, m := range p.Home.Projects {
		path, valid := projectKeyPath(k, p.UserHome)
		if !valid {
			continue
		}
		path = realPath(path)
		if !pathHolds(path, dir) || len(path) < best {
			continue
		}
		if len(path) == best && (strictness(m) > strictness(mode) || (strictness(m) == strictness(mode) && k > key)) {
			continue
		}
		key, mode, ok, best = k, m, true, len(path)
	}
	return key, mode, ok
}

// strictness ranks ask first; an unknown mode reads as ask.
func strictness(mode string) int {
	if i := slices.Index(ApprovalModes, mode); i >= 0 {
		return i
	}
	return 0
}

// projectKeyPath is a projects key as a path: ~ expands to userHome.
// ok is false for a relative key.
func projectKeyPath(key, userHome string) (string, bool) {
	if key == "~" || strings.HasPrefix(key, "~/") || strings.HasPrefix(key, `~\`) {
		if userHome == "" {
			return "", false
		}
		return filepath.Join(userHome, filepath.FromSlash(key[1:])), true
	}
	path := filepath.FromSlash(key)
	return path, filepath.IsAbs(path) || strings.HasPrefix(path, string(filepath.Separator))
}

// realPath is path with symlinks resolved, or cleaned when it does not
// exist, so a link cannot place a folder inside another project.
func realPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

// pathHolds reports whether path is root or inside it. macOS and Windows
// disks ignore case by default, so there a key matches in any case.
func pathHolds(root, path string) bool {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		root, path = strings.ToLower(root), strings.ToLower(path)
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// LoadHomeApprovals reads the approvals block of the home config in
// source. A block in local/agnostic-ai.yaml replaces the shared one whole,
// except always-ask, whose patterns add up, and companion, which keeps
// the shared value unless the local block names it: Ask me writes a
// local block and must not turn the buttons off. A relative source is
// refused: it would read a file in whatever folder the session runs in.
func LoadHomeApprovals(source string) (ApprovalsConfig, error) {
	return LoadHomeApprovalsWith(source, "", nil)
}

// LoadHomeApprovalsWith is LoadHomeApprovals with the home file at path
// read as data, so a writer can check the policy an edit gives before it
// writes.
func LoadHomeApprovalsWith(source, path string, data []byte) (ApprovalsConfig, error) {
	if path == "" {
		return LoadHomeApprovalsFiles(source, nil)
	}
	return LoadHomeApprovalsFiles(source, map[string][]byte{filepath.Clean(path): data})
}

// LoadHomeApprovalsFiles is LoadHomeApprovals with each home file named
// in files, by its clean path, read as its data instead of from disk;
// empty data reads as a missing file.
func LoadHomeApprovalsFiles(source string, files map[string][]byte) (ApprovalsConfig, error) {
	if !filepath.IsAbs(source) {
		return ApprovalsConfig{}, fmt.Errorf("approvals: the home folder %q is not an absolute path", source)
	}
	var out ApprovalsConfig
	var alwaysAsk []string
	for _, path := range []string{filepath.Join(source, ConfigFileName), filepath.Join(source, "local", ConfigFileName)} {
		node, err := approvalsNode(path)
		if data, ok := files[filepath.Clean(path)]; ok {
			node, err = topLevelNodeOf(path, data, "approvals")
		}
		if err != nil {
			return ApprovalsConfig{}, err
		}
		if node == nil {
			continue
		}
		var block ApprovalsConfig
		if node.Tag != "!!null" {
			data, err := yaml.Marshal(node)
			if err != nil {
				return ApprovalsConfig{}, fmt.Errorf("%s: approvals: %w", path, err)
			}
			dec := yaml.NewDecoder(bytes.NewReader(data))
			dec.KnownFields(true)
			if err := dec.Decode(&block); err != nil {
				return ApprovalsConfig{}, errs.Coded(errs.CodeConfigDecode, "%s: approvals: %w", path, err)
			}
		}
		if err := block.Validate(path); err != nil {
			return ApprovalsConfig{}, err
		}
		if !mappingHasKey(node, "companion") {
			block.Companion = out.Companion
		}
		out = block
		alwaysAsk = append(alwaysAsk, block.AlwaysAsk...)
	}
	out.AlwaysAsk = alwaysAsk
	return out, nil
}

func mappingHasKey(node *yaml.Node, key string) bool {
	if node.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return true
		}
	}
	return false
}

// approvalsNode is the value of the top-level approvals key in path, or
// nil when the file or the key is missing.
func approvalsNode(path string) (*yaml.Node, error) {
	return topLevelNode(path, "approvals")
}

// topLevelNode is the value of the top-level key in path, or nil when
// the file or the key is missing.
func topLevelNode(path, key string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return topLevelNodeOf(path, data, key)
}

// topLevelNodeOf is topLevelNode on data read from path.
func topLevelNodeOf(path string, data []byte, key string) (*yaml.Node, error) {
	var doc map[string]yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, errs.Coded(errs.CodeConfigDecode, "parse %s: %w", path, err)
	}
	node, ok := doc[key]
	if !ok {
		return nil, nil
	}
	return &node, nil
}

// projectConfigNames are the project files that may say ask.
var projectConfigNames = []string{ConfigFileName, LegacyConfigFileName, LocalOverrideFileName}

// ProjectApprovalsAsk returns the first project file in dir or a parent
// folder that sets approvals.mode: ask, or "" when none does. Any other
// value is ignored: a project file cannot loosen. Both the path as given
// and the path with links resolved are walked.
func ProjectApprovalsAsk(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", dir, err)
	}
	for _, start := range []string{abs, realPath(abs)} {
		if path, err := projectAskFrom(start); path != "" || err != nil {
			return path, err
		}
	}
	return "", nil
}

func projectAskFrom(dir string) (string, error) {
	for {
		for _, name := range projectConfigNames {
			path := filepath.Join(dir, name)
			node, err := approvalsNode(path)
			if err != nil {
				return "", err
			}
			if approvalsModeOf(node) == ApprovalsAsk {
				return path, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

func approvalsModeOf(node *yaml.Node) string {
	if node == nil {
		return ""
	}
	if node.Kind == yaml.ScalarNode {
		return normalizedMode(node.Value)
	}
	if node.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if strings.EqualFold(node.Content[i].Value, "mode") && node.Content[i+1].Kind == yaml.ScalarNode {
			return normalizedMode(node.Content[i+1].Value)
		}
	}
	return ""
}

// normalizedMode reads a project file's near miss of ask, such as Ask, as ask.
func normalizedMode(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// ApprovalsIssue is a project file key that would loosen approvals.
type ApprovalsIssue struct {
	Path    string
	Line    int
	Message string
}

// ProjectApprovalsIssues reports every approvals key in the project files
// at root other than mode: ask. A cloned repository, a pull request, or
// the agent itself can write those files, so they may only tighten.
func ProjectApprovalsIssues(root string) ([]ApprovalsIssue, error) {
	var issues []ApprovalsIssue
	for _, name := range projectConfigNames {
		path := filepath.Join(root, name)
		builtins, err := topLevelNode(path, "builtins")
		if err != nil {
			return nil, err
		}
		if builtins != nil && builtins.Kind == yaml.SequenceNode {
			for _, item := range builtins.Content {
				if item.Value == ApprovalsBuiltin {
					issues = append(issues, ApprovalsIssue{path, item.Line, "builtins: approvals is read only from the home config, where sync --scope user writes its hooks; a project sync skips it"})
				}
			}
		}
		node, err := approvalsNode(path)
		if err != nil {
			return nil, err
		}
		if node == nil || node.Tag == "!!null" {
			continue
		}
		if node.Kind != yaml.MappingNode {
			issues = append(issues, ApprovalsIssue{path, node.Line, "approvals: a project file may set only approvals.mode: ask"})
			continue
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			switch {
			case key.Value == "mode" && value.Value == ApprovalsAsk:
			case key.Value == "mode":
				issues = append(issues, ApprovalsIssue{path, key.Line, fmt.Sprintf("approvals.mode: %s is read only from the home config; a project file may set only approvals.mode: ask", value.Value)})
			default:
				issues = append(issues, ApprovalsIssue{path, key.Line, fmt.Sprintf("approvals.%s is read only from the home config; a project file may set only approvals.mode: ask", key.Value)})
			}
		}
	}
	return issues, nil
}
