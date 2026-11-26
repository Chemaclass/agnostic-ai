package conversations

import (
	"context"
	"encoding/base64"
	"strings"
	"unicode/utf16"
)

// windowsLauncher opens links with the app Windows registered for them
// and runs resume lines in a new PowerShell window. It cannot find or
// select a tab of a terminal on Windows.
type windowsLauncher struct {
	run func(ctx context.Context, name string, args ...string) (string, error)
	// start starts a program in a console window of its own and returns
	// without waiting for it.
	start func(name string, args ...string) error
}

func (l windowsLauncher) OpenLink(ctx context.Context, link string) error {
	if err := checkedLink(link); err != nil {
		return err
	}
	_, err := l.run(ctx, "rundll32.exe", "url.dll,FileProtocolHandler", link)
	return err
}

func (windowsLauncher) OpenFolder(context.Context, string, string) error { return errNotHere }

func (windowsLauncher) ActivateApp(context.Context, string) error { return errNotHere }

// RunInNewTerminal runs command in Windows PowerShell, which every
// Windows ships, and keeps the window open after it. The command is
// passed encoded, so no quoting of the Windows command line can change it.
func (l windowsLauncher) RunInNewTerminal(_ context.Context, command string) error {
	units := utf16.Encode([]rune(powerShellLine(command)))
	raw := make([]byte, 0, 2*len(units))
	for _, unit := range units {
		raw = append(raw, byte(unit), byte(unit>>8))
	}
	// An npm install puts a .ps1 shim first, which the default Restricted
	// policy of Windows 11 refuses to run.
	return l.start("powershell.exe", "-NoExit", "-NoLogo", "-ExecutionPolicy", "RemoteSigned", "-EncodedCommand", base64.StdEncoding.EncodeToString(raw))
}

func (windowsLauncher) FocusTab(context.Context, string, string) (bool, error) {
	return false, errNotHere
}

func (windowsLauncher) RunInNewTab(context.Context, string, string) error { return errNotHere }

func (windowsLauncher) ShowTmuxPane(context.Context, int, string) (int, error) {
	return 0, errNotHere
}

func (windowsLauncher) TitleTab(context.Context, string, string, string) (bool, error) {
	return false, errNotHere
}

func (windowsLauncher) ReadTabNames(context.Context, string) ([]TerminalTab, error) {
	return nil, errNotHere
}

func (windowsLauncher) TypeInTab(context.Context, string, string, string, bool) (bool, error) {
	return false, errNotHere
}

// powerShellLine rewrites a line resumeIn wrote for a POSIX shell, such
// as cd 'C:\app' && claude --resume x, for Windows PowerShell, which has
// no && and quotes a quote by doubling it. A quoted program runs with &.
// It stops before the tool when the folder is gone.
func powerShellLine(line string) string {
	prefix := ""
	if after, ok := strings.CutPrefix(line, "cd "); ok {
		if folder, rest, ok := cutShellQuoted(after); ok {
			if command, ok := strings.CutPrefix(rest, " && "); ok {
				prefix = "Set-Location -LiteralPath " + powerShellQuote(folder) + " -ErrorAction Stop; "
				line = command
			}
		}
	}
	if program, rest, ok := cutShellQuoted(line); ok {
		line = "& " + powerShellQuote(program) + rest
	}
	return prefix + line
}

// cutShellQuoted reads the word shellQuote wrote at the start of s.
func cutShellQuoted(s string) (word, rest string, ok bool) {
	if !strings.HasPrefix(s, "'") {
		return "", s, false
	}
	var b strings.Builder
	for i := 1; ; {
		end := strings.IndexByte(s[i:], '\'')
		if end < 0 {
			return "", s, false
		}
		b.WriteString(s[i : i+end])
		i += end + 1
		if strings.HasPrefix(s[i:], `\''`) {
			b.WriteByte('\'')
			i += 3
			continue
		}
		return b.String(), s[i:], true
	}
}

// powerShellQuotes end a single-quoted string in PowerShell; each is
// doubled inside one.
var powerShellQuotes = strings.NewReplacer("'", "''", "\u2018", "\u2018\u2018", "\u2019", "\u2019\u2019", "\u201a", "\u201a\u201a", "\u201b", "\u201b\u201b")

func powerShellQuote(s string) string {
	return "'" + powerShellQuotes.Replace(s) + "'"
}

// commandHere is a resume line as a person on goos runs it by hand.
func commandHere(goos, line string) string {
	if goos != "windows" || line == "" {
		return line
	}
	return powerShellLine(line)
}
