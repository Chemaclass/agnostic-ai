package conversations

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// briefLimit bounds a brief, the new conversation's first prompt.
const briefLimit = 4096

// briefsKept is how long a brief no tab ran stays before a later start
// removes it.
const briefsKept = 24 * time.Hour

// briefFile is the name of a one-time brief: 32 hex digits.
var briefFile = regexp.MustCompile(`^([0-9a-f]{32})\.json$`)

var briefID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// startCommands are the tools a conversation can start in, by the
// command that starts each with a first prompt as its one argument.
// Cursor's terminal agent has not been checked on a real install.
var startCommands = map[Tool]string{ToolClaude: "claude", ToolCodex: "codex"}

var errCursorStart = errors.New("the Cursor terminal agent is not supported yet: starting a linked conversation in it has not been checked")

// DefaultBriefsDir is where one-time briefs wait for their tab, beside
// the marks file.
func DefaultBriefsDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "agnostic-ai", "conversation-briefs")
}

// storedBrief is what a one-time brief file holds.
type storedBrief struct {
	Tool   Tool   `json:"tool"`
	Prompt string `json:"prompt"`
}

// Starter starts a conversation linked to the one it runs in, in a new
// terminal tab. The brief never enters a shell or AppleScript string:
// the tab runs a fixed command that names a one-time brief file, and
// BriefRunner starts the tool with the brief as one argument.
type Starter struct {
	Stores    Stores
	Processes Processes
	Launcher  Launcher
	// LookPath finds a command, as exec.LookPath does.
	LookPath func(string) (string, error)
	Getenv   func(string) string
	// BriefsDir holds the one-time brief files.
	BriefsDir string
	Now       func() time.Time
}

// StartRequest is one conversation to start.
type StartRequest struct {
	// Tool is the new conversation's tool; empty uses the parent's.
	Tool Tool
	// Folder is where it starts.
	Folder string
	Brief  string
	// Self is this agnostic-ai binary, which the new tab runs.
	Self string
	// PID is the process that asks; its ancestors name the parent.
	PID int
}

// StartReply says what started, and where.
type StartReply struct {
	Parent  Ref
	Tool    Tool
	Message string
}

// Start checks the brief, the parent and the tool, writes the brief,
// and opens the tab. Nothing is written or opened when a check fails.
// When the tab cannot be opened, the error holds the line that runs
// the brief by hand.
func (s Starter) Start(ctx context.Context, req StartRequest) (StartReply, error) {
	if req.Tool == ToolCursor {
		return StartReply{}, errCursorStart
	}
	if _, known := startCommands[req.Tool]; req.Tool != "" && !known {
		return StartReply{}, fmt.Errorf("unknown tool %q: use claude or codex", req.Tool)
	}
	for what, value := range map[string]string{"folder": req.Folder, "agnostic-ai path": req.Self} {
		if hasControl(value) {
			return StartReply{}, fmt.Errorf("the %s %q holds a control character, which a terminal would act on as it is typed; nothing started", what, value)
		}
	}
	brief := strings.TrimSpace(req.Brief)
	if brief == "" {
		return StartReply{}, errors.New("no brief: a conversation never starts without one; pass it on standard input or with --brief-file")
	}
	if len(brief) > briefLimit {
		return StartReply{}, fmt.Errorf("the brief is %d bytes; keep it to %d", len(brief), briefLimit)
	}
	table, err := s.Processes.Table()
	if err != nil {
		return StartReply{}, fmt.Errorf("could not read the process table: %w", err)
	}
	parent, _, err := currentSession(table, s.Processes, s.Stores, s.Getenv, req.PID)
	if err != nil {
		return StartReply{}, err
	}
	tool := req.Tool
	if tool == "" {
		tool = parent.Tool
	}
	command, ok := startCommands[tool]
	if !ok {
		return StartReply{}, errCursorStart
	}
	if _, err := s.LookPath(command); err != nil {
		return StartReply{}, fmt.Errorf("%s is not installed: no %s command on PATH", toolLabels[tool], command)
	}
	s.sweepBriefs()
	id, err := s.writeBrief(storedBrief{Tool: tool, Prompt: linkLine(parent) + "\n\n" + brief})
	if err != nil {
		return StartReply{}, fmt.Errorf("could not save the brief: %w", err)
	}
	line := resumeIn(req.Folder, shellQuote(req.Self)+" conversations run "+id)
	opened := openInTab(ctx, s.Launcher, terminalAppOf(table, req.PID, s.Getenv), line)
	if opened.err != nil {
		return StartReply{}, notOpened(opened, line)
	}
	where := fmt.Sprintf("a new %s tab", opened.app)
	if opened.window {
		where = "a new Terminal window"
	}
	in := ""
	if req.Folder != "" {
		in = " in " + req.Folder
	}
	return StartReply{
		Parent:  parent,
		Tool:    tool,
		Message: fmt.Sprintf("Started %s in %s%s, linked to %s:%s.", toolLabels[tool], where, in, parent.Tool, parent.ID),
	}, nil
}

