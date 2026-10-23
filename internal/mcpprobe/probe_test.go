package mcpprobe

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A test binary started with MCPPROBE_FAKE_SERVER=1 acts as a stdio MCP
// server: it answers initialize and two pages of tools/list, with a log
// notification and a stray line in between.
func TestMain(m *testing.M) {
	if os.Getenv("MCPPROBE_FAKE_SERVER") == "hold-output" {
		time.Sleep(5 * time.Second)
		return
	}
	if os.Getenv("MCPPROBE_FAKE_SERVER") == "descendant" {
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), "MCPPROBE_FAKE_SERVER=hold-output")
		child.Stdout = os.Stdout
		if child.Start() != nil {
			os.Exit(2)
		}
		if path := os.Getenv("MCPPROBE_CHILD_PID"); path != "" {
			if os.WriteFile(path, []byte(strconv.Itoa(child.Process.Pid)), 0o600) != nil {
				_ = child.Process.Kill()
				os.Exit(2)
			}
		}
		_ = child.Wait()
		return
	}
	if os.Getenv("MCPPROBE_FAKE_SERVER") == "1" {
		fakeStdioServer()
		return
	}
	os.Exit(m.Run())
}

func TestListTools_StdioCancellationClosesDescendantHeldOutput(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c, cancel := context.WithCancel(context.Background())
	defer cancel()
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	done := make(chan error, 1)
	go func() {
		_, err := ListTools(c, Server{Command: exe, Env: map[string]string{"MCPPROBE_FAKE_SERVER": "descendant", "MCPPROBE_CHILD_PID": pidFile}}, "test")
		done <- err
	}()
	var pid int
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(pidFile); err == nil {
			pid, _ = strconv.Atoi(string(data))
			if pid != 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("fake server did not record its child PID")
	}
	child, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Kill(); _ = child.Release() }()
	start := time.Now()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled listing did not return")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("cancellation took %s with a descendant holding stdout", elapsed)
	}
	deadline = time.Now().Add(time.Second)
	for processRunning(t, pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processRunning(t, pid) {
		t.Errorf("owned descendant %d is still running after cancellation", pid)
	}
}

func fakeStdioServer() {
	in := bufio.NewScanner(os.Stdin)
	out := json.NewEncoder(os.Stdout)
	for in.Scan() {
		var req struct {
			ID     *int           `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if json.Unmarshal(in.Bytes(), &req) != nil || req.ID == nil {
			continue
		}
		fmt.Println("not json: a server log line")
		_ = out.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/message", "params": map[string]any{"level": "info"}})
		switch req.Method {
		case "initialize":
			_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": map[string]any{"protocolVersion": ProtocolVersion, "serverInfo": map[string]any{"name": "fake"}}})
		case "tools/list":
			if req.Params["cursor"] == "page2" {
				_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": map[string]any{"tools": []any{map[string]any{"name": "beta"}}}})
			} else {
				_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": map[string]any{"tools": []any{map[string]any{"name": "alpha", "description": "First " + os.Getenv("FAKE_GREETING")}}, "nextCursor": "page2"}})
			}
		}
	}
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestListTools_StdioPagesThroughToolsAndSkipsNoise(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tools, err := ListTools(ctx(t), Server{Command: exe, Env: map[string]string{"MCPPROBE_FAKE_SERVER": "1", "FAKE_GREETING": "hello"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 || tools[0].Name != "alpha" || tools[0].Description != "First hello" || tools[1].Name != "beta" {
		t.Errorf("tools = %+v, want alpha (with the env the spec sets) then beta", tools)
	}
}

func TestListTools_StdioReportsACommandThatCannotStart(t *testing.T) {
	_, err := ListTools(ctx(t), Server{Command: "agnostic-ai-no-such-mcp-server"}, "test")
	if err == nil || !strings.Contains(err.Error(), "start agnostic-ai-no-such-mcp-server") {
		t.Errorf("error = %v, want a start failure", err)
	}
}

// fakeHTTPServer answers streamable HTTP: JSON for initialize, an event
// stream for tools/list, and a session id it then requires.
func fakeHTTPServer(t *testing.T, wantAuth string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != wantAuth {
			if r.Header.Get("Authorization") != "" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="fake", error="invalid_token", error_description="the token expired"`)
			}
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method != "initialize" && r.Header.Get("Mcp-Session-Id") != "s1" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch {
		case req.ID == nil:
			w.WriteHeader(http.StatusAccepted)
		case req.Method == "initialize":
			w.Header().Set("Mcp-Session-Id", "s1")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": map[string]any{"protocolVersion": ProtocolVersion}})
		case req.Method == "tools/list":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\"}\n\n")
			_, _ = fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":%d,\n", *req.ID)
			_, _ = fmt.Fprint(w, "data: \"result\":{\"tools\":[{\"name\":\"send_email\",\"title\":\"Send email\"}]}}\n\n")
		}
	}))
}

func TestListTools_HTTPKeepsTheSessionAndReadsAnEventStream(t *testing.T) {
	srv := fakeHTTPServer(t, "Bearer good")
	defer srv.Close()
	tools, err := ListTools(ctx(t), Server{Type: "http", URL: srv.URL, Headers: map[string]string{"Authorization": "Bearer good"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Name != "send_email" || tools[0].Title != "Send email" {
		t.Errorf("tools = %+v", tools)
	}
}

func TestListTools_HTTPReportsMissingAuthentication(t *testing.T) {
	srv := fakeHTTPServer(t, "Bearer good")
	defer srv.Close()
	_, err := ListTools(ctx(t), Server{Type: "http", URL: srv.URL}, "test")
	if err == nil || !strings.Contains(err.Error(), "needs authentication") {
		t.Errorf("error = %v, want an authentication failure", err)
	}
}

func TestListTools_HTTPNamesTheReasonTheServerRefusedTheCredentials(t *testing.T) {
	srv := fakeHTTPServer(t, "Bearer good")
	defer srv.Close()
	_, err := ListTools(ctx(t), Server{Type: "http", URL: srv.URL, Headers: map[string]string{"Authorization": "Bearer old"}}, "test")
	if err == nil || !strings.Contains(err.Error(), "refused the credentials: the token expired") {
		t.Errorf("error = %v, want the server's own reason", err)
	}
}

func TestListTools_RefusesTransportsItDoesNotSpeak(t *testing.T) {
	for _, typ := range []string{"sse", "ws"} {
		if _, err := ListTools(ctx(t), Server{Type: typ, URL: "http://127.0.0.1:1"}, "test"); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s error = %v, want ErrUnsupported", typ, err)
		}
	}
}
