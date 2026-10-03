package hookrun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// Source: charmbracelet/crush 76cc5c574e15072b15aaed0f4f843a5711fae0d9
// (rechecked against main bdcf796 on 2026-10-03: no change under
// internal/hooks or internal/shell). Every contract item is read from
// that source, so hook run assumes none of them:
//   - shell: Crush parses the command with mvdan.cc/sh/v3 v3.14.1 and runs
//     it in process on every platform (internal/shell/run.go:61-92,
//     go.mod:75), with its own handler stack: builtins, then script
//     dispatch, then the block list, which hooks leave empty
//     (run.go:301-311, hooks/runner.go:158-161), then Go core utilities
//     on Windows only (shell/coreutils.go:11-18).
//   - working directory: the project directory (agent/coordinator.go:872,
//     hooks/runner.go:182).
//   - stdin payload: event, session_id, cwd, tool_name, and tool_input
//     (hooks/input.go:23-49); env adds CRUSH_* (hooks/input.go:53-76,
//     shell/shell.go:43-49) and forces non-interactive variables
//     (shell/run.go:222-255).
//   - exit codes: 2 blocks the tool call, 49 halts the whole turn, any
//     other non-zero exit does not block (hooks/runner.go:220-255,
//     hooks/hooks.go:22).
//   - JSON reply at exit 0: decision deny blocks, halt halts, and a
//     hookSpecificOutput object is read as Claude Code's
//     permissionDecision instead (hooks/input.go:80-124,159-182,
//     agent/hooked_tool.go:62-73).
//   - timeout: the hook's timeout in seconds, 30 by default
//     (config/config.go:806-823); a timed-out hook does not block
//     (hooks/runner.go:210-218).
//
// Crush's jq builtin and its Windows core utilities are Go programs
// hook run does not carry. A hook that calls one runs the program on
// PATH instead, and the run says so as an assumption.
const crushSource = "https://github.com/charmbracelet/crush/tree/76cc5c574e15072b15aaed0f4f843a5711fae0d9/internal/hooks"

const (
	crushEvent          = "PreToolUse"
	crushDefaultTimeout = 30 * time.Second
	crushHaltExit       = 49
	// crushAbandonGrace is how long Crush waits for a hook to yield after
	// its timeout (hooks/runner.go:20).
	crushAbandonGrace = time.Second
)

// crushConfigBuiltins are the crushrc builtins Crush registers on every
// shell; without a config builder in the context they do nothing
// (shellconfig/register.go:21-29).
var crushConfigBuiltins = []string{"provider", "model", "mcp", "lsp", "permissions", "hook", "option"}

// crushCoreUtils are the commands mvdan.cc/sh/x/coreutils serves in
// process (x@b5028a3332a4 coreutils/coreutils.go:37-53), which Crush
// installs on Windows unless CRUSH_CORE_UTILS says otherwise.
var crushCoreUtils = []string{
	"cat", "chmod", "cp", "find", "ls", "mkdir", "mv", "rm", "touch", "xargs",
	"base64", "gzcat", "gzip", "gunzip", "mktemp", "shasum", "tar",
}

// crushNonInteractiveEnv replaces the caller's values on every shell
// Crush starts (shell/run.go:222-231).
var crushNonInteractiveEnv = []string{
	"TERM=xterm-256color", "GIT_EDITOR=false", "EDITOR=false", "VISUAL=false",
	"JJ_EDITOR=false", "JJ_PAGER=cat", "GIT_PAGER=cat", "PAGER=cat",
}

func crushMatches(matcher, tool string) (bool, error) {
	if matcher == "" {
		return true, nil
	}
	re, err := regexp.Compile(matcher)
	if err != nil {
		// hooks/runner.go:54-64 skips such a hook with a warning.
		return false, fmt.Errorf("matcher %q: Crush skips a hook whose matcher does not compile: %w", matcher, err)
	}
	return re.MatchString(tool), nil
}

