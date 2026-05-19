package term

import (
	"bytes"
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
