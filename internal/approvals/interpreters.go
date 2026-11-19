package approvals

import (
	"slices"
	"strings"
)

// shellInterpreter allows only a shell that runs a script file named on
// the line, with trace and error options: inline code, stdin and
// interactive shells ask.
func shellInterpreter(args []shellWord, c simpleCommand) string {
	if c.piped || readsInput(c) {
		return RuleShellSyntax
	}
	for _, a := range args {
		text := a.text
		if !strings.HasPrefix(text, "-") {
			if a.glob {
				return RuleShellSyntax
			}
			return ""
		}
		if !setFlags.MatchString(text) || strings.HasPrefix(text, "+") {
			return RuleShellSyntax
		}
	}
	return RuleShellSyntax
}

// pythonFlags and nodeFlags are the options that run no code from the
// line and take no value, so the next word is still read correctly.
var (
	pythonFlags = []string{"-u", "-B", "-O", "-OO", "-E", "-s", "-S", "-I", "-q", "-b", "-bb", "-v", "-P"}
	nodeFlags   = []string{"--test", "--enable-source-maps", "--trace-warnings", "--no-warnings", "--trace-uncaught", "--throw-deprecation", "--pending-deprecation"}

	// pythonModules are the modules python -m may run: test runners and
	// linters that read the project and fetch nothing.
	pythonModules = []string{"pytest", "unittest", "mypy", "black", "ruff", "pylint", "flake8", "isort", "coverage", "json.tool", "compileall", "py_compile", "doctest", "venv"}
)

// interpreter allows python or node only as `prog [known options]
// script [args]`, or python -m with a listed module. Everything after
// the script belongs to it. Any other option asks: it may run code from
// the line or take a value the gate would read as the script.
func interpreter(name string, args []shellWord, c simpleCommand) string {
	if c.piped || readsInput(c) {
		return RuleShellSyntax
	}
	flags := nodeFlags
	if name != "node" {
		flags = pythonFlags
	}
	for i, a := range args {
		text := a.text
		switch {
		case a.glob:
			return RuleShellSyntax
		case name != "node" && text == "-m":
			if i+1 < len(args) && slices.Contains(pythonModules, args[i+1].text) {
				return ""
			}
			if i+1 < len(args) && slices.Contains([]string{"pip", "pip3", "ensurepip", "http.server", "twine", "build"}, args[i+1].text) {
				return RuleNetwork
			}
			return RuleShellSyntax
		case strings.HasPrefix(text, "-"):
			if !slices.Contains(flags, text) {
				return RuleShellSyntax
			}
		case strings.Contains(text, "://"):
			return RuleNetwork
		default:
			return ""
		}
	}
	return RuleShellSyntax
}

func readsInput(c simpleCommand) bool {
	for _, r := range c.redirects {
		if !r.output && !r.dup {
			return true
		}
	}
	return false
}

func findRule(args []shellWord) string {
	for _, a := range args {
		switch a.text {
		case "-exec", "-execdir", "-ok", "-okdir":
			return RuleShellSyntax
		case "-delete":
			return RuleDelete
		case "-fprint", "-fprint0", "-fprintf", "-fls":
			return RuleShellSyntax
		}
	}
	return ""
}

// awkRule allows only programs given on the line, with -F and -v, that
// run no command, read no file or variable they were not given, and
// write nothing. Any other option asks.
func awkRule(args []shellWord) string {
	var programs []string
	for i := 0; i < len(args); i++ {
		text := args[i].text
		switch {
		case text == "-F" || text == "-v":
			i++
		case strings.HasPrefix(text, "-F") || strings.HasPrefix(text, "-v"):
		case text == "--":
		case strings.HasPrefix(text, "-"):
			return RuleShellSyntax
		default:
			programs = append(programs, text)
			i = len(args)
		}
	}
	if len(programs) == 0 {
		return RuleShellSyntax
	}
	for _, p := range programs {
		for _, risky := range []string{"system", "getline", "|", ">", "ENVIRON", "ARGV", "ARGC", "PROCINFO", "@", "close", "fflush", "printf -"} {
			if strings.Contains(p, risky) {
				return RuleShellSyntax
			}
		}
	}
	return ""
}

// jqRule asks for a filter that reads the environment, or for options
// that read other files or set the filter from a file.
func jqRule(args []shellWord) string {
	text := map[int]bool{}
	markPattern(text, args, "", nil, []string{"--from-file"})
	for i, a := range args {
		if text[i] && (strings.Contains(strings.ToLower(a.text), "env") || strings.Contains(a.text, "$__loc__") || strings.Contains(a.text, "input_filename")) {
			return RuleSecret
		}
	}
	if flagged(args, []string{"--from-file", "--rawfile", "--slurpfile", "--library-path"}, "fL") {
		return RuleShellSyntax
	}
	return ""
}

// tarRule asks for options that run a command or write outside the
// paths the archive names, in any form tar accepts: long, abbreviated,
// bundled with a dash, or bundled without one as the first word.
func tarRule(args []shellWord) string {
	if flagged(args, []string{"--to-command", "--use-compress-program", "--checkpoint-action", "--info-script",
		"--new-volume-script", "--absolute-names", "--unlink-first", "--recursive-unlink", "--remove-files", "--rsh-command"}, "PIFU") {
		return RuleShellSyntax
	}
	if len(args) > 0 && !strings.HasPrefix(args[0].text, "-") && strings.ContainsAny(args[0].text, "PIFU") {
		return RuleShellSyntax
	}
	return ""
}

// sedRule reads the script strictly: only commands that print, delete
// or substitute pass; e, w, r and anything not read ask. In-place
// edits check their files as targets. Any option outside the plain
// ones asks, since GNU sed reads abbreviated long options.
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
		case text == "-i" || text == "-I":
			inPlace = true
			// BSD sed reads the next word as the backup suffix.
			if i+1 < len(args) && (args[i+1].text == "" || strings.HasPrefix(args[i+1].text, ".")) {
				i++
			}
		case strings.HasPrefix(text, "-i") && !strings.ContainsAny(text[2:], "/~") || text == "--in-place" || strings.HasPrefix(text, "--in-place="):
			inPlace = true
		case slices.Contains([]string{"--quiet", "--silent", "--regexp-extended", "--null-data", "--separate", "--posix", "--sandbox", "--debug", "--unbuffered"}, text):
		case strings.HasPrefix(text, "--"):
			return RuleShellSyntax
		case strings.HasPrefix(text, "-") && text != "-":
			if strings.Trim(text[1:], "nErsuz") != "" {
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
		for k := range 2 {
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