// buildCrush writes the PreToolUse payload. The tool input is what the
// model sends: bash takes command (agent/tools/bash.go:25-31), edit takes
// file_path, old_string, and new_string (tools/edit.go:25-30), and write
// takes file_path and content (tools/write.go:27-30).
func buildCrush(event, matcher, root string, in Input) (Payload, error) {
	if err := checkInput("crush", event, event, "", "", in); err != nil {
		return Payload{}, err
	}
	var input map[string]any
	p := Payload{}
	var err error
	if in.Bash != "" {
		p.Trigger = "bash"
		p.Fires, err = crushMatches(matcher, p.Trigger)
		input = map[string]any{"command": in.Bash, "description": ""}
	} else {
		p.Trigger, p.Fires, err = firstMatch(crushMatches, matcher, []string{"edit", "write"})
		input = map[string]any{"file_path": absPath(root, in.Edit)}
		if p.Trigger == "edit" {
			input["old_string"], input["new_string"] = "", ""
		} else {
			input["content"] = ""
		}
	}
	if err != nil {
		return Payload{}, err
	}
	doc := map[string]any{"event": crushEvent, "session_id": SessionID, "cwd": root, "tool_name": p.Trigger, "tool_input": input}
	return marshal(p, doc)
}

// CrushEnv is the environment Crush gives a hook: the process env, its
// agent markers, the CRUSH_* event variables, and the non-interactive
// overrides, in that order.
func CrushEnv(base []string, root string, body []byte) []string {
	var call struct {
		ToolName  string         `json:"tool_name"`
		ToolInput map[string]any `json:"tool_input"`
	}
	_ = json.Unmarshal(body, &call)
	env := append(slices.Clone(base), "CRUSH=1", "AGENT=crush", "AI_AGENT=crush",
		"CRUSH_EVENT="+crushEvent, "CRUSH_TOOL_NAME="+call.ToolName, "CRUSH_SESSION_ID="+SessionID,
		"CRUSH_CWD="+root, "CRUSH_PROJECT_DIR="+root)
	if v, ok := call.ToolInput["command"]; ok {
		env = append(env, "CRUSH_TOOL_INPUT_COMMAND="+fmt.Sprint(v))
	}
	if v, ok := call.ToolInput["file_path"]; ok {
		env = append(env, "CRUSH_TOOL_INPUT_FILE_PATH="+fmt.Sprint(v))
	}
	env = slices.DeleteFunc(env, func(kv string) bool {
		key, _, _ := strings.Cut(kv, "=")
		return slices.ContainsFunc(crushNonInteractiveEnv, func(o string) bool { return strings.HasPrefix(o, key+"=") })
	})
	return append(env, crushNonInteractiveEnv...)
}

// lockedBuffer lets a hook abandoned after its timeout keep writing
// while the run reads what it wrote so far.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// crushShell runs hook commands with Crush's handler stack and records
// each Go program of Crush's that it had to take from PATH.
type crushShell struct {
	goos      string
	coreUtils bool
	mu        sync.Mutex
	fromPath  []string
}

func (s *crushShell) took(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.Contains(s.fromPath, name) {
		s.fromPath = append(s.fromPath, name)
	}
}

func (s *crushShell) runner(dir string, env expand.Environ, stdin io.Reader, stdout, stderr io.Writer, params []string) (*interp.Runner, error) {
	opts := []interp.RunnerOption{
		interp.StdIO(stdin, stdout, stderr),
		interp.Interactive(false),
		interp.Env(env),
		interp.Dir(dir),
		interp.ExecHandlers(s.builtins, s.scriptDispatch, s.coreUtilsFromPath),
	}
	if runtime.GOOS != "windows" {
		opts = append(opts, interp.ExecHandlers(func(interp.ExecHandlerFunc) interp.ExecHandlerFunc { return crushExec }))
	}
	if len(params) > 0 {
		opts = append(opts, interp.Params(append([]string{"--"}, params...)...))
	}
	return interp.New(opts...)
}

func (s *crushShell) builtins(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		switch {
		case len(args) > 0 && slices.Contains(crushConfigBuiltins, args[0]):
			return nil
		case len(args) > 0 && args[0] == "jq":
			s.took("jq")
		}
		return next(ctx, args)
	}
}

func (s *crushShell) coreUtilsFromPath(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		if s.coreUtils && len(args) > 0 && slices.Contains(crushCoreUtils, args[0]) {
			s.took(args[0])
		}
		return next(ctx, args)
	}
}

// crushExec starts a program as shell/exec_unix.go:44-108 does. Unlike
// mvdan's default handler, which Crush keeps on Windows
// (exec_windows.go:20-23), it does not run a file the kernel cannot
// execute as a shell script: that fails, and the hook reads as exit 1.
func crushExec(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	path, err := interp.LookPathDir(hc.Dir, hc.Env, args[0])
	if err != nil {
		_, _ = fmt.Fprintln(hc.Stderr, err)
		return interp.ExitStatus(127)
	}
	cmd := exec.CommandContext(ctx, path)
	cmd.Args, cmd.Dir, cmd.Env = args, hc.Dir, exportedEnv(hc.Env)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = hc.Stdin, hc.Stdout, hc.Stderr
	killTree(cmd)
	err = cmd.Run()
	var exit *exec.ExitError
	var notStarted *exec.Error
	switch {
	case err == nil:
		return nil
	case errors.As(err, &exit):
		if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return interp.ExitStatus(128 + uint8(status.Signal()))
		}
		return interp.ExitStatus(uint8(exit.ExitCode()))
	case errors.As(err, &notStarted):
		_, _ = fmt.Fprintln(hc.Stderr, err)
		return interp.ExitStatus(127)
	}
	return err
}

