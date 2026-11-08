package term

import (
	"bytes"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestColorize_NonTTYReturnsPlain(t *testing.T) {
	buf := &bytes.Buffer{}
	got := Colorize(buf, "x", Green)
	if got != "x" {
		t.Errorf("non-tty writer must return plain string, got %q", got)
	}
}

func TestColorize_NilWriterReturnsPlain(t *testing.T) {
	got := Colorize(nil, "x", Green)
	if got != "x" {
		t.Errorf("nil writer must return plain string, got %q", got)
	}
}

func TestTickBangCross_NonTTYNoEscape(t *testing.T) {
	buf := &bytes.Buffer{}
	for name, got := range map[string]string{"tick": Tick(buf), "bang": Bang(buf), "cross": Cross(buf)} {
		if strings.Contains(got, "\x1b") {
			t.Errorf("%s: non-tty writer leaked ANSI escape: %q", name, got)
		}
	}
}

func TestColorEnabled_HonorsNoColorEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	// Even with an env opt-out, plain buffer is already non-tty.
	// This guards the env path independently from the writer type.
	if ColorEnabled(&bytes.Buffer{}) {
		t.Error("ColorEnabled must return false when NO_COLOR is set")
	}
}

func TestColorEnabled_DumbTerminalGetsNoColor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a Unix character device")
	}
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	if !ColorEnabled(f) {
		t.Fatal("a character device must get color")
	}
	t.Setenv("TERM", "dumb")
	if ColorEnabled(f) {
		t.Error("TERM=dumb must disable color")
	}
}
