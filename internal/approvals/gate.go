// Package approvals decides whether a tool call a coding tool is about
// to prompt for may be approved without the person.
//
// The gate approves a shell command only from a short table of
// programs, verbs and options, and asks for anything else. It is a seat
// belt, not a sandbox: a recursive search such as grep -r or rg can
// print a credential file inside the project, and a project's own
// tests, Makefile targets and package scripts run whatever they hold.
package approvals

import (
	"encoding/json"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/chemaclass/agnostic-ai/internal/applypatch"
	"github.com/chemaclass/agnostic-ai/internal/hookrun"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Outcome is what the gate decided for one request.
type Outcome string

const (
	// Approved lets the hook allow this one call.
	Approved Outcome = "approved"
	// Asked leaves the call to the person: the tool shows its prompt.
	Asked Outcome = "asked"
)

// Rules name the always-ask row that sent a request to the person.
const (
	RuleMode           = "mode"
	RulePermissionMode = "permission-mode"
	RuleTool           = "tool"
	RuleMCP            = "mcp"
	RuleWeb            = "web"
	RuleInput          = "input"
	RuleShellSyntax    = "shell-syntax"
	RuleDelete         = "delete"
	RulePublish        = "publish"
	RuleNetwork        = "network"
	RuleSecret         = "secret"
	RulePrivilege      = "privilege"
	RuleOutsideProject = "outside-project"
	RuleProtectedPath  = "protected-path"
	RulePolicy         = "approvals-policy"
	RuleAlwaysAsk      = "always-ask"
	RuleUnknownCommand = "unknown-command"
)

// ModeYolo is the only mode in which the gate approves.
const ModeYolo = "yolo"

// Decision is the gate's answer and, when it asks, the rule that matched.
type Decision struct {
	Outcome Outcome `json:"decision"`
	Rule    string  `json:"rule,omitempty"`
}

// Request is one tool call as the permission hook reads it.
type Request struct {
	// Call is the tool name the coding tool gives, such as Bash, Edit or
	// mcp__server__action.
	Call string
	// Input is the call's tool_input.
	Input map[string]any
	// PermissionMode is the session's permission mode, when the payload says.
	PermissionMode string
	// Cwd is the session's working directory; relative paths resolve
	// against it.
	Cwd string
}

// Policy is what the gate needs to know about the person's setup.
type Policy struct {
	// Mode is the resolved approvals mode; only yolo approves.
	Mode string
	// ProjectRoot is the absolute root of the project the session runs in.
	ProjectRoot string
	// Home is the person's home folder, for ~ and secret folders.
	Home string
	// AgnosticHome is $AGNOSTIC_AI_HOME; edits under it always ask.
	AgnosticHome string
	// Protected are the settings specs' protected blocks.
	Protected []spec.ProtectGroup
	// SpecSources, when set, returns the absolute folders that the
	// project config files in one folder move agent, hook, MCP or
	// settings specs to. The gate calls it for each folder above a path
	// a call writes; a write under a returned folder asks, and an error
	// asks. SpecSources is the one a hook passes.
	SpecSources func(dir string) ([]string, error)
	// AlwaysAsk are permission rules, such as Bash(make deploy*), that
	// ask on a match, read the way Claude Code reads its permission
	// rules. The caller passes the person's own ask rules from the tool's
	// settings, since a hook allow skips them (decision 022), and the
	// approvals.always-ask list. They add to the fixed rules, never remove.
	AlwaysAsk []string
	// RuleRoots are more folders an AlwaysAsk path rule may be anchored
	// at, such as the session folder and the folder of the settings file
	// a rule came from. A rule asks when it matches from any of them or
	// from the project root.
	RuleRoots []string
	// Resolve, when set, returns a path with its symbolic links
	// resolved. Without it the gate compares paths as written.
	Resolve func(string) string
}

func asked(rule string) Decision {
	return Decision{Outcome: Asked, Rule: rule}
}

var approved = Decision{Outcome: Approved}

// shellCalls, editCalls and readCalls name the tools the gate reads;
// every other tool asks.
var (
	shellCalls = map[string]bool{"Bash": true, "Shell": true, "shell": true, "local_shell": true, "exec_command": true, "run_shell_command": true}
	editCalls  = map[string]string{"Edit": "file_path", "Write": "file_path", "MultiEdit": "file_path", "NotebookEdit": "notebook_path"}
	readCalls  = map[string]bool{"Read": true, "Glob": true, "Grep": true, "LS": true, "NotebookRead": true}
	webCalls   = map[string]bool{"WebFetch": true, "WebSearch": true, "web_search": true, "web_fetch": true}
	patchCalls = map[string]bool{"apply_patch": true}
)

// Decide returns Approved only when policy is yolo, the request is one
// the gate reads in full, and no always-ask rule matches. Anything it
// cannot read asks. It does no I/O unless policy.Resolve or
// policy.SpecSources does.
func Decide(r Request, policy Policy) Decision {
	if policy.Mode != ModeYolo {
		return asked(RuleMode)
	}
	if !slices.Contains(approvingPermissionModes, r.PermissionMode) {
		return asked(RulePermissionMode)
	}
	switch {
	case isMCP(r.Call):
		return asked(RuleMCP)
	case webCalls[r.Call]:
		return asked(RuleWeb)
	case !shellCalls[r.Call] && editCalls[r.Call] == "" && !readCalls[r.Call] && !patchCalls[r.Call]:
		return asked(RuleTool)
	}
	g, ok := newGate(r, policy)
	if !ok {
		return asked(RuleInput)
	}
	if g.holdsHome() {
		return asked(RuleOutsideProject)
	}
	if rule := g.alwaysAsk(r); rule != "" {
		return asked(rule)
	}
	var rule string
	switch {
	case shellCalls[r.Call]:
		command, isText := r.Input["command"].(string)
		if !isText || strings.TrimSpace(command) == "" || !onlyKeys(r.Input, shellInputKeys) {
			return asked(RuleInput)
		}
		rule = g.shell(command)
	case editCalls[r.Call] != "":
		rule = g.editCall(r.Input, editCalls[r.Call])
	case patchCalls[r.Call]:
		rule = g.patch(r.Input)
	default:
		rule = g.readCall(r.Call, r.Input)
	}
	if rule != "" {
		return asked(rule)
	}
	return approved
}

// approvingPermissionModes are the session modes in which a prompt
// means the person would be asked: manual and accept-edits (decision
// 022), or no mode in the payload. In auto or plan the reviewer or a
// rule chose to ask; any other mode, such as dontAsk, was not measured.
var approvingPermissionModes = []string{"", "default", "acceptEdits"}

// shellInputKeys are the shell input fields the gate understands. Any
// other, such as a working folder or a sandbox switch, changes what the
// command does, so it asks.
var shellInputKeys = []string{"command", "description", "timeout", "run_in_background"}

func onlyKeys(input map[string]any, keys []string) bool {
	for key := range input {
		if !slices.Contains(keys, key) {
			return false
		}
	}
	return true
}

func isMCP(call string) bool {
	lower := strings.ToLower(call)
	return strings.HasPrefix(lower, "mcp__") || strings.HasPrefix(lower, "mcp:") || strings.HasPrefix(lower, "mcp.")
}

// gate holds one request's resolved folders.
type gate struct {
	policy    Policy
	roots     []string
	cwd       string
	home      string
	specCache map[string]specFolders
}

func newGate(r Request, policy Policy) (*gate, bool) {
	root := policy.ProjectRoot
	if !filepath.IsAbs(root) || policy.Home == "" || !filepath.IsAbs(policy.Home) {
		return nil, false
	}
	g := &gate{policy: policy, home: filepath.Clean(policy.Home)}
	g.roots = []string{filepath.Clean(root)}
	if policy.Resolve != nil {
		if real := filepath.Clean(policy.Resolve(root)); real != g.roots[0] {
			g.roots = append(g.roots, real)
		}
	}
	cwd := r.Cwd
	if cwd == "" {
		cwd = root
	}
	if !filepath.IsAbs(cwd) {
		return nil, false
	}
	g.cwd = filepath.Clean(cwd)
	if !g.inside(g.cwd) {
		return nil, false
	}
	return g, true
}

// alwaysAsk matches the person's extra rules and the commands and
// paths that name the approvals policy itself.
func (g *gate) alwaysAsk(r Request) string {
	if command, ok := r.Input["command"].(string); ok && namesPolicy(command) {
		return RulePolicy
	}
	if len(g.policy.AlwaysAsk) == 0 {
		return ""
	}
	if searchCalls[r.Call] && slices.ContainsFunc(g.policy.AlwaysAsk, isReadPathRule) {
		return RuleAlwaysAsk
	}
	// A shell command can read or write any file, and the gate cannot
	// list them all, so any Read or Edit rule with a path asks.
	if shellCalls[r.Call] && slices.ContainsFunc(g.policy.AlwaysAsk, isFilePathRule) {
		return RuleAlwaysAsk
	}
	forms, ok := g.ruleCalls(r)
	if !ok {
		return RuleAlwaysAsk
	}
	roots := []string{g.roots[0]}
	for _, root := range g.policy.RuleRoots {
		if filepath.IsAbs(root) && !slices.Contains(roots, filepath.Clean(root)) {
			roots = append(roots, filepath.Clean(root))
		}
	}
	for _, rule := range g.policy.AlwaysAsk {
		rule = asEditRule(rule)
		for _, form := range forms {
			for _, root := range roots {
				rule, root := rule, root
				if form.fold {
					rule, root = foldRule(rule), strings.ToLower(root)
				}
				runs, err := hookrun.ClaudeIfRuns(rule, "PermissionRequest", form.body, root)
				if err != nil || runs {
					return RuleAlwaysAsk
				}
			}
		}
	}
	return ""
}

// ruleForm is one form of a request a permission rule is matched
// against. A fold form has its path in lower case, and is matched with
// the rule's pattern and the root in lower case too.
type ruleForm struct {
	body []byte
	fold bool
}

// foldRule is rule with the pattern inside its parentheses in lower case.
func foldRule(rule string) string {
	tool, pattern, ok := strings.Cut(rule, "(")
	if !ok {
		return rule
	}
	return tool + "(" + strings.ToLower(pattern)
}

// holdsHome reports a project root at or above the home folder, as when
// a session runs in the home folder outside any Git checkout: every file
// of the person would count as the project.
func (g *gate) holdsHome() bool {
	for _, root := range g.roots {
		if underFold(g.home, root) {
			return true
		}
	}
	return false
}

// ruleCalls are the forms of r a permission rule is matched against: the
// call as sent, the shell command with its rtk prefixes removed, and an
// Edit call for each path an edit or patch writes and a Read call for
// each path a read tool reads, since Claude Code applies Edit rules to
// every file edit tool and Read rules to every read tool. Each path is
// added as written, with links resolved, and in lower case, since macOS
// and Windows disks ignore case. ok is false when a form does not encode.
func (g *gate) ruleCalls(r Request) ([]ruleForm, bool) {
	calls := []map[string]any{{"tool_name": r.Call, "tool_input": r.Input}}
	var folded []map[string]any
	if command, ok := r.Input["command"].(string); ok && shellCalls[r.Call] {
		if plain := rtkPrefix.ReplaceAllString(command, "$1$2"); plain != command {
			input := maps.Clone(r.Input)
			input["command"] = plain
			calls = append(calls, map[string]any{"tool_name": r.Call, "tool_input": input})
		}
	}
	addPath := func(tool, p string) {
		abs, ok := g.path(p, g.cwd)
		if !ok {
			return
		}
		paths := []string{abs}
		if g.policy.Resolve != nil {
			if real := filepath.Clean(g.policy.Resolve(abs)); real != abs {
				paths = append(paths, real)
			}
		}
		for _, file := range paths {
			calls = append(calls, map[string]any{"tool_name": tool, "tool_input": map[string]any{"file_path": file}})
			folded = append(folded, map[string]any{"tool_name": tool, "tool_input": map[string]any{"file_path": strings.ToLower(file)}})
		}
	}
	switch {
	case editCalls[r.Call] != "":
		file, _ := r.Input[editCalls[r.Call]].(string)
		addPath("Edit", file)
	case patchCalls[r.Call]:
		for _, field := range []string{"patch", "input", "command"} {
			if body, ok := r.Input[field].(string); ok {
				for _, op := range applypatch.Parse(body) {
					addPath("Edit", op.Path)
					if op.MoveTo != "" {
						addPath("Edit", op.MoveTo)
					}
				}
			}
		}
	case readCalls[r.Call]:
		for _, field := range []string{"file_path", "notebook_path"} {
			if p, ok := r.Input[field].(string); ok && p != "" {
				addPath("Read", p)
			}
		}
	}
	forms := make([]ruleForm, 0, len(calls)+len(folded))
	for i, call := range append(calls, folded...) {
		body, err := json.Marshal(call)
		if err != nil {
			return nil, false
		}
		forms = append(forms, ruleForm{body: body, fold: i >= len(calls)})
	}
	return forms, true
}

// rtkPrefix finds an rtk word that runs the program after it, at the
// start of the text or of a command in a list or pipeline.
var rtkPrefix = regexp.MustCompile(`(^|[;&|(]\s*)rtk\s+(` + strings.Join(rtkPassThrough, "|") + `)\b`)

// searchCalls read every file under a folder, so a Read rule with a
// path may cover any of them.
var searchCalls = map[string]bool{"Glob": true, "Grep": true, "LS": true}

func isReadPathRule(rule string) bool {
	rest, ok := strings.CutPrefix(strings.TrimSpace(rule), "Read(")
	return ok && strings.TrimSpace(strings.TrimSuffix(rest, ")")) != "*"
}

// isFilePathRule reports a Read rule, or a rule of any file edit tool,
// with a path.
func isFilePathRule(rule string) bool {
	if isReadPathRule(rule) {
		return true
	}
	rest, ok := strings.CutPrefix(asEditRule(strings.TrimSpace(rule)), "Edit(")
	return ok && strings.TrimSpace(strings.TrimSuffix(rest, ")")) != "*"
}

// asEditRule reads a Write, MultiEdit or NotebookEdit rule with a path
// as the Edit rule Claude Code applies to every file edit tool.
func asEditRule(rule string) string {
	for _, tool := range []string{"Write(", "MultiEdit(", "NotebookEdit("} {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(rule), tool); ok {
			return "Edit(" + rest
		}
	}
	return rule
}

