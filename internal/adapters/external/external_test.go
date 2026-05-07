package external

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// TestMain doubles as a fake adapter binary when invoked under the
// AGNOSTIC_AI_ADAPTER_HELPER env var. Tests below re-exec the test
// binary with that env set to exercise the protocol end to end without
// shipping a separate fixture binary.
func TestMain(m *testing.M) {
	switch os.Getenv("AGNOSTIC_AI_ADAPTER_HELPER") {
	case "echo":
		runHelperEcho()
		return
	case "fail":
		runHelperFail()
		return
	case "warn":
		runHelperWarn()
		return
	case "bad-json":
		_, _ = os.Stdout.WriteString("not json")
		return
	}
	os.Exit(m.Run())
}

func runHelperEcho() {
	in, err := DecodeInput(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out := Output{
		Files: []File{{
			Path:    "FAKE.md",
			Content: fmt.Sprintf("target=%s rules=%d agents=%d\n", in.Target, len(in.Specs.Rules), len(in.Specs.Agents)),
		}},
	}
	if err := EncodeOutput(os.Stdout, out); err != nil {
		os.Exit(1)
	}
}

func runHelperFail() {
	out := Output{ProtocolVersion: ProtocolVersion, Errors: []string{"adapter said no"}}
	_ = EncodeOutput(os.Stdout, out)
}

func runHelperWarn() {
	out := Output{
		ProtocolVersion: ProtocolVersion,
		Warnings:        []string{"deprecated config field"},
		Files:           []File{{Path: "OK.md", Content: "ok"}},
	}
	_ = EncodeOutput(os.Stdout, out)
}

func helperCommand(mode string) func() *exec.Cmd {
	return func() *exec.Cmd {
		c := exec.Command(os.Args[0], "-test.run=TestMain")
		c.Env = append(os.Environ(), "AGNOSTIC_AI_ADAPTER_HELPER="+mode)
		return c
	}
}

func TestAdapter_Emit_RoundTrips(t *testing.T) {
	adapter := NewWithCommand("fake", helperCommand("echo"))

	bundle := spec.Bundle{
		Rules: []spec.Entry{
			{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
		},
	}
	cfg := &config.Config{
		Sources:       config.Sources{Rules: "rules"},
		OnUnsupported: "warn",
	}

	emit.StartCapture()
	if err := adapter.Emit(bundle, cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	files := emit.StopCapture()
	if len(files) != 1 {
		t.Fatalf("expected 1 captured file, got %d", len(files))
	}
	if files[0].Path != "FAKE.md" {
		t.Errorf("path=%q, want FAKE.md", files[0].Path)
	}
	if !strings.Contains(files[0].Content, "target=fake rules=1") {
		t.Errorf("content=%q", files[0].Content)
	}
}

func TestAdapter_Emit_AdapterErrorBubbles(t *testing.T) {
	adapter := NewWithCommand("fake", helperCommand("fail"))
	err := adapter.Emit(spec.Bundle{}, &config.Config{}, false)
	if err == nil || !strings.Contains(err.Error(), "adapter said no") {
		t.Fatalf("err=%v, want contains 'adapter said no'", err)
	}
}

func TestAdapter_Emit_WarningsRoutedToWarner(t *testing.T) {
	var buf bytes.Buffer
	old := emit.Warner
	emit.Warner = &buf
	t.Cleanup(func() { emit.Warner = old })

	adapter := NewWithCommand("fake", helperCommand("warn"))
	emit.StartCapture()
	if err := adapter.Emit(spec.Bundle{}, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	_ = emit.StopCapture()

	if !strings.Contains(buf.String(), "deprecated config field") {
		t.Errorf("warner=%q, want warning text", buf.String())
	}
}

func TestAdapter_Emit_RejectsBadJSON(t *testing.T) {
	adapter := NewWithCommand("fake", helperCommand("bad-json"))
	err := adapter.Emit(spec.Bundle{}, &config.Config{}, false)
	if err == nil || !strings.Contains(err.Error(), "decode output") {
		t.Fatalf("err=%v, want decode error", err)
	}
}

func TestNew_ReturnsErrNotFound_OnMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := New("nope-no-such-adapter-12345")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("err=%v, want exec.ErrNotFound", err)
	}
}

func TestValidateName(t *testing.T) {
	bad := []string{"", "foo/bar", `foo\bar`, "-foo"}
	for _, n := range bad {
		if err := validateName(n); err == nil {
			t.Errorf("validateName(%q) accepted, expected error", n)
		}
	}
	good := []string{"foo", "my-tool", "tool_v1", "tool.v2"}
	for _, n := range good {
		if err := validateName(n); err != nil {
			t.Errorf("validateName(%q) unexpected error: %v", n, err)
		}
	}
}

func TestBuildInput_PreservesEntryFields(t *testing.T) {
	b := spec.Bundle{
		Agents: []spec.Entry{{
			Kind:  spec.KindAgent,
			Name:  "a",
			Path:  "agents/a.md",
			Scope: "backend",
			Layer: "project",
			Meta:  map[string]any{"description": "d"},
			Body:  "body",
		}},
	}
	in := buildInput("fake", b, &config.Config{Sources: config.Sources{Agents: "agents"}}, false)
	if len(in.Specs.Agents) != 1 {
		t.Fatalf("agents=%d, want 1", len(in.Specs.Agents))
	}
	got := in.Specs.Agents[0]
	if got.Kind != "agent" || got.Name != "a" || got.Scope != "backend" || got.Layer != "project" {
		t.Errorf("entry=%+v", got)
	}
	if got.Meta["description"] != "d" || got.Body != "body" {
		t.Errorf("meta/body lost: %+v", got)
	}
}

func TestEncodeOutput_DefaultsProtocolVersion(t *testing.T) {
	var buf bytes.Buffer
	if err := EncodeOutput(&buf, Output{Files: []File{{Path: "x"}}}); err != nil {
		t.Fatal(err)
	}
	var got Output
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ProtocolVersion != ProtocolVersion {
		t.Errorf("ProtocolVersion=%d, want %d", got.ProtocolVersion, ProtocolVersion)
	}
}
