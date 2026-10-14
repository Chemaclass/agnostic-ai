package antigravity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_MemoryIncludeCountsIndexBytes(t *testing.T) {
	for _, tc := range []struct {
		name         string
		project      string
		personal     string
		rulesDir     string
		scope        string
		layer        string
		repoPersonal bool
		wantNote     bool
	}{
		{name: "oversized project", project: strings.Repeat("p", ruleByteLimit), layer: "builtin", wantNote: true},
		{name: "oversized personal", personal: strings.Repeat("p", ruleByteLimit), layer: "builtin", wantNote: true},
		{name: "combined indexes", project: strings.Repeat("p", 12000), personal: strings.Repeat("q", 12000), layer: "builtin", wantNote: true},
		{name: "missing indexes", layer: "builtin"},
		{name: "repo personal index excluded", personal: strings.Repeat("p", ruleByteLimit), layer: "builtin", repoPersonal: true},
		{name: "small indexes", project: "project fact\n", personal: "personal fact\n", layer: "builtin"},
		{name: "custom scoped output", project: strings.Repeat("p", ruleByteLimit), rulesDir: "custom/rules", scope: "nested", layer: "builtin", wantNote: true},
		{name: "user rule", project: strings.Repeat("p", ruleByteLimit), layer: "project"},
		{name: "included frontmatter stripped", project: "---\nname: " + strings.Repeat("p", ruleByteLimit) + "\n---\nsmall body\n", layer: "builtin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			buf := swapNoteWarner(t)
			for path, body := range map[string]string{emit.ProjectMemoryIndexPath: tc.project, emit.PersonalMemoryIndexPath: tc.personal} {
				if body == "" {
					continue
				}
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			entry := spec.Entry{Kind: spec.KindRule, Name: "antigravity-memory-index", Layer: tc.layer, Scope: tc.scope,
				Body: "@[Project memory](../../.agnostic-ai/memory/MEMORY.md)\n\nPersonal memory, `.agnostic-ai/local/memory/MEMORY.md`:\n\n@[Personal memory](../../.agnostic-ai/local/memory/MEMORY.md)\n"}
			cfg := &config.Config{Outputs: map[string]config.Output{target: {RulesDir: tc.rulesDir}}}
			if tc.repoPersonal {
				cfg.Memory.Personal = config.PersonalMemoryRepo
			}
			if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), cfg, false); err != nil {
				t.Fatal(err)
			}
			emit.FlushCoverageNotes()
			if got := strings.Contains(buf.String(), ruleTooLongReason); got != tc.wantNote {
				t.Errorf("oversized memory note = %v, want %v: %s", got, tc.wantNote, buf.String())
			}
			rulesDir := tc.rulesDir
			if rulesDir == "" {
				rulesDir = defaultRulesDir
			}
			raw, err := os.ReadFile(filepath.Join(dir, tc.scope, rulesDir, entry.Name+".md"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), "@[Project memory](") || strings.Contains(string(raw), strings.Repeat("p", 1000)) {
				t.Errorf("memory rule must retain its include without copying index bytes: %s", raw)
			}
		})
	}
}

func TestNativeMemoryRuleBytes_ReplacesIncludeAtByteLimit(t *testing.T) {
	testutil.TempCwd(t)
	entry := spec.Entry{Kind: spec.KindRule, Name: "antigravity-memory-index", Layer: "builtin", Body: "@[Project memory](../../.agnostic-ai/memory/MEMORY.md)\n"}
	include := "@[Project memory](../../.agnostic-ai/memory/MEMORY.md)"
	index := filepath.FromSlash(emit.ProjectMemoryIndexPath)
	if err := os.MkdirAll(filepath.Dir(index), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []int{0, 1} {
		body := strings.Repeat("p", ruleByteLimit-len(rule(entry))+len(include)+extra)
		if err := os.WriteFile(index, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		size, err := nativeMemoryRuleBytes(entry, defaultRulesDir)
		if err != nil {
			t.Fatal(err)
		}
		if size != ruleByteLimit+extra {
			t.Errorf("expanded size = %d, want %d", size, ruleByteLimit+extra)
		}
	}
}

func TestNativeMemoryIndexBody_PreservesBodyBytes(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"plain\r\nbody\r\n", "plain\r\nbody\r\n"},
		{"---\nname: example\n---\nbody\n", "body\n"},
		{"---\r\nname: example\r\n---\r\nbody\r\n", "body\r\n"},
		{"---\nunfinished\n", "---\nunfinished\n"},
	} {
		if got := string(nativeMemoryIndexBody([]byte(tc.input))); got != tc.want {
			t.Errorf("index body = %q, want %q", got, tc.want)
		}
	}
}

func TestNativeMemoryRuleBytes_ReadErrorNamesIndex(t *testing.T) {
	testutil.TempCwd(t)
	index := filepath.FromSlash(emit.ProjectMemoryIndexPath)
	if err := os.MkdirAll(index, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := spec.Entry{Kind: spec.KindRule, Name: "antigravity-memory-index", Layer: "builtin", Body: "@[Project memory](../../.agnostic-ai/memory/MEMORY.md)\n"}
	_, err := nativeMemoryRuleBytes(entry, defaultRulesDir)
	if err == nil || !strings.Contains(err.Error(), index) {
		t.Errorf("read error must name %s: %v", index, err)
	}
}
