package cli

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func writingAdapterOnPath(t *testing.T, target, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture requires a POSIX shell")
	}
	bin := t.TempDir()
	script := filepath.Join(bin, "agnostic-ai-adapter-"+target)
	body := "#!/bin/sh\ncat >/dev/null\nprintf '%s' '{\"protocol_version\":1,\"files\":[{\"path\":\"" + path + "\",\"content\":\"from the adapter\\n\"}]}'\n"
	mustWriteFile(t, script, body)
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestSync_MissingExternalAdapterKeepsItsOutputs(t *testing.T) {
	testutil.TempCwd(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, fakex]\ngitignore:\n  enabled: true\n")
	mustWriteFile(t, ".agnostic-ai/rules/r.md", "---\nname: r\ndescription: A rule.\n---\nBe kind.\n")
	silence(t)
	originalPath := os.Getenv("PATH")
	writingAdapterOnPath(t, "fakex", "FAKEX.md")
	syncProject(t)
	if _, err := os.Stat("FAKEX.md"); err != nil {
		t.Fatalf("first sync did not write the adapter output: %v", err)
	}
	firstState := readStateFile(".")
	if gitignore, _ := os.ReadFile(".gitignore"); !strings.Contains(string(gitignore), "FAKEX.md") {
		t.Fatalf("first sync did not ignore FAKEX.md:\n%s", gitignore)
	}

	t.Setenv("PATH", originalPath)
	mustWriteFile(t, ".agnostic-ai/rules/r.md", "---\nname: r\ndescription: A rule.\n---\nBe kinder.\n")
	var logged bytes.Buffer
	prev := logOut
	logOut = &logged
	defer func() { logOut = prev }()
	var out string
	var err error
	stderr := captureStderr(t, func() { out, err = runCLI(t, "sync") })
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}

	if data, readErr := os.ReadFile("FAKEX.md"); readErr != nil || string(data) != "from the adapter\n" {
		t.Errorf("FAKEX.md = %q, error %v; want it kept", data, readErr)
	}
	if !strings.Contains(stderr, "AAI-301") || !strings.Contains(stderr, "fakex") {
		t.Errorf("want the AAI-301 warning naming fakex, got stderr:\n%s", stderr)
	}
	report := logged.String() + out
	if strings.Contains(report, "FAKEX.md") || strings.Contains(report, "removed") {
		t.Errorf("report lists the kept output as removed:\n%s", report)
	}
	if !strings.Contains(stderr+report, "skipped removing stale files: fakex did not load") {
		t.Errorf("sync did not say it skipped the stale file cleanup:\n%s%s", stderr, report)
	}
	if !strings.Contains(report, "synced 1 target ") {
		t.Errorf("summary should count only the target that emitted:\n%s", report)
	}
	state := readStateFile(".")
	if !slices.Contains(state.Outputs, "FAKEX.md") {
		t.Errorf("ledger dropped FAKEX.md: %v", state.Outputs)
	}
	if !maps.Equal(state.SpecSums, firstState.SpecSums) {
		t.Errorf("spec baseline moved on a partial run: before %v, after %v", firstState.SpecSums, state.SpecSums)
	}
	if gitignore, _ := os.ReadFile(".gitignore"); !strings.Contains(string(gitignore), "FAKEX.md") {
		t.Errorf("gitignore lost FAKEX.md:\n%s", gitignore)
	}
}
