package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSyncGlobal_GrokRejectsInvalidSkillBeforeNativeWrites(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("GROK_HOME", "")
	instructions := filepath.Join(home, ".grok", "AGENTS.md")
	before := "Personal instructions.\n"
	mustWriteGlobalTest(t, instructions, before)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Shared instructions.\n")
	name := strings.Repeat("a", 65)
	mustWriteGlobalTest(t, filepath.Join(source, "skills", name+".md"), "---\nname: "+name+"\ndescription: Inspect a diff.\n---\nRead the diff.\n")
	if _, _, err := runGlobalAgentTest("--only", "grok"); err == nil || !strings.Contains(err.Error(), "64") {
		t.Fatalf("invalid global native identity accepted: %v", err)
	}
	if got := readGlobalTest(t, instructions); got != before {
		t.Errorf("instructions changed before validation: %q", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".grok", "skills", name)); !os.IsNotExist(err) {
		t.Errorf("invalid skill output exists: %v", err)
	}
}

func TestImportGrok_SkillNamesKeepNativeFallbacksOnFreshSync(t *testing.T) {
	for _, tc := range []struct {
		label, folder, name string
	}{
		{"directory-fallback", "review", "'!!!'"},
		{"native-name", strings.Repeat("a", 65), "review"},
		{"numeric-native-name", strings.Repeat("a", 65), "42"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [grok]\ngitignore:\n  enabled: true\n  commit: [grok:instructions]\n")
			path := filepath.Join(dir, "backend", ".grok", "skills", tc.folder, "SKILL.md")
			writeFile(t, path, "---\nname: "+tc.name+"\ndescription: Inspect a diff.\n---\nRead the supplied diff.\n")
			execCLI(t, "import", "grok")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			execCLI(t, "sync", "-t", "grok")
			if data := readFile(t, path); !strings.Contains(data, "Read the supplied diff.") || !strings.Contains(data, strings.Trim(tc.name, "'")) {
				t.Errorf("native name or fallback lost: %s", data)
			}
			execCLI(t, "sync", "--check", "-t", "grok")
			execCLI(t, "lint")
		})
	}
}

func TestSyncGlobal_GrokKeepsNativeDirectoryNameFallback(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("GROK_HOME", "")
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "review.md"), "---\nname: review\ndescription: Inspect a diff.\nx-grok:\n  name: '!!!'\n---\nRead the supplied diff.\n")
	if _, _, err := runGlobalAgentTest("--only", "grok"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".grok", "skills", "review", "SKILL.md")
	if data := readGlobalTest(t, path); !strings.Contains(data, "!!!") || !strings.Contains(data, "Read the supplied diff.") {
		t.Errorf("native fallback content changed: %s", data)
	}
	if _, _, err := runGlobalAgentTest("--only", "grok", "--check"); err != nil {
		t.Fatal(err)
	}
}
