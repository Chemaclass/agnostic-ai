package conversations

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"testing"
)

func TestLinuxLauncher_RunsANewTerminalWindowThatKeepsAShell(t *testing.T) {
	var got []string
	launcher := linuxLauncher{start: func(name string, args ...string) error {
		got = append([]string{name}, args...)
		return nil
	}}

	if err := launcher.RunInNewTerminal(context.Background(), "cd '/tmp/app' && claude --resume x"); err != nil {
		t.Fatal(err)
	}

	want := []string{"x-terminal-emulator", "-e", "sh", "-c", `cd '/tmp/app' && claude --resume x; exec "${SHELL:-sh}"`}
	if !slices.Equal(got, want) {
		t.Errorf("started %q\nwant    %q", got, want)
	}
}

func TestLinuxLauncher_WithoutAChosenTerminalSaysTheSystemCannot(t *testing.T) {
	launcher := linuxLauncher{start: func(string, ...string) error {
		return &exec.Error{Name: "x-terminal-emulator", Err: exec.ErrNotFound}
	}}

	if err := launcher.RunInNewTerminal(context.Background(), "codex resume x"); !errors.Is(err, errNotHere) {
		t.Errorf("err = %v; want errNotHere", err)
	}
}
