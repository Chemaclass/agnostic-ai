package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestTOMLArray_AddRemoveRestoresBytes(t *testing.T) {
	for _, original := range []string{"", "# user\n", "value = 1", "value = 1\r\n", "\xef\xbb\xbfvalue = 1\n"} {
		t.Run(strings.ReplaceAll(original, "\n", "N"), func(t *testing.T) {
			want := map[string]map[string]any{"docs": {"name": "docs", "transport": "stdio", "command": "node", "env": map[string]any{"X": "v"}}}
			add, layout, err := mergeTOMLArray("config.toml", []byte(original), "mcp_servers", want, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			remove, _, err := mergeTOMLArray("config.toml", add.data, "mcp_servers", nil, add.owned, layout)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(remove.data, []byte(original)) {
				t.Errorf("restoration: got %q want %q", remove.data, original)
			}
		})
	}
}
func TestTOMLArray_PreservesInterleavedTablesAndNextComments(t *testing.T) {
	original := "[[mcp_servers]]\nname='first'\ncommand='one'\n\n[other]\nvalue='keep'\n\n[mcp_servers.auth]\ntype='static'\n\n# next user\n[[mcp_servers]]\nname='second'\ncommand='two'\n"
	previous := map[string]any{"first": map[string]any{"name": "first", "command": "one", "auth": map[string]any{"type": "static"}}}
	m, _, err := mergeTOMLArray("config.toml", []byte(original), "mcp_servers", nil, previous, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[other]\nvalue='keep'", "# next user\n[[mcp_servers]]\nname='second'\ncommand='two'"} {
		if !strings.Contains(string(m.data), want) {
			t.Errorf("lost %q: %s", want, m.data)
		}
	}
	if strings.Contains(string(m.data), "name='first'") || strings.Contains(string(m.data), "[mcp_servers.auth]") {
		t.Fatal("owned record remains")
	}
}
func TestTOMLArray_InlineAdoptsWithoutRewriteAndRefusesMutation(t *testing.T) {
	data := []byte(`mcp_servers = [{name="docs", command="node"}] # user`)
	want := map[string]map[string]any{"docs": {"name": "docs", "command": "node"}}
	m, _, err := mergeTOMLArray("config.toml", data, "mcp_servers", want, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.data != nil {
		t.Fatal("adoption rewrote inline text")
	}
	if _, _, err := mergeTOMLArray("config.toml", data, "mcp_servers", nil, m.owned, nil); err == nil {
		t.Fatal("inline removal must refuse")
	}
}
func TestTOMLArray_RejectsHandEditsAndDuplicateNames(t *testing.T) {
	for _, text := range []string{"[[hooks]]\nname='guard'\ncommand='edited'\n", "[[hooks]]\nname='guard'\n[[hooks]]\nname='guard'\n"} {
		_, _, err := mergeTOMLArray("hooks.toml", []byte(text), "hooks", map[string]map[string]any{"guard": {"name": "guard", "command": "new"}}, map[string]any{"guard": map[string]any{"name": "guard", "command": "old"}}, nil)
		if err == nil {
			t.Fatal("conflicting content must fail")
		}
	}
}

func TestTOMLArray_MultilineStringsAndQuotedHeaders(t *testing.T) {
	text := "text = '''\n[[mcp_servers]]\nname='fake'\n'''\n\n[[\"mcp_servers\"]]\nname='real'\ncommand='node'\n[\"mcp_servers\".\"auth\"]\ntype='static'\n\n[\"unrelated.key\"]\nvalue='kept'\n"
	previous := map[string]any{"real": map[string]any{"name": "real", "command": "node", "auth": map[string]any{"type": "static"}}}
	m, _, err := mergeTOMLArray("config.toml", []byte(text), "mcp_servers", nil, previous, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(m.data), "name='fake'") || !strings.Contains(string(m.data), "[\"unrelated.key\"]\nvalue='kept'") {
		t.Fatalf("unrelated content changed: %s", m.data)
	}
}
func TestTOMLArray_RemovalKeepsLaterUserContent(t *testing.T) {
	want := map[string]map[string]any{"guard": {"name": "guard", "type": "pre_tool", "command": "check"}}
	add, layout, err := mergeTOMLArray("hooks.toml", nil, "hooks", want, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	data := append(add.data, []byte("\n# personal\n[ui]\ncolor='blue'\n")...)
	remove, _, err := mergeTOMLArray("hooks.toml", data, "hooks", nil, add.owned, layout)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(remove.data), "# personal\n[ui]\ncolor='blue'") {
		t.Fatalf("lost appended user content: %s", remove.data)
	}
}
func TestTOMLArray_RejectsInvalidDocumentAndWrongArrayType(t *testing.T) {
	for _, text := range []string{"invalid = [", "hooks = 'wrong'", "[[hooks]]\ncommand='missing name'\n"} {
		if _, _, err := mergeTOMLArray("hooks.toml", []byte(text), "hooks", nil, nil, nil); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
}

func TestTOMLArray_UnnamedEntriesUseExternalIdentity(t *testing.T) {
	original := []byte("# user\n[[hooks]]\nevent='before'\ncommand='user'\n")
	want := map[string]map[string]any{"source-guard": {"event": "before", "command": "managed", "timeout": 10}}
	add, layout, err := mergeTOMLArrayRecords("config.toml", original, "hooks", want, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(add.data), "source-guard") || strings.Contains(string(add.data), "name =") {
		t.Fatal("external identity leaked into native file")
	}
	changed := map[string]map[string]any{"source-guard": {"event": "before", "command": "replacement", "timeout": 20}}
	updated, nextLayout, err := mergeTOMLArrayRecords("config.toml", add.data, "hooks", changed, add.owned, layout, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated.data), "command='user'") || strings.Contains(string(updated.data), `command = "managed"`) {
		t.Fatalf("wrong occurrence updated: %s", updated.data)
	}
	removed, _, err := mergeTOMLArrayRecords("config.toml", updated.data, "hooks", nil, updated.owned, nextLayout, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(removed.data, original) {
		t.Fatalf("restoration: %q want %q", removed.data, original)
	}
}
func TestTOMLArray_UnnamedDuplicatesNeverBulkDelete(t *testing.T) {
	entry := map[string]any{"event": "before", "command": "guard", "timeout": int64(10)}
	data := []byte("[[hooks]]\nevent='before'\ncommand='guard'\ntimeout=10\n[[hooks]]\nevent='before'\ncommand='guard'\ntimeout=10\n")
	_, _, err := mergeTOMLArrayRecords("config.toml", data, "hooks", nil, map[string]any{"source-guard": entry}, nil, true)
	if err == nil {
		t.Fatal("duplicate ownership removal must stop")
	}
}

func TestTOMLArray_MultipleEntriesRestoreAfterNeighborUpdates(t *testing.T) {
	for _, original := range []string{"key=1", "key=1\n", "key=1\r\n"} {
		first := map[string]map[string]any{"a": {"name": "a", "command": "run", "env": map[string]any{"MODE": "old"}}, "b": {"name": "b", "command": "other"}}
		added, layout, err := mergeTOMLArray("config.toml", []byte(original), "mcp_servers", first, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		first["a"] = map[string]any{"name": "a", "command": "run", "env": map[string]any{"MODE": "new"}}
		updated, nextLayout, err := mergeTOMLArray("config.toml", added.data, "mcp_servers", first, added.owned, layout)
		if err != nil {
			t.Fatal(err)
		}
		removed, _, err := mergeTOMLArray("config.toml", updated.data, "mcp_servers", nil, updated.owned, nextLayout)
		if err != nil {
			t.Fatal(err)
		}
		if string(removed.data) != original {
			t.Errorf("neighbor update restoration: got %q want %q", removed.data, original)
		}
	}
}
