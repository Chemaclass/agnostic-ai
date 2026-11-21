package emit

import (
	"syscall/js"
	"testing"
)

func TestMergeTOMLFileBrowserCapturesWithoutFilesystem(t *testing.T) {
	fs := js.Global().Get("fs")
	open := fs.Get("open")
	unavailable := js.FuncOf(func(_ js.Value, args []js.Value) any {
		err := js.Global().Get("Error").New("not implemented on js")
		err.Set("code", "ENOSYS")
		args[len(args)-1].Invoke(err)
		return nil
	})
	fs.Set("open", unavailable)
	defer func() {
		fs.Set("open", open)
		unavailable.Release()
	}()

	sess := NewSession()
	sess.StartCapture()
	const path = ".grok/config.toml"
	if err := sess.MergeTOMLFile(path, []TOMLValue{{Path: []string{"mcp_servers", "docs", "command"}, Value: "docs-server"}}, false); err != nil {
		t.Fatal(err)
	}
	files := sess.StopCapture()
	if len(files) != 1 || files[0].Path != path || !files[0].Merged || len(files[0].Keys) != 1 {
		t.Fatalf("expected one claimed TOML capture, got %+v", files)
	}
	doc, err := parseTOMLMap(path, []byte(files[0].Content))
	if err != nil {
		t.Fatal(err)
	}
	if command, _ := tomlMapValue(doc, []string{"mcp_servers", "docs", "command"}); command != "docs-server" {
		t.Errorf("captured MCP command = %v", command)
	}
}
