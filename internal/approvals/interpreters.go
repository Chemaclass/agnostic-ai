package approvals

import (
	"slices"
	"strings"
)

// shellInterpreter allows only a shell that runs a script file named on
// the line: inline code, stdin and interactive shells ask.
func shellInterpreter(args []shellWord, c simpleCommand) string {
	if c.piped || readsInput(c) {
		return RuleShellSyntax
	}
	for _, a := range args {
		text := a.text
		if text == "--" || !strings.HasPrefix(text, "-") && !strings.HasPrefix(text, "+") {
			if text == "--" || a.glob {
				return RuleShellSyntax
			}
			return ""
		}
		if text == "-" || strings.HasPrefix(text, "--") && text != "--norc" && text != "--noprofile" && text != "--posix" {
			return RuleShellSyntax
		}
		if strings.ContainsAny(strings.TrimLeft(text, "-+"), "csiOo") {
			return RuleShellSyntax
		}
	}
	return RuleShellSyntax
}

// inlineFlags are the options with which an interpreter runs code from
// the line instead of a file.
var inlineFlags = map[string]string{
	"python": "c", "pypy": "c", "node": "epr", "nodejs": "epr", "deno": "", "bun": "e", "ruby": "erI",
	"irb": "", "perl": "eEMIx", "php": "rRBEfa", "lua": "el", "luajit": "elj", "rscript": "e", "r": "e",
	"julia": "eEL", "osascript": "e", "pwsh": "cCeE", "powershell": "cCeE", "elixir": "erS",
}

var inlineLongFlags = []string{"--eval", "--print", "--command", "--import", "--require", "--loader", "--experimental-loader", "--expr", "-command", "-encodedcommand", "-c", "--inspect"}

func interpreter(name string, args []shellWord, c simpleCommand) string {
	if c.piped || readsInput(c) {
		return RuleShellSyntax
	}
	base := strings.TrimRight(name, "0123456789.")
	flags, known := inlineFlags[base]
	if !known {
		flags = "ce"
	}
	for i, a := range args {
		text := strings.ToLower(a.text)
		if a.glob {
			return RuleShellSyntax
		}
		if text == "-" || text == "--" {
			return RuleShellSyntax
		}
		if strings.HasPrefix(text, "--") || base == "pwsh" || base == "powershell" {
			flag, _, _ := strings.Cut(text, "=")
			if slices.Contains(inlineLongFlags, flag) {
				return RuleShellSyntax
			}
			continue
		}
		if strings.HasPrefix(text, "-") {
			if base == "python" || base == "pypy" {
				if text == "-m" {
					return pythonModule(args[i+1:])
				}
			}
			if strings.ContainsAny(strings.TrimPrefix(text, "-"), flags) {
				return RuleShellSyntax
			}
			continue
		}
		if strings.Contains(text, "://") {
			return RuleNetwork
		}
		if base == "deno" || base == "bun" {
			return denoBunRule(args[i:])
		}
		return ""
	}
	return RuleShellSyntax
}

func pythonModule(rest []shellWord) string {
	if len(rest) == 0 {
		return RuleShellSyntax
	}
	switch strings.ToLower(rest[0].text) {
	case "pip", "pip3", "ensurepip", "http.server", "smtpd", "ftplib", "venv", "pipx", "uv", "twine", "poetry":
		return RuleNetwork
	}
	return ""
}

func denoBunRule(rest []shellWord) string {
	switch rest[0].text {
	case "eval", "repl", "jupyter":
		return RuleShellSyntax
	case "x", "add", "install", "i", "upgrade", "update", "create", "init", "remove", "rm", "link", "outdated":
		return RuleNetwork
	case "publish", "deploy":
		return RulePublish
	}
	for _, a := range rest {
		if strings.Contains(a.text, "://") || strings.HasPrefix(a.text, "npm:") || strings.HasPrefix(a.text, "jsr:") {
			return RuleNetwork
		}
	}
	return ""
}

func readsInput(c simpleCommand) bool {
	for _, r := range c.redirects {
		if !r.output && !r.dup {
			return true
		}
	}
	return false
}

// awkRule asks for a program that runs commands, reads other files, or
// writes files; a program in a file is not read, so it asks too.
func awkRule(args []shellWord) string {
	for i, a := range args {
		text := a.text
		switch {
		case text == "-f" || strings.HasPrefix(text, "--file") || text == "-i" || strings.HasPrefix(text, "--include") || text == "-E" || text == "--exec":
			return RuleShellSyntax
		case text == "-v" || text == "-F":
			// Their value is the next word.
			continue
		case i > 0 && (args[i-1].text == "-v" || args[i-1].text == "-F"):
			continue
		case strings.HasPrefix(text, "-"):
			continue
		}
		if strings.Contains(text, "system") || strings.ContainsAny(text, "|>") || strings.Contains(text, "getline") || strings.Contains(text, "ENVIRON") {
			return RuleShellSyntax
		}
		return ""
	}
	return RuleShellSyntax
}

