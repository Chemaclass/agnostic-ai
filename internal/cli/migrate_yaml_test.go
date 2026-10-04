package cli

import (
	"strings"
	"testing"
)

func TestRewriteTopLevelYAMLKeys_KeepsEveryOtherByte(t *testing.T) {
	src := "# Keep this comment.\n" +
		"name: guard\n" +
		"\n" +
		"event:   \"PreToolUse\"   # before the tool runs\n" +
		"matcher: 'Bash'\n" +
		"command: |\n" +
		"  event: not a key\n" +
		"\n" +
		"timeout: 10 # seconds\n"
	got, err := rewriteTopLevelYAMLKeys(src, []yamlKeyRewrite{
		{Key: "event", NewKey: "on", Value: "before-tool"},
		{Key: "matcher", NewKey: "match", Value: "shell"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(strings.Replace(src,
		`event:   "PreToolUse"   # before`, `on:   "before-tool"   # before`, 1),
		"matcher: 'Bash'", "match: 'shell'", 1)
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRewriteTopLevelYAMLKeys_RefusesWhatItCannotEditInPlace(t *testing.T) {
	rw := []yamlKeyRewrite{{Key: "event", NewKey: "on", Value: "stop"}}
	for name, src := range map[string]string{
		"block scalar":    "event: |\n  Stop\n",
		"folded plain":    "event: Sto\n  p\n",
		"flow mapping":    "{event: Stop}\n",
		"missing key":     "name: x\n",
		"new key set":     "event: Stop\non: stop\n",
		"anchored value":  "event: &e Stop\n",
		"tagged value":    "event: !!str Stop\n",
		"quoted key":      "\"event\": Stop\n",
		"value next line": "event:\n  Stop\n",
	} {
		if got, err := rewriteTopLevelYAMLKeys(src, rw); err == nil {
			t.Errorf("%s: rewrote %q to %q, want an error", name, src, got)
		}
	}
}
