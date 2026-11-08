package conversations

import (
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

// fill sets the row's title, opening and prompt, withholding any of them
// that may hold a credential. A row with no title takes the first line
// of its first prompt.
func (c *Conversation) fill(t texts) {
	first := typedText(t.firstPrompt)
	last := typedText(t.lastPrompt)
	title := oneLine(t.title)
	if title == "" && first != "" {
		title = strings.SplitN(first, "\n", 2)[0]
	}

	if mayHoldCredential(title) {
		c.Withheld = append(c.Withheld, FieldTitle)
	} else {
		c.Title = truncate(title, titleRunes)
	}
	if first != "" {
		if mayHoldCredential(typedLayout(t.firstPrompt)) || mayHoldCredential(first) {
			c.Withheld = append(c.Withheld, FieldOpening)
		} else {
			c.Opening = openingOf(first)
		}
	}
	if last != "" {
		if mayHoldCredential(typedLayout(t.lastPrompt)) || mayHoldCredential(last) {
			c.Withheld = append(c.Withheld, FieldPrompt)
		} else {
			c.Prompt = truncate(oneLine(last), promptRunes)
		}
	}
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

// matching keeps the conversations whose shown text holds every word:
// title, folder, branch, opening lines, latest prompt, tool and app. A
// withheld field is not shown, so it does not match either.
func matching(list []Conversation, words []string) []Conversation {
	var kept []Conversation
	for _, c := range list {
		if holdsAll(strings.Join(append([]string{c.Title, c.Folder, c.Branch, c.Prompt, toolLabels[c.Tool], string(c.App)}, c.Opening...), "\n"), words) {
			kept = append(kept, c)
		}
	}
	return kept
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

// filled is what fill wrote for one set of raw texts.
type filled struct {
	texts    texts
	title    string
	opening  []string
	prompt   string
	withheld []string
}

// fill sets a row's shown text from its raw texts, reusing the last
// result while the texts are the same: the credential check is the
// costly part of a scan.
func (s *Scanner) fill(c *Conversation, t texts) {
	if s.filled == nil {
		s.filled = map[string]filled{}
	}
	key := string(c.Tool) + ":" + c.ID
	if hit, ok := s.filled[key]; ok && hit.texts == t {
		c.Title, c.Opening, c.Prompt, c.Withheld = hit.title, hit.opening, hit.prompt, hit.withheld
		c.pending = nil
		return
	}
	c.Title, c.Opening, c.Prompt, c.Withheld = "", nil, "", nil
	c.fill(t)
	c.pending = nil
	s.filled[key] = filled{texts: t, title: c.Title, opening: c.Opening, prompt: c.Prompt, withheld: c.Withheld}
}