func namesPolicy(text string) bool {
	unquoted := strings.NewReplacer(`\`, "", `'`, "", `"`, "").Replace(strings.ToLower(text))
	return strings.Contains(unquoted, "agnostic_ai_")
}

// path is a request path made absolute: ~ from home, relative from cwd.
// ok is false for a path the gate does not read: a Windows drive or
// share path, a backslash path that climbs, a ~user path, or an empty
// one. Elsewhere a backslash is a plain character on the systems the
// hook runs on.
func (g *gate) path(p, cwd string) (string, bool) {
	if p == "" || isWindowsPath(p) || strings.HasPrefix(p, `\\`) || slices.Contains(strings.Split(p, `\`), "..") && strings.ContainsRune(p, '\\') {
		return "", false
	}
	switch {
	case p == "~":
		return g.home, true
	case strings.HasPrefix(p, "~/"):
		return g.join(g.home, p[2:]), true
	case strings.HasPrefix(p, "~"):
		return "", false
	case filepath.IsAbs(p):
		return g.join(string(filepath.Separator), p), true
	}
	return g.join(cwd, p), true
}

// join walks rel from base. When the policy resolves links, a ".."
// steps up from where the path so far really is, as the system does:
// with vendor linked elsewhere, vendor/.. is not the project root.
func (g *gate) join(base, rel string) string {
	if g.policy.Resolve == nil || !slices.Contains(strings.Split(filepath.ToSlash(rel), "/"), "..") {
		return filepath.Join(base, rel)
	}
	cur := filepath.Clean(base)
	for _, segment := range strings.Split(filepath.ToSlash(rel), "/") {
		switch segment {
		case "", ".":
		case "..":
			cur = filepath.Dir(filepath.Clean(g.policy.Resolve(cur)))
		default:
			cur = filepath.Join(cur, segment)
		}
	}
	return cur
}

func isWindowsPath(p string) bool {
	return len(p) >= 2 && p[1] == ':' && (p[0] >= 'a' && p[0] <= 'z' || p[0] >= 'A' && p[0] <= 'Z')
}

// inside reports whether abs sits in the project: once its links are
// resolved when the policy resolves them, else as written.
func (g *gate) inside(abs string) bool {
	if g.policy.Resolve == nil {
		return g.within(abs)
	}
	return g.within(filepath.Clean(g.policy.Resolve(abs)))
}

func (g *gate) within(abs string) bool {
	for _, root := range g.roots {
		if under(abs, root) {
			return true
		}
	}
	return false
}

func under(p, dir string) bool {
	if p == dir {
		return true
	}
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../") && !filepath.IsAbs(rel)
}

// relative is abs relative to the project root it sits in.
func (g *gate) relative(abs string) string {
	for _, root := range g.roots {
		if under(abs, root) {
			rel, _ := filepath.Rel(root, abs)
			return filepath.ToSlash(rel)
		}
	}
	return ""
}

// targetRule checks one path a call writes: outside the project,
// a secret, or a protected path asks.
func (g *gate) targetRule(p, cwd, outsideRule string) string {
	abs, ok := g.path(p, cwd)
	if !ok {
		return RuleOutsideProject
	}
	if isDevice(abs) {
		return ""
	}
	if g.secret(abs) {
		return RuleSecret
	}
	if !g.inside(abs) {
		return outsideRule
	}
	if g.protected(abs) || g.protectedBelow(g.relative(abs)) {
		return RuleProtectedPath
	}
	if rule := g.specRule(abs); rule != "" {
		return rule
	}
	return ""
}

// readRule checks one path a call reads: outside the project or a
// secret asks.
func (g *gate) readRule(p, cwd string) string {
	abs, ok := g.path(p, cwd)
	if !ok {
		return RuleOutsideProject
	}
	if isDevice(abs) {
		return ""
	}
	if g.secret(abs) {
		return RuleSecret
	}
	if !g.inside(abs) {
		return RuleOutsideProject
	}
	if g.gitInternal(abs) {
		return RuleProtectedPath
	}
	return ""
}

// gitInternals are the Git folders a read asks for: their config can
// hold a token in a remote URL.
var gitInternals = spec.ProtectGroup{Paths: []string{"**/.git"}}

func (g *gate) gitInternal(abs string) bool {
	paths := []string{abs}
	if g.policy.Resolve != nil {
		paths = append(paths, filepath.Clean(g.policy.Resolve(abs)))
	}
	for _, p := range paths {
		if rel := g.relative(p); rel != "" && rel != "." {
			if _, ok := gitInternals.Match(rel); ok {
				return true
			}
		}
	}
	return false
}

func isDevice(abs string) bool {
	switch abs {
	case "/dev/null", "/dev/stdin", "/dev/stdout", "/dev/stderr", "/dev/tty":
		return true
	}
	return false
}

func (g *gate) editCall(input map[string]any, field string) string {
	file, _ := input[field].(string)
	if file == "" {
		return RuleInput
	}
	return g.targetRule(file, g.cwd, RuleOutsideProject)
}

func (g *gate) readCall(call string, input map[string]any) string {
	paths := []string{}
	for _, field := range []string{"file_path", "notebook_path", "path"} {
		if p, ok := input[field].(string); ok && p != "" {
			paths = append(paths, p)
		}
	}
	if call == "Glob" {
		pattern, _ := input["pattern"].(string)
		if pattern == "" {
			return RuleInput
		}
		if strings.HasPrefix(pattern, "/") || strings.HasPrefix(pattern, "~") || strings.Contains(pattern, "..") {
			paths = append(paths, globBase(pattern))
		}
	}
	if call == "Read" || call == "NotebookRead" {
		if len(paths) == 0 {
			return RuleInput
		}
	}
	if glob, ok := input["glob"].(string); ok && glob != "" {
		if rule := g.searchGlobRule(glob, g.cwd); rule != "" {
			return rule
		}
	}
	for _, p := range paths {
		if rule := g.readRule(p, g.cwd); rule != "" {
			return rule
		}
	}
	return ""
}

// globBase is the part of a glob pattern before its first wildcard, or
// the whole pattern when a parent step follows a wildcard.
func globBase(pattern string) string {
	i := strings.IndexAny(pattern, "*?[{")
	if i < 0 {
		return pattern
	}
	if strings.Contains(pattern[i:], "..") {
		return "/"
	}
	base := pattern[:i]
	if j := strings.LastIndex(base, "/"); j >= 0 {
		return base[:j+1]
	}
	return "."
}

// patch reads an apply_patch body: each file it adds, updates or moves
// to must pass as an edit; a deleted file asks. A header line the gate
// does not read exactly, such as an indented one, asks.
func (g *gate) patch(input map[string]any) string {
	var body string
	for _, field := range []string{"patch", "input", "command"} {
		if text, ok := input[field].(string); ok && text != "" {
			if body != "" {
				return RuleInput
			}
			body = text
		}
	}
	if !strings.Contains(body, "*** Begin Patch") {
		return RuleInput
	}
	known := []string{"*** Begin Patch", "*** End Patch", "*** End of File", "*** Add File: ", "*** Update File: ", "*** Delete File: ", "*** Move to: "}
	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "***") {
			continue
		}
		if trimmed != strings.TrimRightFunc(line, unicode.IsSpace) || !slices.ContainsFunc(known, func(h string) bool { return strings.HasPrefix(trimmed, h) }) {
			return RuleInput
		}
	}
	ops := applypatch.Parse(body)
	if len(ops) == 0 {
		return RuleInput
	}
	for _, op := range ops {
		if op.Kind == applypatch.Delete {
			return RuleDelete
		}
		for _, file := range []string{op.Path, op.MoveTo} {
			if file == "" {
				continue
			}
			if rule := g.targetRule(file, g.cwd, RuleOutsideProject); rule != "" {
				return rule
			}
		}
	}
	return ""
}
