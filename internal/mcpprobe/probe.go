// Package mcpprobe asks an MCP server which tools it offers: it runs the
// initialize handshake, then tools/list, over stdio or streamable HTTP.
// It is the only code in agnostic-ai that connects to an MCP server, and
// it runs only when a person asks for it (agnostic-ai mcp tools).
package mcpprobe

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/preview"
)

// ProtocolVersion is the MCP revision the probe announces.
const ProtocolVersion = "2025-06-18"

// maxPages bounds tools/list pagination, so a server that keeps
// returning a cursor cannot hold the probe.
const maxPages = 20

// Server is one MCP server to ask, with every reference already resolved.
type Server struct {
	// Type is "stdio" (the default), "http", "sse", or "ws".
	Type            string
	Command         string
	Args            []string
	Env             map[string]string
	Cwd             string
	URL             string
	Headers         map[string]string
	SensitiveValues []string
}

// Tool is one tool a server lists.
type Tool struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// ErrUnsupported reports a transport the probe does not speak.
var ErrUnsupported = errors.New("transport not supported")

// ListTools asks s for its tools. The context bounds the whole exchange.
func ListTools(ctx context.Context, s Server, client string) (tools []Tool, err error) {
	defer func() {
		values := append([]string(nil), s.SensitiveValues...)
		for _, fields := range []map[string]string{s.Env, s.Headers} {
			for key, value := range fields {
				if !preview.CredentialKey(key) {
					continue
				}
				values = append(values, value)
				if strings.EqualFold(key, "Authorization") {
					if _, token, ok := strings.Cut(value, " "); ok {
						values = append(values, token)
					}
				}
			}
		}
		if address, parseErr := url.Parse(s.URL); parseErr == nil {
			if address.User != nil {
				password, _ := address.User.Password()
				values = append(values, address.User.Username(), password)
			}
			for key, items := range address.Query() {
				if preview.CredentialParam(key) {
					values = append(values, items...)
				}
			}
		}
		if err != nil {
			err = &probeError{cause: err, text: preview.RedactText(err.Error(), values...)}
		}
		for i := range tools {
			tools[i].Name = preview.RedactText(tools[i].Name, values...)
			tools[i].Title = preview.RedactText(tools[i].Title, values...)
			tools[i].Description = preview.RedactText(tools[i].Description, values...)
		}
	}()
	switch s.Type {
	case "", "stdio":
		if s.Command == "" {
			return nil, errors.New("no command")
		}
		return listStdio(ctx, s, client)
	case "http":
		if s.URL == "" {
			return nil, errors.New("no url")
		}
		return listHTTP(ctx, s, client)
	}
	return nil, fmt.Errorf("%w: %s", ErrUnsupported, s.Type)
}

type probeError struct {
	cause error
	text  string
}

func (e *probeError) Error() string { return e.text }
func (e *probeError) Unwrap() error { return e.cause }

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	ID     *json.RawMessage `json:"id"`
	Result json.RawMessage  `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type listResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor"`
}

func initializeParams(client string) map[string]any {
	return map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "agnostic-ai", "version": client},
	}
}

// exchange is one transport: call sends a request and returns its
// result; notify sends a notification.
type exchange interface {
	call(ctx context.Context, id int, method string, params any) (json.RawMessage, error)
	notify(ctx context.Context, method string) error
}

// handshake runs initialize, then pages through tools/list.
func handshake(ctx context.Context, x exchange, client string) ([]Tool, error) {
	if _, err := x.call(ctx, 1, "initialize", initializeParams(client)); err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}
	if err := x.notify(ctx, "notifications/initialized"); err != nil {
		return nil, fmt.Errorf("initialized: %w", err)
	}
	var tools []Tool
	cursor := ""
	for page := 0; page < maxPages; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := x.call(ctx, 2+page, "tools/list", params)
		if err != nil {
			return nil, fmt.Errorf("tools/list: %w", err)
		}
		var res listResult
		if err := json.Unmarshal(raw, &res); err != nil {
			return nil, fmt.Errorf("tools/list: %w", err)
		}
		tools = append(tools, res.Tools...)
		if res.NextCursor == "" {
			return tools, nil
		}
		cursor = res.NextCursor
	}
	return nil, errors.New("tools/list exceeded the pagination limit")
}

