package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// scopedKindTargets are the importers checked for an x-<target> key on a
// skill, rule, or command.
var scopedKindTargets = []string{
	"claude", "cursor", "gemini", "qoder", "junie", "kilo", "opencode", "kiro", "copilot",
	"windsurf", "factory", "codex", "cline", "trae", "augment", "antigravity", "amp", "crush",
}

// A key the spec sets only under x-<target> reaches that tool's file.
// Importing the file back must return it under x-<target>, or every
// other tool gets it on the next sync.
func TestImport_ScopedKeysStayScopedForEveryKind(t *testing.T) {
	kinds := []struct{ kind, path, extra string }{
		{"skill", "skills/rev/SKILL.md", ""},
		{"rule", "rules/rev.md", "globs: \"*.go\"\n"},
		{"command", "commands/rev.md", ""},
	}
	for _, target := range scopedKindTargets {
		for _, k := range kinds {
			t.Run(target+"/"+k.kind, func(t *testing.T) {
				root := testutil.TempCwd(t)
				silence(t)
				others := "cursor"
				if target == "cursor" {
					others = "claude"
				}
				writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: ["+target+", "+others+"]\n")
				specFile := filepath.Join(root, ".agnostic-ai", filepath.FromSlash(k.path))
				spec := "---\nname: rev\ndescription: Reviews\n" + k.extra + "x-" + target + ":\n  foo: bar # scoped\n---\n\nReview the diff.\n"
				writeFile(t, specFile, spec)
				mustSync(t)
				var natives []string
				_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
					if err != nil || d.IsDir() || strings.Contains(path, ".agnostic-ai") || strings.Contains(path, "/.git/") || strings.Contains(path, "/."+others+"/") {
						return nil
					}
					if data, _ := os.ReadFile(path); strings.Contains(string(data), "Review the diff.") {
						natives = append(natives, path)
					}
					return nil
				})
				if len(natives) == 0 {
					t.Skipf("%s writes no file for a %s", target, k.kind)
				}
				for _, n := range natives {
					data, _ := os.ReadFile(n)
					writeFile(t, n, strings.Replace(string(data), "Review the diff.", "Review the whole diff.", 1))
				}
				if out, err := runRoot(t, "import", target, "--overwrite"); err != nil {
					t.Fatalf("import: %v\n%s", err, out)
				}
				got, err := os.ReadFile(specFile)
				if err != nil {
					t.Skipf("import wrote no %s at %s", k.kind, k.path)
				}
				meta, _ := splitMdcFrontmatter(got)
				if _, leaked := meta["foo"]; leaked {
					t.Errorf("foo moved to the shared spec (%v):\n%s", natives, got)
				}
				custom, _ := meta["x-"+target].(map[string]any)
				if custom["foo"] != "bar" {
					t.Errorf("x-%s.foo lost (%v):\n%s", target, natives, got)
				}
				mustSync(t)
				if out, err := runRoot(t, "sync", "--check"); err != nil {
					t.Errorf("sync --check after import: %v\n%s", err, out)
				}
			})
		}
	}
}

// importRoundTrip syncs spec for targets, applies edit to target's file
// at native, imports it back, and returns the spec and the files sync
// then writes.
func importRoundTrip(t *testing.T, targets, target, specPath, spec, native string, edit func(string) string) string {
	t.Helper()
	root := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: ["+targets+"]\n")
	specFile := filepath.Join(root, ".agnostic-ai", filepath.FromSlash(specPath))
	writeFile(t, specFile, spec)
	mustSync(t)
	nativeFile := filepath.Join(root, filepath.FromSlash(native))
	data, err := os.ReadFile(nativeFile)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, nativeFile, edit(string(data)))
	if out, err := runRoot(t, "import", target, "--overwrite"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	got, err := os.ReadFile(specFile)
	if err != nil {
		t.Fatal(err)
	}
	mustSync(t)
	if out, err := runRoot(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after import: %v\n%s", err, out)
	}
	return string(got)
}