func (g *gate) sedRule(args []shellWord, cwd string) string {
	var scripts []string
	var files []shellWord
	inPlace := false
	explicit := false
	for i := 0; i < len(args); i++ {
		text := args[i].text
		switch {
		case text == "-e" || text == "--expression":
			if i+1 >= len(args) {
				return RuleInput
			}
			scripts = append(scripts, args[i+1].text)
			explicit = true
			i++
		case strings.HasPrefix(text, "--expression="):
			scripts = append(scripts, strings.TrimPrefix(text, "--expression="))
			explicit = true
		case text == "-f" || strings.HasPrefix(text, "--file"):
			return RuleShellSyntax
		case text == "-i" || text == "-I":
			inPlace = true
			// BSD sed reads the next word as the backup suffix.
			if i+1 < len(args) && (args[i+1].text == "" || strings.HasPrefix(args[i+1].text, ".")) {
				i++
			}
		case strings.HasPrefix(text, "-i") || strings.HasPrefix(text, "--in-place"):
			inPlace = true
		case strings.HasPrefix(text, "-") && text != "-":
			if strings.ContainsAny(strings.TrimPrefix(text, "-"), "if") {
				return RuleShellSyntax
			}
		case !explicit && len(scripts) == 0:
			scripts = append(scripts, text)
		default:
			files = append(files, args[i])
		}
	}
	if len(scripts) == 0 {
		return RuleInput
	}
	for _, s := range scripts {
		if !sedScriptReadable(s) {
			return RuleShellSyntax
		}
	}
	if inPlace {
		return g.targets(files, cwd, RuleOutsideProject)
	}
	return ""
}

// sedScriptReadable reports a script made only of s, y, d, p, q, =, and
// the hold-space commands, each with an optional line or regexp
// address, and s flags that do not run or write.
func sedScriptReadable(script string) bool {
	runes := []rune(script)
	i := 0
	skipSpace := func() {
		for i < len(runes) && (runes[i] == ' ' || runes[i] == '\t') {
			i++
		}
	}
	section := func(delim rune) bool {
		for ; i < len(runes); i++ {
			if runes[i] == '\\' {
				i++
				continue
			}
			if runes[i] == '\n' {
				return false
			}
			if runes[i] == delim {
				i++
				return true
			}
		}
		return false
	}
	address := func() bool {
		for k := 0; k < 2; k++ {
			skipSpace()
			switch {
			case i < len(runes) && runes[i] >= '0' && runes[i] <= '9':
				for i < len(runes) && (runes[i] >= '0' && runes[i] <= '9' || runes[i] == '~') {
					i++
				}
			case i < len(runes) && runes[i] == '$':
				i++
			case i < len(runes) && runes[i] == '/':
				i++
				if !section('/') {
					return false
				}
				for i < len(runes) && (runes[i] == 'I' || runes[i] == 'M') {
					i++
				}
			}
			skipSpace()
			if k == 0 && i < len(runes) && runes[i] == ',' {
				i++
				continue
			}
			break
		}
		skipSpace()
		if i < len(runes) && runes[i] == '!' {
			i++
		}
		return true
	}
	for i < len(runes) {
		skipSpace()
		if i < len(runes) && (runes[i] == ';' || runes[i] == '\n') {
			i++
			continue
		}
		if i >= len(runes) {
			break
		}
		if !address() || i >= len(runes) {
			return false
		}
		cmd := runes[i]
		i++
		switch cmd {
		case 's', 'y':
			if i >= len(runes) {
				return false
			}
			delim := runes[i]
			if delim == '\\' || delim == '\n' || delim == ' ' {
				return false
			}
			i++
			// s and y take two sections: the pattern and its replacement.
			for range 2 {
				if !section(delim) {
					return false
				}
			}
			for i < len(runes) && strings.ContainsRune("gpiIm0123456789", runes[i]) && cmd == 's' {
				i++
			}
		case 'd', 'D', 'p', 'P', 'q', 'Q', '=', 'n', 'N', 'g', 'G', 'h', 'H', 'x', 'l', 'z':
			if cmd == 'q' || cmd == 'Q' {
				skipSpace()
				for i < len(runes) && runes[i] >= '0' && runes[i] <= '9' {
					i++
				}
			}
		default:
			return false
		}
		skipSpace()
		if i < len(runes) && runes[i] != ';' && runes[i] != '\n' {
			return false
		}
	}
	return true
}
