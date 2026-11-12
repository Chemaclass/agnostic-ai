package conversations

import (
	"context"
	"math"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

const nestedRepro = "<system-reminder>Open file: <ide_opened_file>main.go</ide_opened_file>\nProject notes from the tool\n</system-reminder>\nFix the login bug"

func TestTypedText_DropsNestedInjectedBlocksWhole(t *testing.T) {
	for name, in := range map[string]string{
		"different names": nestedRepro,
		"same name":       "<system-reminder>a <system-reminder>b</system-reminder> notes</system-reminder>\nFix the login bug",
		"three levels":    "<rules>a <system-reminder>b <ide_selection>c</ide_selection> d</system-reminder> notes</rules>\nFix the login bug",
		"inner unclosed":  "<system-reminder>a <ide_selection>b notes</system-reminder>\nFix the login bug",
		"two in a row":    "<rules>a <rules>b</rules></rules><user_info>c <git_status>d</git_status></user_info>Fix the login bug",
	} {
		if actual := typedText(in); actual != "Fix the login bug" {
			t.Errorf("%s: typedText = %q", name, actual)
		}
	}
}

func TestTypedText_KeepsTextAnUnmatchedTagCannotEnclose(t *testing.T) {
	for in, want := range map[string]string{
		"<system-reminder>Fix the login bug":                                              "<system-reminder>Fix the login bug",
		"Fix the login bug</system-reminder> now":                                         "Fix the login bug</system-reminder> now",
		"<system-reminder> Fix the login bug <rules>x</rules>":                            "<system-reminder> Fix the login bug",
		"<rulesets>Fix the login bug</rulesets>":                                          "<rulesets>Fix the login bug</rulesets>",
		"Why does ```<system-reminder>``` break?\n<system-reminder>ctx</system-reminder>": "Why does ```<system-reminder>``` break?",
		"<rules>a</system-reminder>Fix the login bug":                                     "Fix the login bug",
	} {
		if actual := typedText(in); actual != want {
			t.Errorf("typedText(%q) = %q, want %q", in, actual, want)
		}
	}
}

// formerInjectedBlocks is the single expression removal used before it
// handled nesting, kept to prove input without nesting reads the same.
var formerInjectedBlocks = regexp.MustCompile(`(?s)<(` + injectedNames + `)\b[^>]*>.*?</(?:` + injectedNames + `)>`)

func TestWithoutInjected_ReadsInputWithoutNestingAsBefore(t *testing.T) {
	for _, in := range []string{
		"Fix the login bug",
		"<system-reminder>be brief</system-reminder>\nFix the login bug",
		"<ide_selection lines=\"1-3\">code</ide_selection>Why? <git_status>clean</git_status> now",
		"<ide_selection path=\"a>b\">code</ide_selection>Fix the login bug",
		"<SYSTEM-REMINDER>shouted</SYSTEM-REMINDER> Fix the login bug",
		"```\n<rules>typed in a fence</rules>\n```\nWhy is this removed?",
		"<rules>a</system-reminder>Fix the login bug",
		"Fix the login bug</system-reminder> now",
		"<system-reminder>Fix the login bug",
		"<rulesets>Fix the login bug</rulesets>",
		"<user_query>Fix</user_query><rules >x</rules>",
	} {
		if actual, former := withoutInjected(in), formerInjectedBlocks.ReplaceAllString(in, ""); actual != former {
			t.Errorf("withoutInjected(%q) = %q, before %q", in, actual, former)
		}
	}
}

func TestTypedText_DropsASelfClosingInjectedTag(t *testing.T) {
	for _, in := range []string{
		"<ide_selection/>\n<system-reminder>ctx</system-reminder>\nFix the login bug",
		"<ide_selection />Fix the login bug",
		"<system-reminder>a <rules/> b</system-reminder>Fix the login bug",
	} {
		if actual := typedText(in); actual != "Fix the login bug" {
			t.Errorf("typedText(%q) = %q", in, actual)
		}
	}
	prompt := "<ide_selection/>\nContinues claude:" + idParent + "\nWrite the notes"
	if shown := showTexts(texts{firstPrompt: prompt}); shown.parent == nil {
		t.Errorf("a link after a self-closing tag must link: title %q", shown.title)
	}
}

func TestTypedText_KeepsATagThatOnlyStartsWithAnInjectedName(t *testing.T) {
	for in, want := range map[string]string{
		"Style the <rules-panel>x</rules-panel> widget": "Style the <rules-panel>x</rules-panel> widget",
		"<rules>a</rules-file>Fix the login bug":        "<rules>a</rules-file>Fix the login bug",
		"<system-reminder-x>Fix the login bug":          "<system-reminder-x>Fix the login bug",
		"<rules.md>Fix the login bug</rules.md>":        "<rules.md>Fix the login bug</rules.md>",
	} {
		if actual := typedText(in); actual != want {
			t.Errorf("typedText(%q) = %q, want %q", in, actual, want)
		}
	}
}

func TestTypedText_StaysLinearOnUnmatchedTags(t *testing.T) {
	for name, prompt := range map[string]func(tags int) string{
		"unclosed": func(tags int) string { return strings.Repeat("<system-reminder>", tags) },
		"stray":    func(tags int) string { return strings.Repeat("</rules>", tags) },
		"nested":   func(tags int) string { return strings.Repeat("<rules>", tags/2) + strings.Repeat("</rules>", tags/2-1) },
		"mixed": func(tags int) string {
			return strings.Repeat("<system-reminder>", tags/2) + strings.Repeat("</ide_selection>", tags/2)
		},
	} {
		small, large := prompt(2500)+"Fix the login bug", prompt(40000)+"Fix the login bug"
		if typed := typedText(large); !strings.HasSuffix(typed, "Fix the login bug") {
			t.Errorf("%s: typed text lost its end: %q", name, typed[max(0, len(typed)-40):])
		}
		// Sixteen times the tags may take about sixteen times as long;
		// quadratic work takes 256. The wide gap keeps a slow, instrumented
		// or loaded runner on the right side of the limit.
		if ratio := fastestRemoval(large).Seconds() / fastestRemoval(small).Seconds(); ratio > 100 {
			t.Errorf("%s: sixteen times the tags took %.1f times as long", name, ratio)
		}
	}
}

func fastestRemoval(prompt string) time.Duration {
	fastest := time.Duration(math.MaxInt64)
	for range 3 {
		start := time.Now()
		withoutInjected(prompt)
		fastest = min(fastest, time.Since(start))
	}
	return fastest
}

func BenchmarkTypedText_UnmatchedTags(b *testing.B) {
	prompt := strings.Repeat("<system-reminder>", 50000) + strings.Repeat("</ide_selection>", 50000) + "Fix the login bug"
	for b.Loop() {
		typedText(prompt)
	}
}

func TestScan_TitlesARowByWhatWasTypedAfterANestedInjectedBlock(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	latest := "<system-reminder>a <system-reminder>b</system-reminder>\nMore notes\n</system-reminder>\nNow add a test"
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idCache+".jsonl"), lines(
		obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": "2026-11-07T08:00:00.000Z",
			"message": obj{"role": "user", "content": nestedRepro}},
		obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": "2026-11-07T08:01:00.000Z",
			"message": obj{"role": "user", "content": latest}},
		obj{"type": "last-prompt", "lastPrompt": latest},
	))

	row := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idCache)

	if row.Title != "Fix the login bug" {
		t.Errorf("title = %q", row.Title)
	}
	if !slices.Equal(row.Opening, []string{"Fix the login bug"}) {
		t.Errorf("opening = %q", row.Opening)
	}
	if row.Prompt != "Now add a test" {
		t.Errorf("prompt = %q", row.Prompt)
	}
}

