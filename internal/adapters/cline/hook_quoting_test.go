package cline_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/cline"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmitWithProvenance_ClineToolOverridesStayLiteral(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("Cline hooks require bash")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{
		"apostrophe's tool",
		"double\"quote",
		"html<>&tool",
		"line\nbreak",
		`back\slash`,
		`$(touch substituted-marker)`,
		"`touch backtick-marker`",
		`literal*?[glob]`,
		`'* ) touch injected-marker ;; esac; #`,
	} {
		t.Run(tool, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			entry := spec.Entry{Kind: spec.KindHook, Name: "literal-tool", Meta: map[string]any{
				"on": "before-tool", "match": "shell", "command": "printf hook-ran",
				"x-cline": map[string]any{"matcher": tool},
			}}
			if problem := spec.PortableHookProblem(entry.Meta); problem != "" {
				t.Fatalf("portable spec must be valid: %s", problem)
			}
			if err := adapters.EmitWithProvenance(adapters.NewSession(), cline.New(), spec.NewBundle([]spec.Entry{entry}), &config.Config{Targets: []string{"cline"}}, false); err != nil {
				t.Fatal(err)
			}
			script := filepath.Join(dir, ".clinerules", "hooks", "PreToolUse")
			if _, err := os.ReadFile(script); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(tool, "injected-marker") {
				testutil.AssertGoldenTree(t, filepath.Dir(script), filepath.Join(root, "testdata", "hook-quoting"))
			}
			for _, shape := range []string{"sdk", "extension"} {
				for _, payloadTool := range []string{tool, "unrelated-tool", strings.ReplaceAll(tool, "*?[glob]", "other")} {
					var input bytes.Buffer
					encoder := json.NewEncoder(&input)
					encoder.SetEscapeHTML(false)
					payload := map[string]any{"preToolUse": map[string]string{"toolName": payloadTool}}
					if shape == "sdk" {
						payload["hookName"] = "tool_call"
						payload["tool_call"] = map[string]string{"name": payloadTool}
					} else {
						payload["hookName"] = "PreToolUse"
						payload["clineVersion"] = ""
					}
					if err := encoder.Encode(payload); err != nil {
						t.Fatal(err)
					}
					cmd := exec.Command("bash", script)
					cmd.Stdin = &input
					out, err := cmd.CombinedOutput()
					if err != nil {
						t.Errorf("tool %q: emitted script failed: %v: %s", payloadTool, err, out)
					}
					want := ""
					if payloadTool == tool {
						want = "hook-ran"
					}
					if string(out) != want {
						t.Errorf("tool %q: output %q, want %q", payloadTool, out, want)
					}
				}
			}
			for _, marker := range []string{"injected-marker", "substituted-marker", "backtick-marker"} {
				if _, err := os.Stat(filepath.Join(dir, marker)); !os.IsNotExist(err) {
					t.Errorf("tool data executed an extra command: %s (stat: %v)", marker, err)
				}
			}
		})
	}
}

func TestEmitWithProvenance_ClineMatchesVendorUnicodeSerialization(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("Cline hooks require bash")
	}
	for _, tc := range []struct {
		tool    string
		payload string
	}{
		{"line\u2028separator", "{\"preToolUse\":{\"toolName\":\"line\u2028separator\"}}"},
		{"paragraph\u2029separator", "{\"preToolUse\":{\"toolName\":\"paragraph\u2029separator\"}}"},
		{`literal\u2028text`, `{"preToolUse":{"toolName":"literal\\u2028text"}}`},
		{`literal\u2029text`, `{"preToolUse":{"toolName":"literal\\u2029text"}}`},
		{"mixed\\\u2028separator", "{\"preToolUse\":{\"toolName\":\"mixed\\\\\u2028separator\"}}"},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			entry := spec.Entry{Kind: spec.KindHook, Name: "unicode-tool", Meta: map[string]any{
				"on": "before-tool", "match": "shell", "command": "printf hook-ran",
				"x-cline": map[string]any{"matcher": tc.tool},
			}}
			if err := adapters.EmitWithProvenance(adapters.NewSession(), cline.New(), spec.NewBundle([]spec.Entry{entry}), &config.Config{Targets: []string{"cline"}}, false); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", filepath.Join(dir, ".clinerules", "hooks", "PreToolUse"))
			cmd.Stdin = strings.NewReader(tc.payload)
			out, err := cmd.CombinedOutput()
			if err != nil || string(out) != "hook-ran" {
				t.Errorf("literal vendor payload: output %q, err %v", out, err)
			}
		})
	}
}
