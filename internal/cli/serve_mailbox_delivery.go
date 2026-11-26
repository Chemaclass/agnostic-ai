package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/builtins"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/preview"
)

const (
	mailboxSetupAuto     = "auto"
	mailboxSetupOff      = "off"
	mailboxSetupTrust    = "trust"
	mailboxSetupDisabled = "disabled"
	mailboxSetupUnknown  = "unknown"
)

const (
	mailboxDeliveryWatchLimit = 16
	mailboxDeliveryFileLimit  = 1 << 20
)

type mailboxDeliveryQuery struct {
	before time.Time
	search string
	tool   conversations.Tool
	editor string
}

type mailboxDeliveryInput struct {
	root string
	path string
}

func (input mailboxDeliveryInput) stamp() string {
	if input.root == "" {
		info, err := os.Lstat(input.path)
		return input.fileStamp(info, err)
	}
	root, err := os.OpenRoot(input.root)
	if err != nil {
		return input.fileStamp(nil, err)
	}
	defer func() { _ = root.Close() }()
	info, err := mailboxDeliveryFileInfo(root, input.path)
	return input.fileStamp(info, err)
}

func mailboxDeliveryFileInfo(root *os.Root, path string) (os.FileInfo, error) {
	if parent := filepath.Dir(path); parent != "." {
		dir, err := mailboxDeliveryFileInfo(root, parent)
		if err != nil {
			return nil, err
		}
		if !dir.IsDir() {
			return nil, fmt.Errorf("%s: expected a real directory", parent)
		}
	}
	return root.Lstat(path)
}

func (input mailboxDeliveryInput) fileStamp(info os.FileInfo, err error) string {
	prefix := fmt.Sprintf("%q:%q:", input.root, input.path)
	if err != nil {
		return prefix + fmt.Sprintf("%q;", err.Error())
	}
	return prefix + fmt.Sprintf("%d:%d:%d:%t;", info.Mode(), info.Size(), info.ModTime().UnixNano(), mailboxDeliveryOwned(info))
}

type mailboxDeliveryWatch struct {
	mu      sync.Mutex
	queries map[mailboxDeliveryQuery][]mailboxDeliveryInput
	order   []mailboxDeliveryQuery
}

func (w *mailboxDeliveryWatch) stamp(query mailboxDeliveryQuery) string {
	w.mu.Lock()
	paths := w.queries[query]
	w.mu.Unlock()
	var stamp strings.Builder
	for _, path := range paths {
		stamp.WriteString(path.stamp())
	}
	return stamp.String()
}

