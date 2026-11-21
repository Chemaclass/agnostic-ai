package conversations

import (
	"context"
	"encoding/base64"
	"errors"
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
		"a folder with a curly quote": {
			"cd 'C:\\x’;calc;‘' && claude --resume x",
			"Set-Location -LiteralPath 'C:\\x’’;calc;‘‘' -ErrorAction Stop; claude --resume x",
		},
		"a line already for PowerShell": {
			`Set-Location -LiteralPath 'C:\app' -ErrorAction Stop; claude --resume x`,
			`Set-Location -LiteralPath 'C:\app' -ErrorAction Stop; claude --resume x`,
		},
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

	if len(got) != 7 || !slices.Equal(got[:6], []string{"powershell.exe", "-NoExit", "-NoLogo", "-ExecutionPolicy", "RemoteSigned", "-EncodedCommand"}) {
		t.Fatalf("started %q", got)
	}
	raw, err := base64.StdEncoding.DecodeString(got[6])
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
	var ran [][]string
	launcher := windowsLauncher{run: func(_ context.Context, name string, args ...string) (string, error) {
		ran = append(ran, append([]string{name}, args...))
		return "", nil
	}}

	if err := launcher.OpenLink(context.Background(), "codex://threads/x"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"reg.exe", "query", `HKCR\codex`, "/v", "URL Protocol"},
		{"rundll32.exe", "url.dll,FileProtocolHandler", "codex://threads/x"},
	}
	if !slices.EqualFunc(ran, want, slices.Equal[[]string]) {
		t.Errorf("ran %q, want %q", ran, want)
	}
	ran = nil
	if err := launcher.OpenLink(context.Background(), "https://claude.ai/code/session_01ABCDEFGHJKMNPQRSTVWXYZ01"); err != nil || len(ran) != 1 {
		t.Errorf("a web link: ran %q, err %v; want rundll32 alone", ran, err)
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

func TestCommandHere_GivesWindowsALineForPowerShell(t *testing.T) {
	line := `cd 'C:\app' && claude --resume x`
	if got := commandHere("darwin", line); got != line {
		t.Errorf("macOS got %q", got)
	}
	if got := commandHere("windows", line); got != `Set-Location -LiteralPath 'C:\app' -ErrorAction Stop; claude --resume x` {
		t.Errorf("Windows got %q", got)
	}
	if got := commandHere("windows", ""); got != "" {
		t.Errorf("no command became %q", got)
	}
}

func TestWindowsLauncher_SaysWhenNoAppTakesALink(t *testing.T) {
	opened := false
	launcher := windowsLauncher{run: func(_ context.Context, name string, _ ...string) (string, error) {
		if name == "reg.exe" {
			return "", errors.New("exit status 1")
		}
		opened = true
		return "", nil
	}}

	err := launcher.OpenLink(context.Background(), "cursor://anysphere.cursor-deeplink/x")

	if err == nil || err.Error() != "no app on this computer opens cursor:// links" || opened {
		t.Errorf("err %v, opened %v", err, opened)
	}
}
