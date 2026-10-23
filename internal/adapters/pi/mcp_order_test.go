package pi

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_ToolExposureKeepsFirstMatchingWildcard(t *testing.T) {
	testutil.TempCwd(t)
	entry, err := spec.ParseYAMLBytes(spec.KindMCP, []byte("name: tools\ncommand: tools-mcp\nx-pi:\n  toolExposure:\n    write_*: hidden\n    '*': direct\n    write_safe: deferred\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".pi/mcp.json")
	if err != nil {
		t.Fatal(err)
	}
	tools := orderedField(t, data, "mcpServers", "tools", "toolExposure")
	if got := nativeExposure(t, tools, "write_file"); got != "hidden" {
		t.Errorf("write tool exposed as %q, want hidden; %s", got, data)
	}
	if got := nativeExposure(t, tools, "write_safe"); got != "deferred" {
		t.Errorf("exact tool exposure=%q, want deferred", got)
	}
}

func orderedField(t *testing.T, data []byte, keys ...string) *emit.OrderedJSON {
	t.Helper()
	object := emit.NewOrderedJSON()
	for _, key := range keys {
		if err := json.Unmarshal(data, object); err != nil {
			t.Fatal(err)
		}
		raw, ok := object.Get(key)
		if !ok {
			t.Fatalf("missing %s in %s", key, data)
		}
		data = raw
		object = emit.NewOrderedJSON()
	}
	if err := json.Unmarshal(data, object); err != nil {
		t.Fatal(err)
	}
	return object
}

func nativeExposure(t *testing.T, tools *emit.OrderedJSON, name string) string {
	t.Helper()
	decode := func(raw json.RawMessage) string {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	if raw, ok := tools.Get(name); ok {
		return decode(raw)
	}
	for _, pattern := range tools.Keys() {
		parts := strings.Split(pattern, "*")
		for i := range parts {
			parts[i] = regexp.QuoteMeta(parts[i])
		}
		if strings.Contains(pattern, "*") && regexp.MustCompile("^"+strings.Join(parts, ".*")+"$").MatchString(name) {
			raw, _ := tools.Get(pattern)
			return decode(raw)
		}
	}
	return ""
}

func TestEmit_ToolExposureUsesWinningMapOrder(t *testing.T) {
	testutil.TempCwd(t)
	entry, err := spec.ParseYAMLBytes(spec.KindMCP, []byte("name: tools\ncommand: tools-mcp\ntoolExposure:\n  write_*: hidden\n  '*': direct\nx-pi:\n  toolExposure:\n    '*': direct\n    write_*: hidden\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".pi/mcp.json")
	if err != nil {
		t.Fatal(err)
	}
	tools := orderedField(t, data, "mcpServers", "tools", "toolExposure")
	if got := nativeExposure(t, tools, "write_file"); got != "direct" {
		t.Errorf("winning native override ignored: %s", data)
	}
	delete(entry.NestedKeys, "x-pi\x00toolExposure")
	if _, err := mcpServer(entry); err == nil {
		t.Error("borrowed losing map order for unhinted native override")
	}
	entry.Meta["x-pi"] = map[string]any{"toolExposure": nil}
	server, err := mcpServer(entry)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := server["toolExposure"]; exists {
		t.Errorf("null override kept losing map: %v", server)
	}
}

func TestEmit_ToolExposureRejectsUnprovenOrderAndInvalidValues(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindMCP, Name: "tools", Meta: map[string]any{"command": "tools-mcp", "toolExposure": map[string]any{"write_*": "hidden", "*": "direct"}}}
	if _, err := mcpServer(entry); err == nil || !strings.Contains(err.Error(), "source key order") {
		t.Errorf("missing order accepted: %v", err)
	}
	entry.NestedKeys = map[string][]string{"toolExposure": {"write_*"}}
	if _, err := mcpServer(entry); err == nil {
		t.Error("incomplete order accepted")
	}
	entry.Meta["toolExposure"] = map[string]any{"write_*": "invalid"}
	if _, err := mcpServer(entry); err == nil {
		t.Error("invalid exposure accepted")
	}
	entry.Meta["toolExposure"] = map[string]any{"write_*": "hidden", "write?": "direct", "[write]": "deferred"}
	entry.NestedKeys = nil
	if _, err := mcpServer(entry); err != nil {
		t.Errorf("unambiguous wildcard and literal metacharacters rejected: %v", err)
	}
}
