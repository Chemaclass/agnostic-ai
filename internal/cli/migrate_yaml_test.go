package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
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

func TestRewriteFrontmatterKeys_KeepsCommentsKeyOrderAndTheBody(t *testing.T) {
	src := "---\n" +
		"# Keep this comment.\n" +
		"name: reviewer\n" +
		"model: \"fast\"   # the tier\n" +
		"tools: [Read, Grep]\n" +
		"---\n" +
		"\n" +
		"model: a body line, not a key\n" +
		"---\n" +
		"More body.\n"
	got, err := rewriteFrontmatterKeys(src, []yamlKeyRewrite{{Key: "model", NewKey: "tier", Value: "small"}})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(src, `model: "fast"   # the tier`, `tier: "small"   # the tier`, 1)
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	entry, err := spec.ParseMarkdownBytes(spec.KindAgent, []byte(got))
	if err != nil || entry.Meta["tier"] != "small" || entry.Meta["model"] != nil || !strings.Contains(entry.Body, "model: a body line") {
		t.Errorf("the loader must read the rewrite: %v %v\n%s", err, entry.Meta, entry.Body)
	}
}

func TestRewriteFrontmatterKeys_RefusesAFileWithoutFrontmatter(t *testing.T) {
	rw := []yamlKeyRewrite{{Key: "model", NewKey: "tier", Value: "small"}}
	for name, src := range map[string]string{
		"no frontmatter": "model: fast\n",
		"never closed":   "---\nmodel: fast\n",
		"key in body":    "---\nname: x\n---\nmodel: fast\n",
	} {
		if got, err := rewriteFrontmatterKeys(src, rw); err == nil {
			t.Errorf("%s: rewrote %q to %q, want an error", name, src, got)
		}
	}
}
