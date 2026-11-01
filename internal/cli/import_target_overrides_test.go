package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A key the spec sets only under x-<target> reaches that tool's agent
// file at the top level. Importing the file back must return it under
// x-<target>, or every other tool gets it on the next sync.
func TestImport_AgentTargetOverridesStayScoped(t *testing.T) {
	for _, tc := range []struct{ target, dir string }{
		{"cursor", ".cursor/agents"},
		{"claude", ".claude/agents"},
		{"qoder", ".qoder/agents"},
		{"junie", ".junie/agents"},
		{"gemini", ".gemini/agents"},
		{"opencode", ".opencode/agents"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			root := testutil.TempCwd(t)
			silence(t)
			writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, "+tc.target+"]\n")
			specFile := filepath.Join(root, ".agnostic-ai", "agents", "rev.md")
			spec := "---\nname: rev\ndescription: Reviews\nx-" + tc.target + ":\n  foo: bar # scoped\n---\n\nReview the diff.\n"
			writeFile(t, specFile, spec)
			mustSync(t)
			native := filepath.Join(root, tc.dir, "rev.md")
			data, err := os.ReadFile(native)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), "foo: bar") {
				t.Skipf("%s does not write x-%s.foo:\n%s", tc.target, tc.target, data)
			}
			writeFile(t, native, strings.Replace(string(data), "Review the diff.", "Review the whole diff.", 1))
			if out, err := runRoot(t, "import", tc.target, "--overwrite"); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			got, err := os.ReadFile(specFile)
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Replace(spec, "Review the diff.", "Review the whole diff.", 1)
			if string(got) != want {
				t.Errorf("spec after import:\n%s\nwant:\n%s", got, want)
			}
			if tc.target != "claude" {
				mustSync(t)
				after, err := os.ReadFile(filepath.Join(root, ".claude", "agents", "rev.md"))
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(after), "foo:") {
					t.Errorf("Claude Code got the scoped key:\n%s", after)
				}
			}
			if out, err := runRoot(t, "sync", "--check"); err != nil {
				t.Errorf("sync --check after import: %v\n%s", err, out)
			}
		})
	}
}

// An override of a shared key takes the edited native value, the shared
// value stays, a scoped key deleted from the native file goes, and a
// delete marker or a key Cursor never writes stays.
func TestImportScopedTargetKeys_SharedAndDeletedKeys(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "rev.md")
	writeFile(t, dst, "---\nname: rev\nmodel: shared\nx-cursor:\n  model: cm # cursor only\n  readonly: true\n  color: null\n  tools: [Read]\n---\n\nBody.\n")
	native := "---\nname: rev\nmodel: cm2\n---\n\nBody.\n"
	got, err := importScopedTargetKeys([]byte(native), dst, "cursor", "tools")
	if err != nil {
		t.Fatal(err)
	}
	want := "---\nname: rev\nmodel: shared\nx-cursor:\n  model: cm2 # cursor only\n  color: null\n  tools: [Read]\n---\n\nBody.\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	plain := filepath.Join(t.TempDir(), "plain.md")
	writeFile(t, plain, "---\nname: rev\nmodel: shared\n---\n\nBody.\n")
	if got, _ := importScopedTargetKeys([]byte(native), plain, "cursor"); string(got) != native {
		t.Errorf("a file whose spec scopes nothing changed:\n%s", got)
	}
	got, err = importScopedTargetKeys([]byte("---\nname: rev\n---\n\nBody.\n"), dst, "cursor", "tools")
	if err != nil {
		t.Fatal(err)
	}
	if want := "---\nname: rev\nx-cursor:\n  color: null\n  tools: [Read]\n---\n\nBody.\n"; string(got) != want {
		t.Errorf("deleting every written scoped key natively:\n%s\nwant:\n%s", got, want)
	}
}