func (w *mailboxDeliveryWatch) remember(query mailboxDeliveryQuery, inputs map[mailboxDeliveryInput]string) string {
	paths := slices.Collect(maps.Keys(inputs))
	slices.SortFunc(paths, func(a, b mailboxDeliveryInput) int {
		if a.root != b.root {
			return strings.Compare(a.root, b.root)
		}
		return strings.Compare(a.path, b.path)
	})
	var stamp strings.Builder
	for _, path := range paths {
		stamp.WriteString(inputs[path])
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.queries == nil {
		w.queries = make(map[mailboxDeliveryQuery][]mailboxDeliveryInput)
	}
	if _, exists := w.queries[query]; !exists {
		if len(w.order) >= mailboxDeliveryWatchLimit {
			delete(w.queries, w.order[0])
			w.order = w.order[1:]
		}
		w.order = append(w.order, query)
	}
	w.queries[query] = paths
	return stamp.String()
}

type mailboxDeliveryFolder struct {
	folder string
	tool   conversations.Tool
}

type mailboxDeliveryChecks struct {
	setups map[mailboxDeliveryFolder]string
	inputs map[mailboxDeliveryInput]string
}

func newMailboxDeliveryChecks() mailboxDeliveryChecks {
	return mailboxDeliveryChecks{
		setups: make(map[mailboxDeliveryFolder]string),
		inputs: make(map[mailboxDeliveryInput]string),
	}
}

func (c *mailboxDeliveryChecks) watch(path string) {
	input := mailboxDeliveryInput{path: path}
	if _, seen := c.inputs[input]; !seen {
		c.inputs[input] = input.stamp()
	}
}

func (c *mailboxDeliveryChecks) fileInfo(root *os.Root, path string) (os.FileInfo, error) {
	input := mailboxDeliveryInput{root: root.Name(), path: path}
	info, err := mailboxDeliveryFileInfo(root, path)
	if _, seen := c.inputs[input]; !seen {
		c.inputs[input] = input.fileStamp(info, err)
	}
	return info, err
}

func (c *mailboxDeliveryChecks) read(root *os.Root, path string) ([]byte, error) {
	info, err := c.fileInfo(root, path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > mailboxDeliveryFileLimit {
		return nil, fmt.Errorf("%s: expected a regular file up to %d bytes", path, mailboxDeliveryFileLimit)
	}
	file, err := root.OpenFile(path, mailboxDeliveryReadFlags, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("%s: file changed before reading", path)
	}
	body, err := io.ReadAll(io.LimitReader(file, mailboxDeliveryFileLimit+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(body) > mailboxDeliveryFileLimit {
		return nil, fmt.Errorf("%s: file exceeds %d bytes", path, mailboxDeliveryFileLimit)
	}
	return body, nil
}

func (c *mailboxDeliveryChecks) readHome(path string) ([]byte, error) {
	dir := filepath.Dir(path)
	root, err := os.OpenRoot(dir)
	if os.IsNotExist(err) {
		c.watch(dir)
		return nil, nil
	}
	if err != nil {
		c.watch(dir)
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return c.read(root, filepath.Base(path))
}

func (c *mailboxDeliveryChecks) setup(receiver conversations.Conversation) string {
	if !canUseMailbox(receiver) || !filepath.IsAbs(receiver.Folder) {
		return mailboxSetupUnknown
	}
	key := mailboxDeliveryFolder{folder: receiver.Folder, tool: receiver.Tool}
	if setup, found := c.setups[key]; found {
		return setup
	}
	result := c.readSetup(key)
	c.setups[key] = result
	return result
}

func (c *mailboxDeliveryChecks) readSetup(key mailboxDeliveryFolder) string {
	folder, err := filepath.EvalSymlinks(key.folder)
	if err != nil {
		c.watch(key.folder)
		return mailboxSetupUnknown
	}
	if folder != key.folder {
		c.watch(key.folder)
	}
	checkout, ok := c.checkout(folder)
	if !ok {
		return mailboxSetupUnknown
	}
	root, err := os.OpenRoot(checkout)
	if err != nil {
		c.watch(checkout)
		return mailboxSetupUnknown
	}
	defer func() { _ = root.Close() }()
	projectBuiltins, readable := c.projectBuiltins(root, folder)
	if !readable {
		return mailboxSetupUnknown
	}
	source, err := userSourceRoot()
	if err != nil {
		return mailboxSetupUnknown
	}
	userBuiltins, ok := c.userBuiltins(source)
	if !ok {
		return mailboxSetupUnknown
	}
	enabled := slices.Contains(projectBuiltins, "mailbox") || slices.Contains(userBuiltins, "mailbox")
	var setup string
	if key.tool == conversations.ToolClaude {
		setup = c.claudeSetup(root, folder)
	} else {
		setup = c.codexSetup(root, folder)
	}
	if setup == mailboxSetupAuto && !enabled {
		return mailboxSetupUnknown
	}
	return setup
}

func (c *mailboxDeliveryChecks) checkout(folder string) (string, bool) {
	if info, err := os.Stat(folder); err != nil || !info.IsDir() {
		c.watch(folder)
		return "", false
	}
	for dir := folder; ; dir = filepath.Dir(dir) {
		marker := filepath.Join(dir, ".git")
		c.watch(marker)
		info, err := os.Lstat(marker)
		if err == nil {
			// A Git directory pointer can make native settings resolve outside this checkout.
			return dir, info.IsDir()
		}
		if !os.IsNotExist(err) {
			return "", false
		}
		if filepath.Dir(dir) == dir {
			return folder, true
		}
	}
}

func (c *mailboxDeliveryChecks) projectBuiltins(root *os.Root, folder string) ([]string, bool) {
	rel, err := filepath.Rel(root.Name(), folder)
	if err != nil {
		return nil, false
	}
	for dir := rel; ; dir = filepath.Dir(dir) {
		for _, name := range []string{config.ConfigFileName, config.LegacyConfigFileName} {
			path := filepath.Join(dir, name)
			body, err := c.read(root, path)
			if err != nil {
				return nil, false
			}
			if body == nil {
				continue
			}
			names, ok := mailboxBuiltinNames(body, path, nil)
			if !ok {
				return nil, false
			}
			local := filepath.Join(dir, config.LocalOverrideFileName)
			override, err := c.read(root, local)
			if err != nil {
				return nil, false
			}
			return mailboxBuiltinNames(override, local, names)
		}
		if dir == "." {
			return nil, true
		}
	}
}

func (c *mailboxDeliveryChecks) userBuiltins(source string) ([]string, bool) {
	root, err := os.OpenRoot(source)
	if err != nil {
		c.watch(source)
		return nil, os.IsNotExist(err)
	}
	defer func() { _ = root.Close() }()
	var names []string
	for _, path := range userConfigPaths(".") {
		body, err := c.read(root, path)
		if err != nil {
			return nil, false
		}
		var ok bool
		names, ok = mailboxBuiltinNames(body, path, names)
		if !ok {
			return nil, false
		}
	}
	return names, true
}

func mailboxBuiltinNames(body []byte, path string, names []string) ([]string, bool) {
	var doc map[string]yaml.Node
	if yaml.Unmarshal(body, &doc) != nil {
		return nil, false
	}
	if node, ok := doc["builtins"]; ok {
		var next []string
		if node.Decode(&next) != nil || validateBuiltinNames(next, path) != nil {
			return nil, false
		}
		return next, true
	}
	return names, true
}

const mailboxPromptEvent = "UserPromptSubmit"

type mailboxNativeHooks struct {
	DisableAllHooks       *bool `json:"disableAllHooks"`
	AllowManagedHooksOnly bool  `json:"allowManagedHooksOnly"`
	Hooks                 map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Type           string `json:"type"`
			Command        string `json:"command"`
			CommandWindows string `json:"commandWindows"`
			WindowsCommand string `json:"command_windows"`
			If             string `json:"if"`
			Async          bool   `json:"async"`
		} `json:"hooks"`
	} `json:"hooks"`
}

type mailboxPromptCommands struct {
	Command        string
	CommandWindows string                   `yaml:"commandWindows"`
	Codex          struct{ Command string } `yaml:"x-codex"`
}

var mailboxPromptSpec = sync.OnceValue(func() mailboxPromptCommands {
	var spec mailboxPromptCommands
	body, err := builtins.ReadFile("mailbox/hooks/mailbox-user-prompt.yaml")
	if err == nil && yaml.Unmarshal(body, &spec) == nil {
		return spec
	}
	return mailboxPromptCommands{}
})

func mailboxPromptCommand(command string, tool conversations.Tool) bool {
	command = strings.TrimSpace(command)
	command, _ = strings.CutPrefix(command, "export AGNOSTIC_AI_TARGET="+string(tool)+"; ")
	if command == "agnostic-ai project -- hook mailbox --target "+string(tool) {
		return true
	}
	spec := mailboxPromptSpec()
	switch {
	case tool == conversations.ToolCodex && runtime.GOOS == "windows":
		return command != "" && command == spec.CommandWindows
	case tool == conversations.ToolCodex:
		return command != "" && command == spec.Codex.Command
	default:
		return command != "" && command == spec.Command
	}
}

type mailboxHookSlot struct{ group, handler int }

func (h mailboxNativeHooks) promptHooks(tool conversations.Tool) map[mailboxHookSlot]bool {
	matched := map[mailboxHookSlot]bool{}
	for i, group := range h.Hooks[mailboxPromptEvent] {
		if tool == conversations.ToolClaude && group.Matcher != "" && group.Matcher != "*" {
			continue
		}
		for j, hook := range group.Hooks {
			command := hook.Command
			if runtime.GOOS == "windows" {
				switch {
				case hook.CommandWindows != "":
					command = hook.CommandWindows
				case hook.WindowsCommand != "":
					command = hook.WindowsCommand
				}
			}
			if hook.Type == "command" && hook.If == "" && !hook.Async && mailboxPromptCommand(command, tool) {
				matched[mailboxHookSlot{i, j}] = true
			}
		}
	}
	return matched
}

func parseMailboxNativeHooks(body []byte) (mailboxNativeHooks, bool) {
	var hooks *mailboxNativeHooks
	if body == nil {
		return mailboxNativeHooks{}, true
	}
	if json.Unmarshal(body, &hooks) != nil || hooks == nil {
		return mailboxNativeHooks{}, false
	}
	return *hooks, true
}

func (c *mailboxDeliveryChecks) claudeSetup(root *os.Root, folder string) string {
	user, ok := claudeUserSettingsPath()
	if !ok {
		return mailboxSetupUnknown
	}
	rel, err := filepath.Rel(root.Name(), folder)
	if err != nil {
		return mailboxSetupUnknown
	}
	local := filepath.Join(".claude", "settings.local.json")
	paths := []string{user, filepath.Join(rel, ".claude", "settings.json"), filepath.Join(rel, local)}
	if rel != "." {
		atRoot, ok := c.claudeLocalAtRoot(root)
		if !ok {
			return mailboxSetupUnknown
		}
		if atRoot {
			paths = append(paths, local)
		}
	}
	var disabled, installed bool
	for _, path := range paths {
		var data []byte
		var err error
		if path == user {
			data, err = c.readHome(path)
		} else {
			data, err = c.read(root, path)
		}
		if err != nil {
			return mailboxSetupUnknown
		}
		if data != nil {
			data, _ = preview.StripJSONC(data)
		}
		hooks, ok := parseMailboxNativeHooks(data)
		if !ok || hooks.AllowManagedHooksOnly {
			return mailboxSetupUnknown
		}
		if hooks.DisableAllHooks != nil {
			disabled = *hooks.DisableAllHooks
		}
		installed = installed || len(hooks.promptHooks(conversations.ToolClaude)) > 0
	}
	switch {
	case disabled:
		return mailboxSetupDisabled
	case installed:
		return mailboxSetupAuto
	default:
		return mailboxSetupOff
	}
}

func (c *mailboxDeliveryChecks) claudeLocalAtRoot(root *os.Root) (atRoot, known bool) {
	if runtime.GOOS == "windows" {
		return false, true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false, false
	}
	if samePath(root.Name(), resolvedPath(home)) {
		return false, true
	}
	for _, path := range []string{".", ".git", ".claude"} {
		info, err := c.fileInfo(root, path)
		if os.IsNotExist(err) {
			return false, true
		}
		if err != nil || !info.IsDir() {
			return false, false
		}
		if !mailboxDeliveryOwned(info) {
			return false, true
		}
	}
	return true, true
}

type mailboxCodexConfig struct {
	Features struct {
		Hooks  *bool `toml:"hooks"`
		Legacy *bool `toml:"codex_hooks"`
	}
	Projects map[string]struct {
		Trust string `toml:"trust_level"`
	}
	Profile string
	Hooks   map[string]toml.Primitive
}

func (cfg mailboxCodexConfig) hooksEnabled() *bool {
	if cfg.Features.Hooks != nil {
		return cfg.Features.Hooks
	}
	return cfg.Features.Legacy
}

func (cfg mailboxCodexConfig) trusts(root string) bool {
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	for dir := root; ; dir = filepath.Dir(dir) {
		if project, found := cfg.Projects[dir]; found && project.Trust != "" {
			return project.Trust == "trusted"
		}
		if filepath.Dir(dir) == dir {
			return false
		}
	}
}

func parseMailboxCodexConfig(data []byte) (mailboxCodexConfig, bool) {
	var cfg mailboxCodexConfig
	if _, err := toml.Decode(string(data), &cfg); err != nil || cfg.Profile != "" {
		return cfg, false
	}
	for key := range cfg.Hooks {
		if key != "state" {
			return cfg, false
		}
	}
	return cfg, true
}

func (c *mailboxDeliveryChecks) codexSetup(root *os.Root, folder string) string {
	userPath, err := codex.UserHooksPath()
	if err != nil {
		if home := os.Getenv("CODEX_HOME"); home != "" {
			c.watch(home)
		}
		return mailboxSetupUnknown
	}
	userBody, err := c.readHome(filepath.Join(filepath.Dir(userPath), "config.toml"))
	user, ok := parseMailboxCodexConfig(userBody)
	if err != nil || !ok {
		return mailboxSetupUnknown
	}
	trusted := user.trusts(root.Name())
	if user.trusts(folder) != trusted {
		return mailboxSetupUnknown
	}
	rel, err := filepath.Rel(root.Name(), folder)
	if err != nil {
		return mailboxSetupUnknown
	}
	for dir := rel; dir != "."; dir = filepath.Dir(dir) {
		if _, err := c.fileInfo(root, filepath.Join(dir, ".codex")); !os.IsNotExist(err) {
			return mailboxSetupUnknown
		}
	}
	enabled := user.hooksEnabled()
	if trusted {
		body, err := c.read(root, filepath.Join(".codex", "config.toml"))
		project, ok := parseMailboxCodexConfig(body)
		if err != nil || !ok {
			return mailboxSetupUnknown
		}
		if local := project.hooksEnabled(); local != nil {
			enabled = local
		}
	}
	if enabled != nil && !*enabled {
		return mailboxSetupDisabled
	}
	userHooks, err := c.readHome(userPath)
	if err != nil {
		return mailboxSetupUnknown
	}
	userSetup := mailboxCodexHookSetup(userPath, userHooks, userBody)
	if userSetup == mailboxSetupAuto {
		return userSetup
	}
	projectPath := filepath.Join(".codex", "hooks.json")
	projectHooks, err := c.read(root, projectPath)
	if err != nil {
		return mailboxSetupUnknown
	}
	projectSetup := mailboxCodexHookSetup(filepath.Join(root.Name(), projectPath), projectHooks, userBody)
	if projectSetup == mailboxSetupAuto && !trusted {
		projectSetup = mailboxSetupTrust
	}
	for _, setup := range []string{mailboxSetupAuto, mailboxSetupUnknown, mailboxSetupTrust, mailboxSetupDisabled} {
		if userSetup == setup || projectSetup == setup {
			return setup
		}
	}
	return mailboxSetupOff
}

func mailboxCodexHookSetup(path string, body, userConfig []byte) string {
	hooks, ok := parseMailboxNativeHooks(body)
	if !ok {
		return mailboxSetupUnknown
	}
	matched := hooks.promptHooks(conversations.ToolCodex)
	if len(matched) == 0 {
		return mailboxSetupOff
	}
	findings, err := codex.HookTrustFindingsWithConfig(path, body, userConfig)
	if err != nil {
		return mailboxSetupUnknown
	}
	setup := mailboxSetupDisabled
	for _, finding := range findings {
		slot := mailboxHookSlot{finding.Group, finding.Handler}
		if finding.Event != mailboxPromptEvent || !matched[slot] {
			continue
		}
		delete(matched, slot)
		if finding.Status != "disabled" {
			setup = mailboxSetupTrust
		}
	}
	if len(matched) > 0 {
		return mailboxSetupAuto
	}
	return setup
}
