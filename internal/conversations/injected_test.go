package conversations

import (
	"context"
	"math"
	"path/filepath"
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

func TestTypedText_StaysLinearOnUnmatchedTags(t *testing.T) {
	for name, prompt := range map[string]func(tags int) string{
		"unclosed": func(tags int) string { return strings.Repeat("<system-reminder>", tags) },
		"stray":    func(tags int) string { return strings.Repeat("</rules>", tags) },
		"nested":   func(tags int) string { return strings.Repeat("<rules>", tags/2) + strings.Repeat("</rules>", tags/2-1) },
		"mixed": func(tags int) string {
			return strings.Repeat("<system-reminder>", tags/2) + strings.Repeat("</ide_selection>", tags/2)
		},
	} {
		small, large := prompt(20000)+"Fix the login bug", prompt(80000)+"Fix the login bug"
		if typed := typedText(large); !strings.HasSuffix(typed, "Fix the login bug") {
			t.Errorf("%s: typed text lost its end: %q", name, typed[max(0, len(typed)-40):])
		}
		// Four times the tags may take about four times as long; quadratic
		// work takes sixteen. A ratio holds on a slow or instrumented runner
		// where a time limit does not.
		if ratio := fastestRemoval(large).Seconds() / fastestRemoval(small).Seconds(); ratio > 10 {
			t.Errorf("%s: four times the tags took %.1f times as long", name, ratio)
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