func TestShowTexts_ChecksTheTextANestedBlockNoLongerHides(t *testing.T) {
	token := "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	typedToken := nestedRepro + "\nuse " + token
	shown := showTexts(texts{firstPrompt: typedToken, lastPrompt: typedToken})
	if len(shown.opening) != 0 || shown.prompt != "" || !slices.Equal(shown.withheld, []string{FieldOpening, FieldPrompt}) {
		t.Errorf("a credential after a nested block: title %q opening %q prompt %q withheld %q", shown.title, shown.opening, shown.prompt, shown.withheld)
	}

	toolToken := "<system-reminder>a <rules>b</rules>\n" + token + "\n</system-reminder>\nFix the login bug"
	shown = showTexts(texts{firstPrompt: toolToken, lastPrompt: toolToken})
	for _, text := range append([]string{shown.title, shown.prompt}, shown.opening...) {
		if strings.Contains(text, token) || strings.Contains(text, "system-reminder") {
			t.Errorf("text from inside a nested block is shown: %q", text)
		}
	}
	if shown.title != "Fix the login bug" {
		t.Errorf("title = %q", shown.title)
	}
}

func TestShowTexts_LinksALineAfterANestedInjectedBlock(t *testing.T) {
	prompt := "<system-reminder>Context <ide_selection>x</ide_selection>\nnotes\n</system-reminder>\nContinues claude:" + idParent + "\nWrite the notes"
	shown := showTexts(texts{firstPrompt: prompt, lastPrompt: prompt})
	if shown.parent == nil || shown.parent.ID != idParent || shown.title != "Write the notes" {
		t.Errorf("parent %+v title %q", shown.parent, shown.title)
	}

	unclosed := "<system-reminder>Context\nContinues claude:" + idParent + "\nWrite the notes"
	if shown := showTexts(texts{firstPrompt: unclosed}); shown.parent != nil {
		t.Errorf("a link after an unclosed block must stay text: parent %+v", shown.parent)
	}
}

