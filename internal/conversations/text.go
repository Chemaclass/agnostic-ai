package conversations

import (
	"context"
	"maps"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/chemaclass/agnostic-ai/internal/preview"
)

const (
	titleRunes   = 120
	openingRunes = 160
	openingLines = 3
	promptRunes  = 300
)

// injectedBlocks are wrappers a tool adds around or beside what the
// person typed: editor context, reminders, command output.
var injectedBlocks = regexp.MustCompile(`(?s)<(system-reminder|ide_opened_file|ide_selection|ide_diagnostics|environment_context|user_info|local-command-stdout|local-command-stderr|local-command-caveat|command-message|attached_files|git_status|open_and_recently_viewed_files|rules|user_instructions)\b[^>]*>.*?</(?:system-reminder|ide_opened_file|ide_selection|ide_diagnostics|environment_context|user_info|local-command-stdout|local-command-stderr|local-command-caveat|command-message|attached_files|git_status|open_and_recently_viewed_files|rules|user_instructions)>`)

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

// texts are the raw strings a reader found for one conversation.
type texts struct {
	title       string
	firstPrompt string
	lastPrompt  string
}

// shownTexts are what a row shows of its texts once any that may hold a
// credential are withheld.
type shownTexts struct {
	title    string
	opening  []string
	prompt   string
	withheld []string
}

// showTexts withholds any text that may hold a credential. A row with no
// title takes the first line of its first prompt.
func showTexts(t texts) shownTexts {
	var shown shownTexts
	first := typedText(t.firstPrompt)
	last := typedText(t.lastPrompt)
	title := oneLine(t.title)
	if title == "" && first != "" {
		title = strings.SplitN(first, "\n", 2)[0]
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
			shown.opening = openingOf(first)
		}
	}
	if last != "" {
		if mayHoldCredential(typedLayout(t.lastPrompt)) || mayHoldCredential(last) {
			shown.withheld = append(shown.withheld, FieldPrompt)
		} else {
			shown.prompt = truncate(oneLine(last), promptRunes)
		}
	}
	return shown
}

// shownCache keeps each row's shown texts from one scan to the next,
// since the credential check costs more than the rest of a warm scan.
// Texts no scan asked for since the previous one are dropped.
type shownCache struct {
	last, next map[texts]shownTexts
}

func (c *shownCache) start() { c.next = map[texts]shownTexts{} }

// finish keeps what this scan asked for. A canceled scan stopped before
// it reached every row, so it adds to what the cache held instead.
func (c *shownCache) finish(ctx context.Context) {
	if ctx.Err() == nil || c.last == nil {
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

// fill sets the row's title, opening and prompt from t.
func (s *Scanner) fill(c *Conversation, t texts) {
	shown := s.shown.get(t)
	c.Title, c.Opening, c.Prompt, c.Withheld = shown.title, shown.opening, shown.prompt, shown.withheld
}

// mayHoldCredential withholds a whole text on any sign of a credential,
// as the file content view does, read as prose rather than as a file.
func mayHoldCredential(text string) bool {
	return text != "" && preview.ProseSensitive(text)
}
