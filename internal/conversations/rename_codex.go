package conversations

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// This file is the only code that talks to Codex's app-server. It runs
// `codex app-server` over stdio and asks it, in JSON-RPC, to name one
// thread with thread/name/set, as the protocol schema of Codex 0.167.0
// describes. The app-server writes the name where Codex keeps it.

// codexAppServerTimeout bounds one rename, from start to exit.
const codexAppServerTimeout = 20 * time.Second

// codexWaitDelay is how long Wait waits for the app-server after its
// kill.
const codexWaitDelay = time.Second

// maxCodexLineBytes bounds one line the app-server writes.
const maxCodexLineBytes = 4 << 20

// codexAppServer names Codex threads through `codex app-server`.
type codexAppServer struct {
	// command starts the app-server; tests start a fake.
	command func(ctx context.Context) (*exec.Cmd, error)
}

func newCodexAppServer() *codexAppServer {
	return &codexAppServer{command: func(ctx context.Context) (*exec.Cmd, error) {
		binary, err := exec.LookPath("codex")
		if err != nil {
			return nil, err
		}
		return exec.CommandContext(ctx, binary, "app-server"), nil
	}}
}

// codexRPCRequest is a JSON-RPC request or, without an id, a notification.
type codexRPCRequest struct {
	ID     int    `json:"id,omitempty"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

// codexRPCReply is a line the app-server writes: a reply when it has an
// id, a notification otherwise.
type codexRPCReply struct {
	ID     *json.RawMessage `json:"id"`
	Result json.RawMessage  `json:"result"`
	Error  *struct {
		Code    int64  `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type codexInitializeParams struct {
	ClientInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

type codexSetNameParams struct {
	ThreadID string `json:"threadId"`
	Name     string `json:"name"`
}

// SetName runs the handshake the protocol asks for, initialize and then
// initialized, and then thread/name/set.
func (a *codexAppServer) SetName(ctx context.Context, threadID, name string) error {
	ctx, cancel := context.WithTimeout(ctx, codexAppServerTimeout)
	defer cancel()
	cmd, err := a.command(ctx)
	if err != nil {
		return &RenameError{Tool: ToolCodex, Reason: RenameNoProgram, Cause: err}
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return &RenameError{Tool: ToolCodex, Reason: RenameUnexpected, Cause: err}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return &RenameError{Tool: ToolCodex, Reason: RenameUnexpected, Cause: err}
	}
	// The app-server runs in a process group of its own, so the deadline
	// stops it and every program it started, and Wait gives up on a
	// program that outlives the kill.
	ownCodexTree(cmd)
	cmd.WaitDelay = codexWaitDelay
	if err := cmd.Start(); err != nil {
		return &RenameError{Tool: ToolCodex, Reason: RenameNoProgram, Cause: err}
	}
	// A program the app-server started can keep its output open after
	// it ends, so the deadline also closes the output: a read waiting on
	// it then ends.
	stopClosing := context.AfterFunc(ctx, func() { _ = stdout.Close() })
	talkErr := codexSetName(ctx, stdin, stdout, threadID, name)
	stopClosing()
	_ = stdin.Close()
	// Once its input closes the app-server exits; how it exits says
	// nothing about the name, which its reply already settled.
	_ = cmd.Wait()
	reapCodexTree(cmd)
	return talkErr
}

// codexSetName holds the conversation with a started app-server.
func codexSetName(ctx context.Context, in io.Writer, out io.Reader, threadID, name string) error {
	lines := bufio.NewScanner(out)
	lines.Buffer(make([]byte, 0, 64<<10), maxCodexLineBytes)
	var init codexInitializeParams
	init.ClientInfo.Name, init.ClientInfo.Version = "agnostic-ai", "rename"
	if err := codexSend(in, codexRPCRequest{ID: 1, Method: "initialize", Params: init}); err != nil {
		return err
	}
	if err := codexAwait(ctx, lines, 1, "initialize"); err != nil {
		return err
	}
	if err := codexSend(in, codexRPCRequest{Method: "initialized"}); err != nil {
		return err
	}
	if err := codexSend(in, codexRPCRequest{ID: 2, Method: "thread/name/set", Params: codexSetNameParams{ThreadID: threadID, Name: name}}); err != nil {
		return err
	}
	return codexAwait(ctx, lines, 2, "thread/name/set")
}

func codexSend(in io.Writer, req codexRPCRequest) error {
	line, err := json.Marshal(req)
	if err != nil {
		return &RenameError{Tool: ToolCodex, Reason: RenameUnexpected, Cause: err}
	}
	if _, err := in.Write(append(line, '\n')); err != nil {
		return &RenameError{Tool: ToolCodex, Reason: RenameUnreachable, Cause: fmt.Errorf("write %s: %w", req.Method, err)}
	}
	return nil
}

// codexAwait reads lines until the reply to id, skipping
// notifications. The reply must hold an error or a result that is a
// JSON object, as every reply to method does.
func codexAwait(ctx context.Context, lines *bufio.Scanner, id int, method string) error {
	want, _ := json.Marshal(id)
	for lines.Scan() {
		var reply codexRPCReply
		if err := json.Unmarshal(lines.Bytes(), &reply); err != nil {
			return &RenameError{Tool: ToolCodex, Reason: RenameUnexpected, Cause: fmt.Errorf("app-server line: %w", err)}
		}
		if reply.ID == nil || string(*reply.ID) != string(want) {
			continue
		}
		if reply.Error != nil {
			return &RenameError{Tool: ToolCodex, Reason: RenameRefused, Cause: fmt.Errorf("app-server error %d: %s", reply.Error.Code, reply.Error.Message)}
		}
		var result map[string]json.RawMessage
		if err := json.Unmarshal(reply.Result, &result); err != nil || result == nil {
			return &RenameError{Tool: ToolCodex, Reason: RenameUnexpected, Cause: fmt.Errorf("the app-server's reply to %s holds no result: %s", method, lines.Bytes())}
		}
		return nil
	}
	if ctx.Err() != nil {
		return &RenameError{Tool: ToolCodex, Reason: RenameUnreachable, Cause: ctx.Err()}
	}
	cause := lines.Err()
	if cause == nil {
		cause = errors.New("the app-server ended before it answered")
	}
	return &RenameError{Tool: ToolCodex, Reason: RenameUnreachable, Cause: cause}
}
