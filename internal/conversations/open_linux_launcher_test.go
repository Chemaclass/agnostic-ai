package conversations

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"testing"
)

func displayAt(display string) func(string) string {
	return func(name string) string {
		switch name {
		case "DISPLAY":
			return display
		case "SHELL":
			return "/bin/bash"
		}
		return ""
	}
}

func TestLinuxLauncher_RunsANewTerminalWindowThroughTheUsersShell(t *testing.T) {
	var got []string
	launcher := linuxLauncher{getenv: displayAt(":0"), start: func(name string, args ...string) error {
		got = append([]string{name}, args...)
		return nil
	}}

	if err := launcher.RunInNewTerminal(context.Background(), "cd '/tmp/app' && claude --resume x"); err != nil {
		t.Fatal(err)
	}

	want := []string{"x-terminal-emulator", "-e", "/bin/bash", "-ic", "cd '/tmp/app' && claude --resume x; exec '/bin/bash'"}
	if !slices.Equal(got, want) {
		t.Errorf("started %q\nwant    %q", got, want)
	}
}

func TestLinuxLauncher_WithoutAChosenTerminalSaysTheSystemCannot(t *testing.T) {
	launcher := linuxLauncher{getenv: displayAt(":0"), start: func(string, ...string) error {
		return &exec.Error{Name: "x-terminal-emulator", Err: exec.ErrNotFound}
	}}

	if err := launcher.RunInNewTerminal(context.Background(), "codex resume x"); !errors.Is(err, errNotHere) {
		t.Errorf("err = %v; want errNotHere", err)
	}
}

func TestLinuxLauncher_WithoutADisplayStartsNoTerminal(t *testing.T) {
	launcher := linuxLauncher{getenv: displayAt(""), start: func(string, ...string) error {
		t.Error("a terminal started with no display to show it on")
		return nil
	}}

	if err := launcher.RunInNewTerminal(context.Background(), "codex resume x"); !errors.Is(err, errNotHere) {
		t.Errorf("err = %v; want errNotHere", err)
	}
}