func exportedEnv(env expand.Environ) []string {
	var out []string
	env.Each(func(name string, v expand.Variable) bool {
		if v.Exported && v.Kind == expand.String {
			out = append(out, name+"="+v.Str)
		}
		return true
	})
	return out
}

// scriptDispatch follows shell/dispatch.go:48-73: a command named by a
// path runs through its shebang interpreter, a binary runs as is, and
// any other file runs as shell source in a nested runner.
func (s *crushShell) scriptDispatch(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		if len(args) == 0 || !s.pathPrefixed(args[0]) {
			return next(ctx, args)
		}
		hc := interp.HandlerCtx(ctx)
		path := args[0]
		if !filepath.IsAbs(path) {
			path = filepath.Join(hc.Dir, path)
		}
		head, err := readHead(path, 128)
		if err != nil {
			return err
		}
		switch {
		case bytes.HasPrefix(head, []byte("#!")):
			return s.shebang(ctx, path, head, args)
		case crushBinary(head):
			return next(ctx, args)
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := syntax.NewParser().Parse(bytes.NewReader(source), path)
		if err != nil {
			return fmt.Errorf("could not parse %s: %w", path, err)
		}
		nested, err := s.runner(hc.Dir, hc.Env, hc.Stdin, hc.Stdout, hc.Stderr, args[1:])
		if err != nil {
			return err
		}
		return nested.Run(ctx, file)
	}
}

