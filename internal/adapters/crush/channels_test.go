package crush

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func channelReply() map[string]any {
	return map[string]any{
		"user":          map[string]any{"tool": "send_user", "target_param": "user_id", "target_meta": "sender_id"},
		"group":         map[string]any{"tool": "send_group", "target_param": "group_id", "target_meta": "room"},
		"message_param": "text", "suppress_tools": []any{"send", "reply"},
	}
}

func TestEmit_MCP_ChannelFields(t *testing.T) {
	for _, tc := range []struct {
		name    string
		meta    map[string]any
		enabled bool
	}{
		{"top-level", map[string]any{"channel_enabled": true, "channel_reply": channelReply()}, true},
		{"explicit-false", map[string]any{"channel_enabled": false, "channel_reply": channelReply()}, false},
		{"target-override", map[string]any{"channel_enabled": true, "channel_reply": "overridden", "x-crush": map[string]any{"channel_enabled": false, "channel_reply": channelReply(), "unknown": "ignored"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.TempCwd(t)
			tc.meta["command"] = "channel-server"
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindMCP, Name: "chat", Meta: tc.meta}})
			if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile("crush.json")
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]map[string]map[string]any
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			server := got["mcp"]["chat"]
			if actual, ok := server["channel_enabled"].(bool); !ok || actual != tc.enabled {
				t.Errorf("channel_enabled = %#v, want %v", server["channel_enabled"], tc.enabled)
			}
			want, _ := json.Marshal(channelReply())
			actual, _ := json.Marshal(server["channel_reply"])
			if !reflect.DeepEqual(actual, want) {
				t.Errorf("channel_reply = %s, want %s", actual, want)
			}
			if _, ok := server["unknown"]; ok {
				t.Error("unknown extension field emitted")
			}
		})
	}
}

func TestEmit_MCP_InvalidChannelFields(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		value       any
	}{
		{"boolean", "channel_enabled", "true"},
		{"reply-object", "channel_reply", "reply"},
		{"unknown-reply-key", "channel_reply", map[string]any{"typo": true}},
		{"route-object", "channel_reply", map[string]any{"user": "send"}},
		{"missing-tool", "channel_reply", map[string]any{"user": map[string]any{"target_param": "id"}}},
		{"missing-target", "channel_reply", map[string]any{"group": map[string]any{"tool": "send"}}},
		{"route-string", "channel_reply", map[string]any{"user": map[string]any{"tool": true, "target_param": "id"}}},
		{"unknown-route-key", "channel_reply", map[string]any{"user": map[string]any{"tool": "send", "target_param": "id", "typo": "x"}}},
		{"target-meta-string", "channel_reply", map[string]any{"user": map[string]any{"tool": "send", "target_param": "id", "target_meta": false}}},
		{"suppress-type", "channel_reply", map[string]any{"suppress_tools": "send"}},
		{"message-string", "channel_reply", map[string]any{"message_param": 1}},
		{"suppress-list", "channel_reply", map[string]any{"suppress_tools": []any{"send", false}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.TempCwd(t)
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindMCP, Name: "chat", Path: "mcps/chat.yaml", Meta: map[string]any{"command": "channel-server", tc.field: tc.value}}})
			err := New().Emit(emit.NewSession(), b, &config.Config{}, false)
			if err == nil {
				t.Fatal("invalid channel field accepted")
			}
			for _, text := range []string{"mcps/chat.yaml", tc.field} {
				if !strings.Contains(err.Error(), text) {
					t.Errorf("error %q missing %q", err, text)
				}
			}
			if _, err := os.Stat("crush.json"); !os.IsNotExist(err) {
				t.Errorf("invalid channel config wrote crush.json: %v", err)
			}
		})
	}
}

func TestEmit_MCP_ChannelDefaultsStayOmitted(t *testing.T) {
	testutil.TempCwd(t)
	reply := map[string]any{"user": map[string]any{"tool": "send_user", "target_param": "user_id"}, "suppress_tools": []any{}}
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindMCP, Name: "chat", Meta: map[string]any{"command": "channel-server", "channel_reply": reply}}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("crush.json")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]map[string]map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	server := got["mcp"]["chat"]
	if _, ok := server["channel_enabled"]; ok {
		t.Error("channel_enabled default emitted")
	}
	want, _ := json.Marshal(reply)
	actual, _ := json.Marshal(server["channel_reply"])
	if !reflect.DeepEqual(actual, want) {
		t.Errorf("channel defaults changed: got %s, want %s", actual, want)
	}
}