func editBody(s string) string {
	return strings.Replace(s, "Review the diff.", "Review the whole diff.", 1)
}

func TestImport_GeminiScopedMCPServersStayScoped(t *testing.T) {
	spec := "---\nname: rev\ndescription: Reviews\nx-gemini:\n  mcpServers:\n    gh:\n      command: gh-mcp\n---\n\nReview the diff.\n"
	got := importRoundTrip(t, "gemini, claude", "gemini", "agents/rev.md", spec, ".gemini/agents/rev.md", editBody)
	if want := editBody(spec); got != want {
		t.Errorf("spec after import:\n%s\nwant:\n%s", got, want)
	}
	claude, _ := os.ReadFile(filepath.Join(".claude", "agents", "rev.md"))
	if strings.Contains(string(claude), "mcp_servers") || strings.Contains(string(claude), "gh-mcp") {
		t.Errorf("Claude Code got Gemini's servers:\n%s", claude)
	}
}

// Deleting the only scoped key natively drops the block, so neither the
// spec nor the next sync keeps it.
func TestImport_EmptiedScopedBlockIsDropped(t *testing.T) {
	for _, tc := range []struct{ target, native string }{
		{"cursor", ".cursor/agents/rev.md"},
		{"qoder", ".qoder/agents/rev.md"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			spec := "---\nname: rev\ndescription: Reviews\nx-" + tc.target + ":\n  foo: bar\n---\n\nReview the diff.\n"
			got := importRoundTrip(t, tc.target+", claude", tc.target, "agents/rev.md", spec, tc.native, func(s string) string {
				return editBody(strings.Replace(s, "foo: bar\n", "", 1))
			})
			if want := "---\nname: rev\ndescription: Reviews\n---\n\nReview the whole diff.\n"; got != want {
				t.Errorf("spec after import:\n%s\nwant:\n%s", got, want)
			}
			native, _ := os.ReadFile(filepath.FromSlash(tc.native))
			if strings.Contains(string(native), "foo:") {
				t.Errorf("sync wrote the deleted key back:\n%s", native)
			}
		})
	}
}

// With readonly, a scoped disallowedTools goes back under x-claude
// whether or not another x-claude key is there.
func TestImport_ClaudeReadonlyScopedDisallowedToolsStaysScoped(t *testing.T) {
	for name, extra := range map[string]string{"alone": "", "with another key": "  color: red\n"} {
		t.Run(name, func(t *testing.T) {
			spec := "---\nname: rev\ndescription: Reviews\nreadonly: true\nx-claude:\n  disallowedTools: Bash\n" + extra + "---\n\nReview the diff.\n"
			got := importRoundTrip(t, "claude, cursor", "claude", "agents/rev.md", spec, ".claude/agents/rev.md", editBody)
			meta, _ := splitMdcFrontmatter([]byte(got))
			custom, _ := meta["x-claude"].(map[string]any)
			if _, top := meta["disallowedTools"]; top || custom["disallowedTools"] != "Bash" || meta["readonly"] != true {
				t.Errorf("spec after import:\n%s", got)
			}
		})
	}
}

