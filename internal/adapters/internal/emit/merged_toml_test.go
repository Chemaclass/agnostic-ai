package emit

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestMergeTOMLFile_CapturesMergedValuesAndKeepsNativeFile(t *testing.T) {
	testutil.TempCwd(t)
	const before = "[plugins]\nenabled = ['manual']\n"
	if err := os.WriteFile("config.toml", []byte(before), 0600); err != nil {
		t.Fatal(err)
	}
	sess := NewSession()
	sess.StartCapture()
	if err := sess.MergeTOMLFile("config.toml", []TOMLValue{{Path: []string{"model"}, Value: "fast"}}, false); err != nil {
		t.Fatal(err)
	}
	files := sess.StopCapture()
	if len(files) != 1 || !files[0].Merged || len(files[0].Keys) != 1 || !strings.Contains(files[0].Content, "manual") || !strings.Contains(files[0].Content, "fast") {
		t.Errorf("native TOML was not merged into capture: %+v", files)
	}
	if data, err := os.ReadFile("config.toml"); err != nil || string(data) != before {
		t.Errorf("capture changed the native file: %v", err)
	}
}

func TestMergeTOMLFile_RejectsMalformedNativeInput(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.WriteFile("config.toml", []byte("broken = ["), 0600); err != nil {
		t.Fatal(err)
	}
	sess := NewSession()
	sess.StartCapture()
	if err := sess.MergeTOMLFile("config.toml", []TOMLValue{{Path: []string{"model"}, Value: "fast"}}, false); err == nil || !strings.Contains(err.Error(), "parse config.toml") {
		t.Errorf("malformed TOML was not refused with its path: %v", err)
	}
	if files := sess.StopCapture(); len(files) != 0 {
		t.Errorf("malformed input captured output: %+v", files)
	}
}

func TestMergeTOMLFile_ReturnsNativePermissionErrors(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.WriteFile("config.toml", []byte("model = 'manual'\n"), 0000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod("config.toml", 0600) }()
	if _, err := os.ReadFile("config.toml"); !errors.Is(err, os.ErrPermission) {
		t.Skip("filesystem does not enforce the unreadable file mode")
	}
	sess := NewSession()
	sess.StartCapture()
	err := sess.MergeTOMLFile("config.toml", []TOMLValue{{Path: []string{"model"}, Value: "fast"}}, false)
	if !errors.Is(err, os.ErrPermission) || !strings.Contains(err.Error(), "config.toml") {
		t.Errorf("native permission error was not retained: %v", err)
	}
	if files := sess.StopCapture(); len(files) != 0 {
		t.Errorf("unreadable input captured output: %+v", files)
	}
}
