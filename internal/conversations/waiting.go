package conversations

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// waitKind is what a conversation that waits on the person waits for.
type waitKind string

const (
	// waitPermission is a tool call the person must approve.
	waitPermission waitKind = "permission"
	// waitAnswer is a question for the person.
	waitAnswer waitKind = "answer"
	waitShell  waitKind = "shell"
)

// waitingOn says what a waiting conversation waits for, as its tool
// reports it. A row whose tool does not say has none.
type waitingOn struct {
	For waitKind `json:"for"`
	// Tool is the tool call that waits for approval, as the tool names
	// it; set only for a permission whose call the transcript shows.
	Tool string `json:"tool,omitempty"`
	// Detail is what that call acts on, short enough for a row: see
	// callDetails. Left out when it may hold a credential.
	Detail string `json:"detail,omitempty"`
	// since is when the tool began to wait, when it says.
	since time.Time
	// called is when the transcript recorded the call that waits; zero
	// when unknown.
	called time.Time
}

// CalledAt is when the transcript recorded the call that waits, zero
// when it is unknown.
func (w *waitingOn) CalledAt() time.Time {
	if w == nil {
		return time.Time{}
	}
	return w.called
}

// PermissionWait reports whether the conversation waits for the person
// to approve a tool call and, when its tool says, since when.
func (c Conversation) PermissionWait() (since time.Time, waits bool) {
	if c.Waiting == nil || c.Waiting.For != waitPermission {
		return time.Time{}, false
	}
	return c.Waiting.since, true
}

// HoldForPermission marks live row c as waiting for the person to
// approve call, which acts on detail, since since: a hook holds the
// request for the companion. Codex writes nothing while a hook holds
// its prompt, so the held request is its only wait signal. It reports
// whether the row newly counts in View.NeedsYou.
func (c *Conversation) HoldForPermission(call, detail string, since time.Time) bool {
	counted := needsPerson(c.State)
	c.Activity = ActivityWaiting
	c.State = StateBlocked
	c.Waiting = &waitingOn{For: waitPermission, Tool: call, Detail: detail, since: since}
	return !counted
}

// claudeWaitKinds maps the waitingFor a live Claude Code session writes
// to sessions/<pid>.json. Claude Code writes "permission prompt" for a
// tool call it asks about, "sandbox request" for a sandboxed command
// that asks for network access, and "input needed" for a question
// (AskUserQuestion or an MCP server's form). Any other value, such as
// "dialog open", says nothing about the kind.
var claudeWaitKinds = map[string]waitKind{
	"permission prompt": waitPermission,
	"sandbox request":   waitPermission,
	"input needed":      waitAnswer,
}

// codexCloudWaitKinds maps a Codex cloud thread's active flags.
var codexCloudWaitKinds = map[string]waitKind{
	"waitingOnApproval":  waitPermission,
	"waitingOnUserInput": waitAnswer,
}

// codexCloudWaiting is what a cloud thread's flags say it waits for. An
// approval wins over a question: it blocks the thread's work.
func codexCloudWaiting(flags []string) *waitingOn {
	var found *waitingOn
	for _, flag := range flags {
		kind, known := codexCloudWaitKinds[flag]
		if !known {
			continue
		}
		if kind == waitPermission {
			return &waitingOn{For: waitPermission}
		}
		found = &waitingOn{For: kind}
	}
	return found
}

// toolCall is a tool call the agent made, with the parts of its input
// a detail is read from.
type toolCall struct {
	id    string
	name  string
	input toolInput
	// at is when the transcript recorded the call; zero when unknown.
	at time.Time
}

// toolInput holds the input fields callDetails reads; the rest of the
// input, such as a file's new content, is never kept.
type toolInput struct {
	Command      string `json:"command"`
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
	URL          string `json:"url"`
}

// callInput reads the fields callDetails needs from the input of a
// tool it lists. Another tool's input is never read: an MCP tool may
// send a "command" that is no string. An input of another shape than
// the tool's own gives no detail.
func callInput(tool string, raw json.RawMessage) toolInput {
	var input toolInput
	if _, listed := callDetails[tool]; !listed || json.Unmarshal(raw, &input) != nil {
		return toolInput{}
	}
	return input
}

// detailSource picks the input field a detail is read from.
type detailSource func(toolInput) string

// detailRule is how one tool's detail is read: the input field, and
// how that field is shortened for a row.
type detailRule struct {
	source  detailSource
	shorten func(string) string
}

// callDetails maps a Claude Code tool to its detail rule. A tool not
// listed, other than an MCP tool, has no detail.
var callDetails = map[string]detailRule{
	"Bash":         {source: func(in toolInput) string { return in.Command }, shorten: commandStart},
	"Edit":         {source: func(in toolInput) string { return in.FilePath }, shorten: fileName},
	"MultiEdit":    {source: func(in toolInput) string { return in.FilePath }, shorten: fileName},
	"Write":        {source: func(in toolInput) string { return in.FilePath }, shorten: fileName},
	"Read":         {source: func(in toolInput) string { return in.FilePath }, shorten: fileName},
	"NotebookEdit": {source: func(in toolInput) string { return in.NotebookPath }, shorten: fileName},
	"WebFetch":     {source: func(in toolInput) string { return in.URL }, shorten: linkHost},
}