func notOpened(opened tabOpening, line string) error {
	byHand := "run this line in a terminal: " + line
	switch {
	case errors.Is(opened.err, errNotHere):
		return errors.New("this system cannot open a terminal tab; " + byHand)
	case errors.Is(opened.err, errAutomationDenied):
		return fmt.Errorf("macOS blocked opening %s; allow the terminal that runs this command to control %s in System Settings › Privacy & Security › Automation, or %s", opened.app, opened.app, byHand)
	case errors.Is(opened.err, context.DeadlineExceeded):
		return fmt.Errorf("macOS is waiting for you to allow control of %s; answer its prompt, or %s", opened.app, byHand)
	case opened.window:
		return fmt.Errorf("could not open a Terminal window (%v); %s", opened.err, byHand)
	default:
		return fmt.Errorf("could not open a %s tab (%v); %s", opened.app, opened.err, byHand)
	}
}

// terminalAppOf is the app that shows the terminal pid runs in: the
// nearest ancestor with a terminal names it. A Codex thread runs its
// commands from a background process with none, so TERM_PROGRAM, which
// it inherits, says it then.
func terminalAppOf(table []ProcessInfo, pid int, getenv func(string) string) string {
	byPID := make(map[int]ProcessInfo, len(table))
	for _, process := range table {
		byPID[process.PID] = process
	}
	process, found := byPID[pid]
	for range maxSessionAncestors {
		if !found {
			break
		}
		if process.TTY != "" {
			if host, ok := hostOf(table, process.PID); ok {
				return host.App
			}
			break
		}
		process, found = byPID[process.PPID]
	}
	switch getenv("TERM_PROGRAM") {
	case "ghostty":
		return "Ghostty"
	case "Apple_Terminal":
		return "Terminal"
	}
	return ""
}

// briefsFolder makes the folder that holds briefs, readable by its
// owner only, and refuses one that is a link or another user's.
func (s Starter) briefsFolder() error {
	if s.BriefsDir == "" {
		return errors.New("no folder for briefs on this system")
	}
	if err := os.MkdirAll(s.BriefsDir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(s.BriefsDir)
	if err != nil {
		return err
	}
	if !info.IsDir() || !ownedByMe(info) {
		return fmt.Errorf("%s is not a folder of this user", s.BriefsDir)
	}
	if info.Mode().Perm() != 0o700 {
		return os.Chmod(s.BriefsDir, 0o700)
	}
	return nil
}

// writeBrief saves brief under a new random name, readable by its
// owner only, and returns that name's id.
func (s Starter) writeBrief(brief storedBrief) (string, error) {
	if err := s.briefsFolder(); err != nil {
		return "", err
	}
	data, err := json.Marshal(brief)
	if err != nil {
		return "", err
	}
	random := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, random); err != nil {
		return "", err
	}
	id := hex.EncodeToString(random)
	path := filepath.Join(s.BriefsDir, id+".json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	_, err = file.Write(data)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return id, nil
}

// sweepBriefs removes briefs no tab ran within a day.
func (s Starter) sweepBriefs() {
	entries, err := os.ReadDir(s.BriefsDir)
	if err != nil {
		return
	}
	cutoff := s.Now().Add(-briefsKept)
	for _, entry := range entries {
		if !briefFile.MatchString(entry.Name()) || !entry.Type().IsRegular() {
			continue
		}
		if info, err := entry.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(s.BriefsDir, entry.Name()))
		}
	}
}

// BriefRunner runs a one-time brief in the tab a start opened: it reads
// and removes the brief, then replaces itself with the tool, the brief
// its one argument.
type BriefRunner struct {
	BriefsDir string
	LookPath  func(string) (string, error)
	// Exec replaces this process with path, as syscall.Exec does.
	Exec    func(path string, argv, env []string) error
	Environ func() []string
}

// Run starts the brief id names. The id is checked before any path is
// built from it.
func (r BriefRunner) Run(id string) error {
	if !briefID.MatchString(id) {
		return fmt.Errorf("%q is not a brief id", id)
	}
	brief, err := r.take(filepath.Join(r.BriefsDir, id+".json"))
	if err != nil {
		return err
	}
	command, ok := startCommands[brief.Tool]
	if !ok || !strings.HasPrefix(brief.Prompt, linkWord) {
		return errors.New("the brief names no tool this version starts")
	}
	path, err := r.LookPath(command)
	if err != nil {
		return fmt.Errorf("%s is not installed: no %s command on PATH", toolLabels[brief.Tool], command)
	}
	return r.Exec(path, []string{command, brief.Prompt}, r.Environ())
}

// take reads the brief at path and removes it, so it runs once. It
// refuses a link, a file of another user, and one others can read.
func (r BriefRunner) take(path string) (storedBrief, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return storedBrief{}, errors.New("this brief was already used or has expired; nothing started")
	}
	if err != nil {
		return storedBrief{}, err
	}
	switch {
	case !info.Mode().IsRegular():
		return storedBrief{}, fmt.Errorf("%s is not a regular file; nothing started", path)
	case !ownedByMe(info):
		return storedBrief{}, fmt.Errorf("%s belongs to another user; nothing started", path)
	case info.Mode().Perm()&0o077 != 0:
		return storedBrief{}, fmt.Errorf("others can read %s; nothing started", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return storedBrief{}, err
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return storedBrief{}, fmt.Errorf("%s changed while it was read; nothing started", path)
	}
	data, err := io.ReadAll(io.LimitReader(file, 4*briefLimit))
	if err != nil {
		return storedBrief{}, err
	}
	if err := os.Remove(path); err != nil {
		return storedBrief{}, err
	}
	var brief storedBrief
	if err := json.Unmarshal(data, &brief); err != nil {
		return storedBrief{}, fmt.Errorf("%s is not a brief: %w", path, err)
	}
	return brief, nil
}
