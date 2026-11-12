package conversations

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/chemaclass/agnostic-ai/internal/preview"
)

const (
	titleRunes   = 120
	openingRunes = 160
	openingLines = 3
	promptRunes  = 300
	replyRunes   = 200
	// replyCheckedBytes is what a reader keeps of an agent message for the
	// credential check, cut at a space so a token is checked whole or
	// never shown. Titles and prompts are checked whole; a reply is
	// capped because the check costs about 3 ms a kilobyte and a working
	// agent writes a new one every few seconds.
	replyCheckedBytes = 4096
)

// injectedBlocks are wrappers a tool adds around or beside what the
// person typed: editor context, reminders, command output.
const injectedNames = `system-reminder|ide_opened_file|ide_selection|ide_diagnostics|environment_context|user_info|local-command-stdout|local-command-stderr|local-command-caveat|command-message|attached_files|git_status|open_and_recently_viewed_files|rules|user_instructions`

var injectedBlocks = regexp.MustCompile(`(?s)<(` + injectedNames + `)\b[^>]*>.*?</(?:` + injectedNames + `)>`)

// injectedTag is an open or close tag of an injected block that removal
// left behind: a block nested in another ends the match early.
var injectedTag = regexp.MustCompile(`</?(` + injectedNames + `)\b[^>]*>`)

// typedWrappers hold what the person typed: keep their content.
var typedWrappers = regexp.MustCompile(`</?(user_query|command-name|command-args)\b[^>]*>`)

var spaceRun = regexp.MustCompile(`[ \t]+`)

// typedLayout is what the person typed, without the blocks tools
// inject, keeping its indentation for the credential check.
func typedLayout(text string) string {
	text = injectedBlocks.ReplaceAllString(text, "")
	return typedWrappers.ReplaceAllString(text, " ")
}