// commandRunes is about how much of a command a row shows.
const commandRunes = 30

// mcpDetailRunes bounds an MCP tool's server and action.
const mcpDetailRunes = 40

// leadingCd is a first step that only changes folder, as in
// "cd /Users/me/app && make test": the row shows the step after it.
// The folder stops at a shell separator, so "cd /tmp;./deploy" keeps
// "./deploy".
var leadingCd = regexp.MustCompile(`^cd\s+("[^"]*"|'[^']*'|[^\s;&|]+)\s*(&&|;)\s*`)

// commandStart is the first words of a command that fit in about
// commandRunes, with "…" when words were cut, after a leading cd step.
// When whole words would leave less than half of that, as for a long
// path, the cut falls inside a word.
func commandStart(command string) string {
	line := oneLine(command)
	if rest := leadingCd.ReplaceAllString(line, ""); rest != "" {
		line = rest
	}
	if utf8.RuneCountInString(line) <= commandRunes {
		return line
	}
	runes := []rune(line)
	cut := strings.LastIndexFunc(string(runes[:commandRunes+1]), unicode.IsSpace)
	if utf8.RuneCountInString(line[:max(cut, 0)]) < commandRunes/2 {
		return truncate(line, commandRunes)
	}
	return strings.TrimSpace(line[:cut]) + "…"
}

// fileName is a path's last name.
func fileName(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return filepath.Base(path)
}

// linkHost is a link's host, without a port or a sign-in.
func linkHost(link string) string {
	parsed, err := url.Parse(strings.TrimSpace(link))
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

// mcpToolPrefix starts the name Claude Code gives an MCP server's tool:
// mcp__<server>__<action>.
const mcpToolPrefix = "mcp__"

// uuidName is a server registered by id, as a claude.ai connector can be.
var uuidName = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// mcpServerPrefixes are the prefixes Claude Code adds to a server's
// name: a claude.ai connector, and a plugin's server, named
// plugin_<plugin>_<server>.
const (
	claudeConnectorPrefix = "claude_ai_"
	pluginServerPrefix    = "plugin_"
)

// mcpDetail is an MCP tool's server and action as a row says them:
// "mcp__claude_ai_Notion__notion-fetch" is "Notion fetch". A server
// registered by id is named by the action's first word, so
// "mcp__<uuid>__notion-fetch" is "Notion fetch" too and the id never
// shows. A name that is not an MCP tool's has none.
func mcpDetail(tool string) string {
	rest, isMCP := strings.CutPrefix(tool, mcpToolPrefix)
	if !isMCP {
		return ""
	}
	server, action, found := strings.Cut(rest, "__")
	if !found || server == "" || action == "" {
		return ""
	}
	actionWords := strings.Fields(strings.NewReplacer("_", " ", "-", " ").Replace(action))
	if len(actionWords) == 0 {
		return ""
	}
	var serverName string
	switch {
	case uuidName.MatchString(server) && len(actionWords) > 1:
		serverName, actionWords = actionWords[0], actionWords[1:]
	case uuidName.MatchString(server):
		// One word names no server: the action alone shows, never the id.
		serverName = ""
	case strings.HasPrefix(server, claudeConnectorPrefix):
		serverName = strings.TrimPrefix(server, claudeConnectorPrefix)
	case strings.HasPrefix(server, pluginServerPrefix):
		// The plugin's own name may hold underscores; the server's is the last part.
		serverName = server[strings.LastIndex(server, "_")+1:]
	default:
		serverName = server
	}
	serverName = strings.ReplaceAll(serverName, "_", " ")
	// "notion-fetch" on the Notion server repeats the server's name.
	if len(actionWords) > 1 && strings.EqualFold(actionWords[0], strings.ReplaceAll(serverName, " ", "")) {
		actionWords = actionWords[1:]
	}
	detail := strings.TrimSpace(capitalized(serverName) + " " + strings.Join(actionWords, " "))
	return truncate(detail, mcpDetailRunes)
}

func capitalized(word string) string {
	first, size := utf8.DecodeRuneInString(word)
	if size == 0 {
		return word
	}
	return string(unicode.ToUpper(first)) + word[size:]
}

// approvalOf is a permission's tool and detail from the call that
// waits. The detail and the text it is read from both go through the
// credential check; when either may hold one, the row gets no detail.
func approvalOf(call toolCall) *waitingOn {
	w := &waitingOn{For: waitPermission, Tool: call.name, called: call.at}
	var source, detail string
	if rule, listed := callDetails[call.name]; listed {
		source = rule.source(call.input)
		detail = rule.shorten(source)
	} else {
		source = call.name
		detail = mcpDetail(call.name)
	}
	if detail == "" || mayHoldCredential(keptStart(source)) || mayHoldCredential(detail) {
		return w
	}
	w.Detail = detail
	return w
}

// claudeWaiting is what a waiting Claude Code session waits for: the
// kind from its live status, and for a permission the call at the end
// of its transcript that has no result yet. An unknown kind gives nil.
func claudeWaiting(waitingFor string, pending *toolCall) *waitingOn {
	kind, known := claudeWaitKinds[waitingFor]
	switch {
	case !known:
		return nil
	case kind == waitPermission && pending != nil:
		return approvalOf(*pending)
	default:
		return &waitingOn{For: kind}
	}
}