func resultOf(resp rpcResponse) (json.RawMessage, error) {
	if resp.Error != nil {
		return nil, fmt.Errorf("server error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

func idIs(raw *json.RawMessage, id int) bool {
	if raw == nil {
		return false
	}
	var n int
	return json.Unmarshal(*raw, &n) == nil && n == id
}

// stdio ------------------------------------------------------------------

type stdioExchange struct {
	in  io.Writer
	out *bufio.Scanner
}

func listStdio(ctx context.Context, s Server, client string) (tools []Tool, err error) {
	cmd := exec.CommandContext(ctx, s.Command, s.Args...)
	cmd.Dir = s.Cwd
	cmd.Env = os.Environ()
	for k, v := range s.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open MCP input pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("open MCP output pipe: %w", err)
	}
	cleanup, err := startProcess(ctx, cmd)
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("start %s: %w", s.Command, err)
	}
	stop := context.AfterFunc(ctx, func() {
		_ = stdin.Close()
		_ = stdout.Close()
	})
	defer func() {
		stop()
		_ = stdin.Close()
		_ = stdout.Close()
		err = errors.Join(err, cleanup())
		_ = cmd.Wait()
	}()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 16<<20)
	return handshake(ctx, &stdioExchange{in: stdin, out: scanner}, client)
}

func (x *stdioExchange) send(msg rpcRequest) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = x.in.Write(append(data, '\n'))
	return err
}

func (x *stdioExchange) call(ctx context.Context, id int, method string, params any) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := x.send(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	// Lines that are not the reply, such as log notifications or a
	// server's own requests, are skipped.
	for x.out.Scan() {
		var resp rpcResponse
		if json.Unmarshal(x.out.Bytes(), &resp) != nil || !idIs(resp.ID, id) {
			continue
		}
		return resultOf(resp)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := x.out.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("the server closed its output before replying")
}

func (x *stdioExchange) notify(ctx context.Context, method string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return x.send(rpcRequest{JSONRPC: "2.0", Method: method})
}

// streamable HTTP -------------------------------------------------------

type httpExchange struct {
	client  *http.Client
	url     string
	headers map[string]string
	session string
}

func listHTTP(ctx context.Context, s Server, client string) ([]Tool, error) {
	return handshake(ctx, &httpExchange{client: &http.Client{}, url: s.URL, headers: s.Headers}, client)
}

func (x *httpExchange) post(ctx context.Context, msg rpcRequest) (*http.Response, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, x.url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	for k, v := range x.headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if msg.Method != "initialize" {
		req.Header.Set("MCP-Protocol-Version", ProtocolVersion)
	}
	if x.session != "" {
		req.Header.Set("Mcp-Session-Id", x.session)
	}
	resp, err := x.client.Do(req)
	if err != nil {
		return nil, err
	}
	if id := resp.Header.Get("Mcp-Session-Id"); id != "" {
		x.session = id
	}
	return resp, nil
}

func (x *httpExchange) call(ctx context.Context, id int, method string, params any) (json.RawMessage, error) {
	resp, err := x.post(ctx, rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		if reason := authReason(resp.Header.Get("WWW-Authenticate")); reason != "" {
			return nil, fmt.Errorf("HTTP %d: the server refused the credentials: %s", resp.StatusCode, reason)
		}
		return nil, fmt.Errorf("HTTP %d: the server needs authentication", resp.StatusCode)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body := io.LimitReader(resp.Body, 16<<20)
	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mediaType == "text/event-stream" {
		return replyFromStream(body, id)
	}
	var reply rpcResponse
	if err := json.NewDecoder(body).Decode(&reply); err != nil {
		return nil, fmt.Errorf("decode reply: %w", err)
	}
	return resultOf(reply)
}

// authReason is the error_description, else the error, of a
// WWW-Authenticate challenge.
func authReason(challenge string) string {
	for _, key := range []string{"error_description", "error"} {
		if _, rest, ok := strings.Cut(challenge, key+`="`); ok {
			if value, _, ok := strings.Cut(rest, `"`); ok && value != "" {
				return value
			}
		}
	}
	return ""
}

// replyFromStream reads server-sent events until the reply to id.
func replyFromStream(r io.Reader, id int) (json.RawMessage, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16<<20)
	var data strings.Builder
	flush := func() (json.RawMessage, bool, error) {
		defer data.Reset()
		if data.Len() == 0 {
			return nil, false, nil
		}
		var resp rpcResponse
		if json.Unmarshal([]byte(data.String()), &resp) != nil || !idIs(resp.ID, id) {
			return nil, false, nil
		}
		res, err := resultOf(resp)
		return res, true, err
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if res, done, err := flush(); done {
				return res, err
			}
			continue
		}
		if rest, ok := strings.CutPrefix(line, "data:"); ok {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(rest, " "))
		}
	}
	if res, done, err := flush(); done {
		return res, err
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("the event stream ended before the reply")
}

func (x *httpExchange) notify(ctx context.Context, method string) error {
	resp, err := x.post(ctx, rpcRequest{JSONRPC: "2.0", Method: method})
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}
