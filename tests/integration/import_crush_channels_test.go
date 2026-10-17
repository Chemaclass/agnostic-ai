package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestImportCrush_ChannelRoundTrip(t *testing.T) {
	packageDir, err := os.Getwd()
	must(t, err)
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Join(packageDir, "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	dir := t.TempDir()
	write := func(path, content string) { t.Helper(); must(t, os.WriteFile(path, []byte(content), 0o644)) }
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	write(filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [crush]\n")
	native := `{"mcp":{"chat":{"type":"stdio","command":"channel-server","channel_enabled":true,"channel_reply":{"user":{"tool":"send_user","target_param":"user_id","target_meta":"sender_id"},"group":{"tool":"send_group","target_param":"group_id","target_meta":"room"},"message_param":"text","suppress_tools":["send","reply"]}},"off":{"type":"http","url":"https://example.test/mcp","channel_enabled":false}}}`
	write(filepath.Join(dir, "crush.json"), native)
	run("import", "crush")
	run("sync", "-t", "crush", "--gitignore", "off")
	var want, got any
	if err := json.Unmarshal([]byte(native), &want); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "crush.json"))
	must(t, err)
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("channel server changed after import/sync:\ngot %#v\nwant %#v", got, want)
	}
	run("sync", "-t", "crush", "--check")
	run("lint")
}