// typedText is what the person typed, without the blocks tools inject.
func typedText(text string) string {
	lines := strings.Split(typedLayout(text), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(spaceRun.ReplaceAllString(line, " "))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func truncate(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}

func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// openingOf is the first non-empty lines of a prompt.
func openingOf(prompt string) []string {
	var lines []string
	for _, line := range strings.Split(prompt, "\n") {
		if line == "" {
			continue
		}
		lines = append(lines, truncate(line, openingRunes))
		if len(lines) == openingLines {
			break
		}
	}
	return lines
}

// agentSaid cuts an agent message to what a row keeps of it: its start
// and, when it asks one, its question. start and ending are the
// message's first and last part, the whole message when the reader has
// it; asked is a question the store marks.
func agentSaid(start, ending, asked string) (reply, question string) {
	reply = keptStart(strings.TrimSpace(start))
	if asked = strings.TrimSpace(asked); asked != "" {
		return reply, keptStart(asked)
	}
	return reply, closingQuestion(ending)
}

// replyEnds is an agent message as a query reads it: at most the first
// and last replyCheckedBytes+1 characters, so a long reply costs a fixed
// amount, and its whole length.
type replyEnds struct {
	Start  string `json:"start"`
	Ending string `json:"ending"`
	Length int    `json:"length"`
}

// replyEndsColumns selects the replyEnds of the message text is.
func replyEndsColumns(text string) string {
	part := strconv.Itoa(replyCheckedBytes + 1)
	return `substr(` + text + `, 1, ` + part + `) AS start,
  substr(` + text + `, -` + part + `) AS ending,
  length(` + text + `) AS length`
}

// wholeLines is the ending without the line it may start inside: a
// slice cut from a longer message, whose first line is then partial.
func (ends replyEnds) wholeLines() string {
	if utf8.RuneCountInString(ends.Ending) >= ends.Length {
		return ends.Ending
	}
	_, rest, found := strings.Cut(ends.Ending, "\n")
	if !found || strings.TrimSpace(rest) == "" {
		return ""
	}
	return rest
}

// keptStart cuts text to replyCheckedBytes at the last space before the
// limit, or at a rune boundary when there is none.
func keptStart(text string) string {
	if len(text) <= replyCheckedBytes {
		return text
	}
	if cut := strings.LastIndexFunc(text[:replyCheckedBytes+1], unicode.IsSpace); cut > 0 {
		return strings.TrimSpace(text[:cut])
	}
	cut := replyCheckedBytes
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}

// closingQuestion is the message's last line when it asks something.
func closingQuestion(message string) string {
	lines := strings.Split(strings.TrimSpace(message), "\n")
	last := strings.Trim(lines[len(lines)-1], " \t*_`")
	if !strings.HasSuffix(last, "?") || len(last) > replyCheckedBytes {
		return ""
	}
	return last
}

// markupStart is a heading, quote or list marker at the start of a line.
var markupStart = regexp.MustCompile(`(?m)^[ \t]*[#>*-]+[ \t]+`)

// replyLine is an agent message as one line of plain text: it drops the
// Markdown marks that would show as noise, and renders nothing else.
func replyLine(text string) string {
	text = markupStart.ReplaceAllString(text, "")
	text = strings.NewReplacer("**", "", "`", "").Replace(text)
	return oneLine(text)
}

// replyMayHoldCredential checks a reply as stored and, when it holds the
// markup replyLine drops, as shown: dropping a code span's backticks or
// bold's asterisks can join a token's pieces. The rest of replyLine
// only moves spaces, which joins nothing.
func replyMayHoldCredential(text string) bool {
	if mayHoldCredential(text) {
		return true
	}
	return (strings.Contains(text, "`") || strings.Contains(text, "**")) && mayHoldCredential(replyLine(text))
}

// texts are the raw strings a reader found for one conversation.
type texts struct {
	title       string
	firstPrompt string
	lastPrompt  string
	reply       string
	question    string
}

// shownTexts are what a row shows of its texts once any that may hold a
// credential are withheld, and the conversation its link line names.
type shownTexts struct {
	title    string
	opening  []string
	prompt   string
	reply    string
	withheld []string
	parent   *Ref
}

// showTexts withholds any text that may hold a credential. A row with no
// title takes the first line of its first prompt. A link line at the
// start of the first prompt names the parent and is cut from the title,
// the opening and the latest prompt; the credential check still reads
// the whole prompt.
func showTexts(t texts) shownTexts { return showTextsOf(t, true) }

func showTextsOf(t texts, readLinks bool) shownTexts {
	var shown shownTexts
	first := typedText(t.firstPrompt)
	last := typedText(t.lastPrompt)
	title := oneLine(t.title)
	opening := first
	// An id that may be a credential is no parent: the API would serve
	// it while the texts that hold it are withheld.
	if ref, rest, ok := linkOf(t.firstPrompt); ok && readLinks && !mayHoldCredential(linkLine(ref)) {
		shown.parent = &ref
		opening = typedText(rest)
		title = cutLink(title, ref)
		if again, rest, ok := linkOf(t.lastPrompt); ok && again == ref {
			last = typedText(rest)
		}
	}
	if title == "" && opening != "" {
		title = strings.SplitN(opening, "\n", 2)[0]
	}

	if mayHoldCredential(title) {
		shown.withheld = append(shown.withheld, FieldTitle)
	} else {
		shown.title = truncate(title, titleRunes)
	}
	if first != "" {
		if mayHoldCredential(typedLayout(t.firstPrompt)) || mayHoldCredential(first) {
			shown.withheld = append(shown.withheld, FieldOpening)
		} else {
			shown.opening = openingOf(opening)
		}
	}
	if last != "" {
		if mayHoldCredential(typedLayout(t.lastPrompt)) || mayHoldCredential(last) {
			shown.withheld = append(shown.withheld, FieldPrompt)
		} else {
			shown.prompt = truncate(oneLine(last), promptRunes)
		}
	}
	if t.reply != "" || t.question != "" {
		if replyMayHoldCredential(t.reply) || replyMayHoldCredential(t.question) {
			shown.withheld = append(shown.withheld, FieldReply)
		} else {
			shown.reply = truncate(replyLine(firstNonEmpty(t.question, t.reply)), replyRunes)
		}
	}
	return shown
}

// shownCache keeps each row's shown texts from one scan to the next,
// since the credential check costs more than the rest of a warm scan.
// A list asks for the rows of one week and a search for every row that
// may match, so what one scan did not ask for is kept for the next: a
// search between two lists would otherwise pay for every row again.
type shownCache struct {
	last, next map[texts]shownTexts
	// lastAsked is how many texts the latest scan asked for, the size of
	// the next scan's map.
	lastAsked int
}

// maxShownKept bounds the cache. Past it, only what the latest scan
// asked for is kept; texts of chats that changed or went away would
// otherwise pile up for as long as the server runs.
const maxShownKept = 20000

func (c *shownCache) start() { c.next = make(map[texts]shownTexts, c.lastAsked) }

// finish adds what this scan asked for to what the cache held.
func (c *shownCache) finish() {
	c.lastAsked = len(c.next)
	if c.last == nil || len(c.last)+len(c.next) > maxShownKept {
		c.last, c.next = c.next, nil
		return
	}
	maps.Copy(c.last, c.next)
	c.next = nil
}

func (c *shownCache) get(t texts) shownTexts {
	if shown, ok := c.next[t]; ok {
		return shown
	}
	shown, ok := c.last[t]
	if !ok {
		shown = showTexts(t)
	}
	if c.next != nil {
		c.next[t] = shown
	}
	return shown
}

// fill sets the row's title, opening, prompt, reply and parent from t. The
// reply is looked up on its own: a working agent writes a new one every
// few seconds, which must not check the other texts again.
func (s *Scanner) fill(c *Conversation, t texts) {
	prompts := texts{title: t.title, firstPrompt: t.firstPrompt, lastPrompt: t.lastPrompt}
	shown := s.shown.get(prompts)
	if shown.parent != nil && *shown.parent == (Ref{Tool: c.Tool, ID: c.ID}) {
		// A conversation cannot continue itself: its line stays text.
		shown = showTextsOf(prompts, false)
	}
	c.Title, c.Opening, c.Prompt, c.Withheld, c.Parent = shown.title, shown.opening, shown.prompt, shown.withheld, shown.parent
	if t.reply != "" || t.question != "" {
		said := s.shown.get(texts{reply: t.reply, question: t.question})
		c.Reply = said.reply
		if len(said.withheld) > 0 {
			c.Withheld = append(slices.Clone(c.Withheld), said.withheld...)
		}
	}
	c.showChosenName()
}

// showChosenName puts the name the person chose over the tool's title.
// Withheld is shared with cached rows, so it is replaced, never changed.
func (c *Conversation) showChosenName() {
	if c.renamed == "" {
		return
	}
	c.Title = c.renamed
	if slices.Contains(c.Withheld, FieldTitle) {
		c.Withheld = slices.DeleteFunc(slices.Clone(c.Withheld), func(field string) bool { return field == FieldTitle })
	}
}

// CleanTitle is a name the person typed, as one line, or why it cannot
// be kept: empty, longer than a row's title, or a possible credential.
func CleanTitle(typed string) (string, error) {
	title := oneLine(typed)
	switch {
	case title == "":
		return "", errors.New("the name is empty")
	case utf8.RuneCountInString(title) > titleRunes:
		return "", fmt.Errorf("the name is longer than %d characters", titleRunes)
	case mayHoldCredential(title):
		return "", errors.New("the name may hold a password or key, so it is not saved")
	}
	return title, nil
}

// mayHoldCredential withholds a whole text on any sign of a credential,
// as the file content view does, read as prose rather than as a file.
func mayHoldCredential(text string) bool {
	return text != "" && preview.ProseSensitive(text)
}

// queryWords splits a search into lowercase words.
func queryWords(query string) []string {
	return strings.Fields(strings.ToLower(query))
}

// matching keeps the conversations whose row text holds every word:
// title, folder, branch, latest prompt, latest reply, tool and app, the
// fields a row shows. A withheld field is not shown, so it does not match either.
func matching(list []Conversation, words []string) []Conversation {
	var kept []Conversation
	for _, c := range list {
		if holdsAll(strings.Join([]string{asShown(c.Title), c.Folder, c.Branch, asShown(c.Prompt), c.Reply, toolLabels[c.Tool], string(c.App)}, "\n"), words) {
			kept = append(kept, c)
		}
	}
	return kept
}

// typedShell is a command typed with Claude Code's ! prefix, which the
// page shows as "! command"; the tool may cut the closing tag.
var typedShell = regexp.MustCompile(`^\s*<bash-input>((?s).*?)(?:</bash-input>\s*)?$`)

// asShown reads a title or prompt the way the page shows it, so a search
// matches the text a person sees and never the markup around it.
func asShown(text string) string {
	if match := typedShell.FindStringSubmatch(text); match != nil {
		return "! " + match[1]
	}
	return text
}

// holdsAll reports whether text holds each lowercase word, ignoring case.
func holdsAll(text string, words []string) bool {
	text = strings.ToLower(text)
	for _, word := range words {
		if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}