// Review cases: each spec scopes a key, the native file is synced and its
// body edited, and the re-imported spec must keep both the shared and the
// scoped values as written.
func TestImport_AgentScopedValuesRoundTripAsWritten(t *testing.T) {
	cases := []struct {
		name, target, dir, fields, edit, edited string
		others                                  string
		absent                                  string
	}{
		{name: "gemini scoped tools", target: "gemini", dir: ".gemini/agents",
			fields: "tools: [Read, Grep]\nx-gemini:\n  tools: [read_file] # gemini names\n"},
		{name: "cursor lone key deleted", target: "cursor", dir: ".cursor/agents",
			fields: "x-cursor:\n  foo: bar\n", edit: "foo: bar\n", edited: "", others: "claude", absent: "foo:"},
		{name: "junie nested override", target: "junie", dir: ".junie/agents",
			fields: "x-junie:\n  extra: {zed: 1, alpha: [x, y]} # nested\n  list: [b, a]\n"},
		{name: "qoder scoped tool list", target: "qoder", dir: ".qoder/agents",
			fields: "x-qoder:\n  tools: [Read, Bash]\n"},
		{name: "claude scoped limits", target: "claude", dir: ".claude/agents",
			fields: "model: sonnet\ntools: [Read, Grep]\nx-claude:\n  model: opus\n  tools: [Read, \"Bash(git *)\"]\n  disallowedTools: [Write]\n", others: "cursor", absent: "Write"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := testutil.TempCwd(t)
			silence(t)
			targets := tc.target
			if tc.others != "" {
				targets += ", " + tc.others
			}
			writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: ["+targets+"]\n")
			specFile := filepath.Join(root, ".agnostic-ai", "agents", "rev.md")
			spec := "---\nname: rev\ndescription: Reviews\n" + tc.fields + "---\n\nReview the diff.\n"
			writeFile(t, specFile, spec)
			mustSync(t)
			native := filepath.Join(root, tc.dir, "rev.md")
			data, err := os.ReadFile(native)
			if err != nil {
				t.Fatal(err)
			}
			body := strings.Replace(string(data), "Review the diff.", "Review the whole diff.", 1)
			if tc.edit != "" {
				body = strings.Replace(body, tc.edit, "", 1)
			}
			writeFile(t, native, body)
			if out, err := runRoot(t, "import", tc.target, "--overwrite"); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			got, err := os.ReadFile(specFile)
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Replace(spec, "Review the diff.", "Review the whole diff.", 1)
			if tc.edit != "" {
				want = "---\nname: rev\ndescription: Reviews\n" + tc.edited + "---\n\nReview the whole diff.\n"
			}
			if string(got) != want {
				t.Errorf("spec after import:\n%s\nwant:\n%s", got, want)
			}
			mustSync(t)
			if tc.others != "" {
				dir := map[string]string{"claude": ".claude/agents", "cursor": ".cursor/agents"}[tc.others]
				other, _ := os.ReadFile(filepath.Join(root, dir, "rev.md"))
				if strings.Contains(string(other), tc.absent) {
					t.Errorf("%s got a scoped value:\n%s", tc.others, other)
				}
			}
			if tc.edit != "" {
				again, _ := os.ReadFile(native)
				if strings.Contains(string(again), tc.absent) {
					t.Errorf("sync wrote the deleted key back:\n%s", again)
				}
			}
			if out, err := runRoot(t, "sync", "--check"); err != nil {
				t.Errorf("sync --check after import: %v\n%s", err, out)
			}
		})
	}
}

func TestImportScopedTargetKeysKeeping_PreservesUnwrittenKeyWithIncomingBlock(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "reviewer.md")
	writeFile(t, dst, "---\nname: reviewer\nx-openhands:\n  tools: [terminal]\n  skills: [review]\n---\n\nReview.\n")
	native := []byte("---\nname: reviewer\nx-openhands:\n  skills: [review]\n---\n\nReview.\n")
	got, err := importScopedTargetKeysKeeping(native, dst, "openhands", nil, func(key string) bool { return key == "tools" }, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "terminal") {
		t.Errorf("unwritten scoped tools lost: %s", got)
	}
}
