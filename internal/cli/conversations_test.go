package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func fakeStartConversation(t *testing.T, err error) *[]conversations.StartRequest {
	t.Helper()
	var asked []conversations.StartRequest
	previous := startConversation
	startConversation = func(_ context.Context, req conversations.StartRequest) (conversations.StartReply, error) {
		asked = append(asked, req)
		if err != nil {
			return conversations.StartReply{}, err
		}
		return conversations.StartReply{Message: "Started Codex in a new Ghostty tab in " + req.Folder + "."}, nil
	}
	t.Cleanup(func() { startConversation = previous })
	return &asked
}

func runRootWithInput(t *testing.T, input string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetIn(strings.NewReader(input))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestConversationsStart_PassesTheBriefFromStandardInput(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	asked := fakeStartConversation(t, nil)

	out, err := runRootWithInput(t, "Write the release notes\n", "conversations", "start", "--tool", "codex")

	if err != nil {
		t.Fatal(err)
	}
	if len(*asked) != 1 {
		t.Fatalf("asked %d times", len(*asked))
	}
	req := (*asked)[0]
	self, _ := os.Executable()
	if req.Tool != conversations.ToolCodex || req.Brief != "Write the release notes\n" || req.Folder != canonicalDir(dir) && req.Folder != dir || req.Self != self || req.PID != os.Getpid() {
		t.Errorf("request = %+v", req)
	}
	if !strings.HasPrefix(out, "Started Codex in a new Ghostty tab in ") {
		t.Errorf("out = %q", out)
	}
}

func TestConversationsStart_ReadsABriefFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brief.md")
	mustWriteFile(t, path, "Tag the release\n")
	asked := fakeStartConversation(t, nil)

	if _, err := runRootWithInput(t, "ignored", "conversations", "start", "--brief-file", path); err != nil {
		t.Fatal(err)
	}

	if req := (*asked)[0]; req.Brief != "Tag the release\n" || req.Tool != "" {
		t.Errorf("request = %+v", req)
	}
	if _, err := runRootWithInput(t, "", "conversations", "start", "--brief-file", filepath.Join(t.TempDir(), "missing.md")); err == nil || !strings.Contains(err.Error(), "could not read the brief") {
		t.Errorf("a missing brief file: err = %v", err)
	}
}

func TestConversationsStart_ReturnsTheRefusal(t *testing.T) {
	fakeStartConversation(t, errors.New("cannot tell which conversation this runs in"))

	out, err := runRootWithInput(t, "Write the notes", "conversations", "start")

	if err == nil || err.Error() != "cannot tell which conversation this runs in" || out != "" {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestConversationsRun_HandsTheIDToTheRunner(t *testing.T) {
	var ran []string
	previous := runBrief
	runBrief = func(id string) error {
		ran = append(ran, id)
		return nil
	}
	t.Cleanup(func() { runBrief = previous })
	id := strings.Repeat("a", 32)

	if _, err := runRoot(t, "conversations", "run", id); err != nil || len(ran) != 1 || ran[0] != id {
		t.Errorf("ran %q err %v", ran, err)
	}
	if _, err := runRoot(t, "conversations", "run"); err == nil {
		t.Error("run needs an id")
	}
	root := NewRootCmd("test")
	run, _, err := root.Find([]string{"conversations", "run"})
	if err != nil || !run.Hidden {
		t.Errorf("run is not hidden: %v", err)
	}
}