func TestCursorTyped_ReadsTheQueryAfterAnInjectedBlockThatQuotesIt(t *testing.T) {
	raw := "<rules>Wrap what the person typed in <user_query> tags.</rules>\n<user_query>Fix the CI job</user_query>"
	if typed := typedText(cursorTyped(raw)); typed != "Fix the CI job" {
		t.Errorf("typed = %q", typed)
	}
}

func TestScan_DoesNotTakeACursorAgentLineThatOnlyQuotesTheQueryTagForAPrompt(t *testing.T) {
	agent := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	scanner, _ := cursorAgentScanner(t, fakeProcesses{}, agent)
	write(t, filepath.Join(scanner.Stores.CursorDir, "projects", "p1", "agent-transcripts", agent, agent+".jsonl"), lines(
		obj{"role": "user", "message": obj{"content": []obj{textPart("<timestamp>Friday</timestamp>\n<user_query>Fix the CI job</user_query>")}}},
		obj{"role": "assistant", "message": obj{"content": []obj{textPart("The CI job passes now.")}}},
		obj{"role": "user", "message": obj{"content": []obj{textPart("<rules>Wrap what the person typed in <user_query> tags.</rules>")}}},
	))

	c := byID(t, scanner.Scan(context.Background(), ScanOptions{Within: []string{"/tmp/app"}}), agent)

	if c.Prompt != "Fix the CI job" || c.Reply != "The CI job passes now." {
		t.Errorf("prompt %q reply %q", c.Prompt, c.Reply)
	}
}

func TestTypedText_DropsACursorNotification(t *testing.T) {
	if typed := typedText(cursorTyped("<system_notification><task>build done</task></system_notification>")); typed != "" {
		t.Errorf("typed = %q", typed)
	}
}

// cursorTypedBeforeInjected is cursorTyped before it dropped injected
// blocks first: the status quo.
func cursorTypedBeforeInjected(text string) string {
	found := cursorQuery.FindAllStringSubmatch(text, -1)
	if len(found) == 0 {
		return text
	}
	queries := make([]string, len(found))
	for i, match := range found {
		queries[i] = match[1]
	}
	return strings.Join(queries, "\n")
}

// BenchmarkCursorTyped times the prompt a search reads for each pending
// Cursor row, on a short prompt and on one with a large attachment:
// the unwrap before it dropped injected blocks (status quo), the one in
// use, and typedText alone, which every shown prompt pays after either.
func BenchmarkCursorTyped(b *testing.B) {
	short := "<timestamp>Friday, Nov 7, 2025, 8:00 AM</timestamp>\n<system_notification><task>build done</task></system_notification>\n<user_query>Fix the CI job and add a test for the cookie redirect</user_query>"
	attached := "<attached_files>\n" + strings.Repeat("func handler(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }\n", 100) + "</attached_files>\n" + short
	for _, prompt := range []struct{ name, text string }{{"short", short}, {"attached", attached}} {
		for _, subject := range []struct {
			name   string
			unwrap func(string) string
		}{
			{"status_quo", cursorTypedBeforeInjected},
			{"in_use", cursorTyped},
			{"typed_text", typedText},
		} {
			b.Run(subject.name+"/"+prompt.name, func(b *testing.B) {
				for b.Loop() {
					subject.unwrap(prompt.text)
				}
			})
		}
	}
}

func TestTypedText_KeepsATagThatOnlyStartsWithAWrapperName(t *testing.T) {
	for in, want := range map[string]string{
		"<command-name-x>/review</command-name-x>":            "<command-name-x>/review</command-name-x>",
		"<user_query-notes>Fix the login bug":                 "<user_query-notes>Fix the login bug",
		"<user_query.md>Fix the login bug":                    "<user_query.md>Fix the login bug",
		"<command-name>/review</command-name>":                "/review",
		"<user_query id=\"1\">Fix the login bug</user_query>": "Fix the login bug",
	} {
		if actual := typedText(in); actual != want {
			t.Errorf("typedText(%q) = %q, want %q", in, actual, want)
		}
	}
}

func TestLinkOf_ReadsALinkLineOnlyInsideAWrapper(t *testing.T) {
	if _, _, ok := linkOf("<user_query-x>Continues claude:" + idSafari + "\nGo on</user_query-x>"); ok {
		t.Error("a tag that only starts with a wrapper name is text, so the line is not first")
	}
	if ref, _, ok := linkOf("<user_query>Continues claude:" + idSafari + "\nGo on</user_query>"); !ok || ref.ID != idSafari {
		t.Errorf("ref %+v ok %v", ref, ok)
	}
}
