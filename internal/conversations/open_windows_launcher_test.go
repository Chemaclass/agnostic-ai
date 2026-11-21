package conversations

import (
	"context"
	"encoding/base64"
	"runtime"
	"slices"
	"testing"
	"time"
	"unicode/utf16"
)

func TestPowerShellLine_TurnsAResumeLineIntoPowerShell(t *testing.T) {
	for name, tc := range map[string]struct{ line, want string }{
		"a resume in a folder": {
			`cd 'C:\Users\me\app' && claude --resume x`,
			`Set-Location -LiteralPath 'C:\Users\me\app' -ErrorAction Stop; claude --resume x`,
		},
		"a folder with a quote": {
			`cd 'C:\it'\''s' && codex resume x`,
			`Set-Location -LiteralPath 'C:\it''s' -ErrorAction Stop; codex resume x`,
		},
		"a quoted program": {
			`cd 'C:\app' && 'C:\Program Files\agnostic-ai\agnostic-ai.exe' conversations run b1`,
			`Set-Location -LiteralPath 'C:\app' -ErrorAction Stop; & 'C:\Program Files\agnostic-ai\agnostic-ai.exe' conversations run b1`,
		},
		"no folder": {`claude --resume x`, `claude --resume x`},
	} {
		t.Run(name, func(t *testing.T) {
			if got := powerShellLine(tc.line); got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestWindowsLauncher_RunsANewPowerShellWindowThatStaysOpen(t *testing.T) {
	var got []string
	launcher := windowsLauncher{start: func(name string, args ...string) error {
		got = append([]string{name}, args...)
		return nil
	}}

	if err := launcher.RunInNewTerminal(context.Background(), `cd 'C:\app' && claude --resume x`); err != nil {
		t.Fatal(err)
	}

	if len(got) != 5 || !slices.Equal(got[:4], []string{"powershell.exe", "-NoExit", "-NoLogo", "-EncodedCommand"}) {
		t.Fatalf("started %q", got)
	}
	raw, err := base64.StdEncoding.DecodeString(got[4])
	if err != nil || len(raw)%2 != 0 {
		t.Fatalf("encoded command %q: %v", got[4], err)
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = uint16(raw[2*i]) | uint16(raw[2*i+1])<<8
	}
	if line := string(utf16.Decode(units)); line != `Set-Location -LiteralPath 'C:\app' -ErrorAction Stop; claude --resume x` {
		t.Errorf("runs %q", line)
	}
}

func TestWindowsLauncher_OpensALinkWithTheRegisteredApp(t *testing.T) {
	var got []string
	launcher := windowsLauncher{run: func(_ context.Context, name string, args ...string) (string, error) {
		got = append([]string{name}, args...)
		return "", nil
	}}

	if err := launcher.OpenLink(context.Background(), "codex://threads/x"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"rundll32.exe", "url.dll,FileProtocolHandler", "codex://threads/x"}; !slices.Equal(got, want) {
		t.Errorf("ran %q, want %q", got, want)
	}
	if err := launcher.OpenLink(context.Background(), "file:///C:/Windows/System32/calc.exe"); err == nil {
		t.Error("a file link was opened")
	}
}

func TestMayStillRun_OnlyARecentSessionOnWindowsSystemProcesses(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	recent := Conversation{UpdatedAt: "2026-11-21T08:00:00Z", Open: Route{Command: "claude --resume x"}}
	quiet := Conversation{UpdatedAt: "2026-11-19T08:00:00Z", Open: Route{Command: "claude --resume x"}}
	if !cannotCheckProcesses(SystemProcesses{}, "windows") || cannotCheckProcesses(SystemProcesses{}, "linux") || cannotCheckProcesses(fakeProcesses{}, "windows") {
		t.Error("only Windows' own processes cannot be checked")
	}
	if mayStillRun(quiet, SystemProcesses{}, now) {
		t.Error("a dormant session may still run")
	}
	if got := mayStillRun(recent, SystemProcesses{}, now); got != (runtime.GOOS == "windows") {
		t.Errorf("a recent session may still run = %v on %s", got, runtime.GOOS)
	}
}