// pathPrefixed follows shell/dispatch.go:84-104.
func (s *crushShell) pathPrefixed(arg string) bool {
	if strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "../") || strings.HasPrefix(arg, "/") {
		return true
	}
	if s.goos != "windows" {
		return false
	}
	drive := len(arg) >= 3 && (arg[0]|0x20) >= 'a' && (arg[0]|0x20) <= 'z' && arg[1] == ':' && (arg[2] == '\\' || arg[2] == '/')
	return drive || strings.HasPrefix(arg, `\`)
}

func readHead(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, n)
	read, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return head[:read], nil
}

// crushBinary follows shell/dispatch.go:149-167: a NUL byte or the
// magic number of a PE, ELF, or Mach-O file.
func crushBinary(head []byte) bool {
	if bytes.IndexByte(head, 0) >= 0 {
		return true
	}
	for _, magic := range []string{"MZ", "\x7fELF", "\xfe\xed\xfa\xce", "\xfe\xed\xfa\xcf", "\xcf\xfa\xed\xfe", "\xce\xfa\xed\xfe", "\xca\xfe\xba\xbe"} {
		if bytes.HasPrefix(head, []byte(magic)) {
			return true
		}
	}
	return false
}

// shebang runs the interpreter the first line names, as
// shell/dispatch.go:174-224 does: the literal path, or its base name on
// PATH when that path does not exist, so #!/bin/bash finds Git Bash on
// Windows.
func (s *crushShell) shebang(ctx context.Context, path string, head []byte, args []string) error {
	hc := interp.HandlerCtx(ctx)
	program, extra, err := parseCrushShebang(head)
	if err == nil {
		program, err = crushInterpreter(program)
		if err != nil {
			_, _ = fmt.Fprintf(hc.Stderr, "crush: %s: %v\n", path, err)
			return interp.ExitStatus(127)
		}
	} else {
		_, _ = fmt.Fprintf(hc.Stderr, "crush: %s: %v\n", path, err)
		return interp.ExitStatus(126)
	}
	cmd := exec.CommandContext(ctx, program, append(append(extra, path), args[1:]...)...)
	cmd.Dir, cmd.Env, cmd.Stdin, cmd.Stdout, cmd.Stderr = hc.Dir, exportedEnv(hc.Env), hc.Stdin, hc.Stdout, hc.Stderr
	err = cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code := exit.ExitCode()
		if code < 0 {
			code = 1
		}
		return interp.ExitStatus(uint8(code))
	}
	return err
}

// parseCrushShebang follows shell/dispatch.go:267-370: a literal path
// passes the rest of the line as one argument; env names the program,
// and only `env -S` splits the rest on whitespace.
func parseCrushShebang(head []byte) (string, []string, error) {
	line, _, _ := bytes.Cut(head[2:], []byte("\n"))
	text := strings.TrimLeft(strings.TrimRight(string(line), "\r"), " \t")
	if text == "" {
		return "", nil, errors.New("empty shebang")
	}
	program, rest := text, ""
	if i := strings.IndexAny(text, " \t"); i >= 0 {
		program, rest = text[:i], strings.TrimLeft(text[i+1:], " \t")
	}
	if program != "/usr/bin/env" && program != "/bin/env" && filepath.Base(program) != "env" {
		if rest == "" {
			return program, nil, nil
		}
		return program, []string{rest}, nil
	}
	split := false
	if strings.HasPrefix(rest, "-") {
		flag, after := rest, ""
		if i := strings.IndexAny(rest, " \t"); i >= 0 {
			flag, after = rest[:i], rest[i+1:]
		}
		if flag != "-S" {
			return "", nil, fmt.Errorf("unsupported env flag: %s", flag)
		}
		split, rest = true, strings.TrimLeft(after, " \t")
	}
	if rest == "" {
		return "", nil, errors.New("env: missing program name")
	}
	fields := strings.Fields(rest)
	program = fields[0]
	remainder := strings.TrimLeft(strings.TrimPrefix(rest, program), " \t")
	switch {
	case remainder == "":
		return program, nil, nil
	case split:
		return program, fields[1:], nil
	}
	return program, []string{remainder}, nil
}

func crushInterpreter(path string) (string, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return path, nil
	case !errors.Is(err, os.ErrNotExist):
		return "", err
	}
	resolved, err := exec.LookPath(filepath.Base(path))
	if err != nil {
		return "", fmt.Errorf("interpreter %q not found in PATH", path)
	}
	return resolved, nil
}

// crushCoreUtilsOn reads CRUSH_CORE_UTILS the way shell/coreutils.go:11-18
// does: a boolean wins, and Windows is the default.
func crushCoreUtilsOn(goos string) bool {
	if on, err := strconv.ParseBool(os.Getenv("CRUSH_CORE_UTILS")); err == nil {
		return on
	}
	return goos == "windows"
}

// RunCrush runs command in Crush's embedded shell with env and stdin,
// from dir. Like hooks/runner.go:172-218, it gives up on a hook that
// does not yield within a second of its timeout.
func RunCrush(command, dir string, env []string, stdin []byte, timeout time.Duration, goos string) Result {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	file, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil {
		return Result{StartErr: fmt.Errorf("could not parse command: %w", err)}
	}
	shell := &crushShell{goos: goos, coreUtils: crushCoreUtilsOn(goos)}
	var stdout, stderr lockedBuffer
	runner, err := shell.runner(dir, expand.ListEnviron(env...), bytes.NewReader(stdin), &stdout, &stderr, nil)
	if err != nil {
		return Result{StartErr: err}
	}
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx, file) }()
	select {
	case err = <-done:
	case <-ctx.Done():
		select {
		case err = <-done:
		case <-time.After(crushAbandonGrace):
			err = ctx.Err()
		}
	}
	r := Result{Stdout: stdout.String(), Stderr: stderr.String(), Elapsed: time.Since(start)}
	shell.mu.Lock()
	r.FromPath = slices.Clone(shell.fromPath)
	shell.mu.Unlock()
	var status interp.ExitStatus
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		r.TimedOut = true
	case errors.As(err, &status):
		r.Exit = int(status)
	case err != nil:
		// shell.ExitCode reads any other error as exit 1 (shell/shell.go:304-312).
		r.Exit = 1
		r.Stderr += err.Error() + "\n"
	}
	return r
}

type crushReply struct {
	Decision           string          `json:"decision"`
	Halt               bool            `json:"halt"`
	Context            json.RawMessage `json:"context"`
	HookSpecificOutput *struct {
		PermissionDecision string `json:"permissionDecision"`
		AdditionalContext  string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

func readCrushReply(r Result) (crushReply, bool) {
	var reply crushReply
	ok := json.Unmarshal([]byte(strings.TrimSpace(r.Stdout)), &reply) == nil
	return reply, ok
}

// decideCrush reads a result as hooks/runner.go:210-264 and
// agent/hooked_tool.go:62-73 do.
func decideCrush(r Result) Decision {
	switch {
	case r.TimedOut:
		return Timeout
	case r.StartErr != nil:
		return Error
	case r.Exit == 2 || r.Exit == crushHaltExit:
		return Block
	case r.Exit != 0:
		return Error
	}
	reply, ok := readCrushReply(r)
	switch {
	case !ok:
		return Allow
	case reply.HookSpecificOutput != nil:
		if strings.EqualFold(reply.HookSpecificOutput.PermissionDecision, "deny") {
			return Block
		}
	case strings.EqualFold(reply.Decision, "deny") || reply.Halt:
		return Block
	}
	return Allow
}

// CrushHalts reports whether Crush ends the whole turn on r, not only
// the tool call: exit 49, or halt in a JSON reply.
func CrushHalts(r Result) bool {
	if r.TimedOut || r.StartErr != nil {
		return false
	}
	if r.Exit == crushHaltExit {
		return true
	}
	reply, ok := readCrushReply(r)
	return r.Exit == 0 && ok && reply.HookSpecificOutput == nil && reply.Halt
}

// crushAddsContext reports whether Crush appends the reply's context to
// the tool result, which it does only when the tool runs.
func crushAddsContext(r Result) bool {
	reply, ok := readCrushReply(r)
	if !ok || decideCrush(r) != Allow {
		return false
	}
	if reply.HookSpecificOutput != nil {
		return reply.HookSpecificOutput.AdditionalContext != ""
	}
	var text string
	var list []string
	switch {
	case json.Unmarshal(reply.Context, &text) == nil:
		return text != ""
	case json.Unmarshal(reply.Context, &list) == nil:
		return slices.ContainsFunc(list, func(s string) bool { return s != "" })
	}
	return false
}

// CrushAssumptions names what r ran from PATH that Crush runs as its
// own Go program.
func CrushAssumptions(r Result) []Assumption {
	var out []Assumption
	if slices.Contains(r.FromPath, "jq") {
		out = append(out, Assumption{Item: "jq", Value: "jq on PATH", Reason: "Crush runs jq as its own gojq builtin (internal/shell/jq.go)"})
	}
	var utils []string
	for _, name := range r.FromPath {
		if name != "jq" {
			utils = append(utils, name)
		}
	}
	if len(utils) > 0 {
		out = append(out, Assumption{Item: "coreutils", Value: strings.Join(utils, ", ") + " on PATH", Reason: "Crush runs its Go core utilities on Windows (internal/shell/coreutils.go); set CRUSH_CORE_UTILS=false to match"})
	}
	return out
}

// crushEntry is one hook in crush.json; Crush lists hooks flat under
// the event (config/config.go:795-808).
type crushEntry struct {
	Name    string  `json:"name"`
	Matcher string  `json:"matcher"`
	Command string  `json:"command"`
	Timeout float64 `json:"timeout"`
}

type crushDoc struct {
	Hooks map[string][]crushEntry `json:"hooks"`
}

// CrushHandlers reads the handlers out of the crush.json hooks sync
// writes for one spec.
func CrushHandlers(body []byte) ([]Handler, error) {
	if len(body) == 0 {
		return nil, nil
	}
	var doc crushDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	var out []Handler
	for _, e := range doc.Hooks[crushEvent] {
		out = append(out, Handler{Command: e.Command, Timeout: time.Duration(e.Timeout * float64(time.Second))})
	}
	return out, nil
}

// crushDrift names each handler crush.json does not run as the spec
// says: the command, the matcher, and the timeout.
func crushDrift(body []byte, matcher string, handlers []Handler) ([]HandlerDrift, error) {
	var doc crushDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	var drift []HandlerDrift
	for _, h := range handlers {
		reason := fmt.Sprintf("has no %s command %q", crushEvent, h.Command)
		for _, e := range doc.Hooks[crushEvent] {
			if e.Command != h.Command {
				continue
			}
			got := time.Duration(e.Timeout * float64(time.Second))
			switch {
			case e.Matcher != matcher:
				reason = fmt.Sprintf("runs %q with matcher %q, not %q", h.Command, e.Matcher, matcher)
			case got != h.Timeout:
				reason = fmt.Sprintf("runs %q with timeout %s, not %s", h.Command, got, h.Timeout)
			default:
				reason = ""
			}
			if reason == "" {
				break
			}
		}
		if reason != "" {
			drift = append(drift, HandlerDrift{Handler: h, Reason: reason})
		}
	}
	return drift, nil
}

func init() {
	otherBuilders["crush"] = buildCrush
	otherDefaultTimeouts["crush"] = crushDefaultTimeout
}