func TestImportKiroAgentDocument_MovesNameInPlace(t *testing.T) {
	in := "---\ndescription: Reviews # why\nname: Rev Bot\nmodel: auto\n---\n\nBody.\n"
	got, err := importKiroAgentDocument([]byte(in), "rev")
	if err != nil {
		t.Fatal(err)
	}
	if want := "---\ndescription: Reviews # why\nmodel: auto\nx-kiro:\n  name: Rev Bot\n---\n\nBody.\n"; string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// A rule format holds only rule fields, so a scoped key it cannot carry
// stays in the spec next to the ones it does.
func TestImport_RuleScopedKeysTheFormatCannotHoldStay(t *testing.T) {
	for _, tc := range []struct{ target, native string }{
		{"cursor", ".cursor/rules/rev.mdc"},
		{"qoder", ".qoder/rules/rev.md"},
	} {
		for name, block := range map[string]string{"with a rule field": "  alwaysApply: true\n  foo: bar\n", "alone": "  foo: bar\n"} {
			t.Run(tc.target+"/"+name, func(t *testing.T) {
				spec := "---\nname: rev\ndescription: Reviews\nglobs: '**/*.go'\nx-" + tc.target + ":\n" + block + "---\n\nReview the diff.\n"
				got := importRoundTrip(t, tc.target+", claude", tc.target, "rules/rev.md", spec, tc.native, editBody)
				meta, _ := splitMdcFrontmatter([]byte(got))
				custom, _ := meta["x-"+tc.target].(map[string]any)
				if custom["foo"] != "bar" {
					t.Errorf("x-%s.foo lost:\n%s", tc.target, got)
				}
				if strings.Contains(block, "alwaysApply") && custom["alwaysApply"] != true {
					t.Errorf("x-%s.alwaysApply lost:\n%s", tc.target, got)
				}
			})
		}
	}
}

// Import says which scoped keys it keeps without seeing them: one the
// native file has no field for stays in the spec even when deleted there.
func TestImportScopedTargetKeys_NotesKeysTheNativeFileDoesNotCarry(t *testing.T) {
	root := testutil.TempCwd(t)
	dst := filepath.Join(root, ".agnostic-ai", "skills", "rev", "SKILL.md")
	writeFile(t, dst, "---\nname: rev\nx-amp:\n  metadata:\n    owner: me\n  icon: x\n---\n\nBody.\n")
	var logged strings.Builder
	prev := logOut
	logOut = &logged
	t.Cleanup(func() { logOut = prev })
	got, err := importScopedTargetKeysKeeping([]byte("---\nname: rev\n---\n\nBody.\n"), dst, "amp", nil, importCarrying([]string{"name", "icon"}), true)
	if err != nil {
		t.Fatal(err)
	}
	flushKeptNote(dst, got)
	if want := "---\nname: rev\nx-amp:\n  metadata:\n    owner: me\n---\n\nBody.\n"; string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if want := "note: .agnostic-ai/skills/rev/SKILL.md keeps x-amp.metadata, which the amp file does not carry, so import cannot see a change to it there; edit the spec to change or remove it\n"; logged.String() != want {
		t.Errorf("notes:\n%s\nwant:\n%s", logged.String(), want)
	}
}

// The kept-key note is for an import that changes the spec: an unedited
// native file says nothing, and two keys read as "them".
func TestImport_KeptKeyNoteOnlyWhenTheFileChanged(t *testing.T) {
	root := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [cursor, claude]\n")
	writeFile(t, filepath.Join(root, ".agnostic-ai", "rules", "go.md"), "---\nname: go\ndescription: Go\nglobs: '**/*.go'\nx-cursor:\n  foo: bar\n  baz: 1\n---\n\nReview the diff.\n")
	mustSync(t)
	var logged strings.Builder
	prev := logOut
	logOut = &logged
	t.Cleanup(func() { logOut = prev })
	if out, err := runRoot(t, "import", "cursor", "--overwrite"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if strings.Contains(logged.String(), "note: .agnostic-ai/rules/go.md keeps") {
		t.Errorf("an unedited import printed the note:\n%s", logged.String())
	}
	native := filepath.Join(root, ".cursor", "rules", "go.mdc")
	data, err := os.ReadFile(native)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, native, editBody(string(data)))
	logged.Reset()
	if out, err := runRoot(t, "import", "cursor", "--overwrite"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if want := "note: .agnostic-ai/rules/go.md keeps x-cursor.foo, x-cursor.baz, which the cursor file does not carry, so import cannot see a change to them there; edit the spec to change or remove them\n"; !strings.Contains(logged.String(), want) {
		t.Errorf("notes:\n%s\nwant:\n%s", logged.String(), want)
	}
}
