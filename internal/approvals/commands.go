package approvals

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// shell checks every command of a shell text and returns the first
// rule that asks, or "" when all pass.
func (g *gate) shell(command string) string {
	commands, err := parseShell(command)
	if err != nil {
		return RuleShellSyntax
	}
	if len(commands) == 0 {
		return RuleInput
	}
	cwd := g.cwd
	// andChain stays true while every command so far ran only after the
	// one before it succeeded, so a cd in it surely took effect.
	andChain := true
	for _, c := range commands {
		if rule := g.command(c, &cwd, andChain); rule != "" {
			return rule
		}
		andChain = andChain && !c.piped && c.sep == sepAnd
	}
	return ""
}

// safeAssignment matches variables a command may set on its line: they
// change no program that runs and no file it reads.
var safeAssignment = regexp.MustCompile(`^(CGO_ENABLED|GOOS|GOARCH|GOAMD64|GOARM|GOARM64|GOEXPERIMENT|GOMAXPROCS|GODEBUG|GOGC|GOMEMLIMIT|NODE_ENV|CI|DEBUG|VERBOSE|NO_COLOR|FORCE_COLOR|CLICOLOR|CLICOLOR_FORCE|TERM|COLUMNS|LINES|TZ|LANG|LANGUAGE|LC_[A-Z]+|RUST_LOG|RUST_BACKTRACE|PYTHONUNBUFFERED|PYTHONDONTWRITEBYTECODE|PYTHONHASHSEED|UPDATE|UPDATE_[A-Z0-9_]+|[A-Z0-9_]+_UPDATE|GOLDEN|TEST_[A-Z0-9_]+)$`)

var (
	negativeNice = regexp.MustCompile(`^-[0-9]+$`)
	duration     = regexp.MustCompile(`^[0-9.]+[smhd]?$`)
	setFlags     = regexp.MustCompile(`^[-+][euxv]+$`)
)

func (g *gate) command(c simpleCommand, cwd *string, andChain bool) string {
	for _, a := range c.assigns {
		if !safeAssignment.MatchString(assignmentName(a)) {
			return RuleShellSyntax
		}
	}
	for _, r := range c.redirects {
		if r.dup {
			continue
		}
		if r.target.glob {
			return RuleShellSyntax
		}
		var rule string
		if r.output {
			rule = g.targetRule(r.target.text, *cwd, RuleDelete)
		} else {
			rule = g.readRule(r.target.text, *cwd)
		}
		if rule != "" {
			return rule
		}
	}
	return g.program(c.words, c, cwd, andChain)
}

// program checks one program and its arguments, after the wrappers
// that only change how it runs.
func (g *gate) program(words []shellWord, c simpleCommand, cwd *string, andChain bool) string {
	words, rule := stripWrappers(words)
	if rule != "" {
		return rule
	}
	if len(words) == 0 {
		return ""
	}
	if words[0].glob || words[0].text == "" {
		return RuleShellSyntax
	}
	if namesPolicyWords(words) {
		return RulePolicy
	}
	name := g.commandName(words[0].text, *cwd)
	args := words[1:]
	if name == "cd" {
		if !andChain || c.piped || c.sep != sepAnd && c.sep != "" {
			return RuleShellSyntax
		}
		return g.changeDir(args, cwd)
	}
	if rule := g.commandRule(name, words[0], args, c, *cwd); rule != "" {
		return rule
	}
	if rule := g.argsRule(name, args, *cwd); rule != "" {
		return rule
	}
	if rest, runs := runsProgram(name, args); runs {
		return g.program(rest, c, cwd, false)
	}
	return ""
}

// namesPolicyWords reports a command that names the approvals policy
// once the shell has removed its quoting.
func namesPolicyWords(words []shellWord) bool {
	texts := make([]string, len(words))
	for i, w := range words {
		texts[i] = w.text
	}
	return namesPolicy(strings.Join(texts, " "))
}

// commandName is the lower-case base name of the program, after the
// links of a path to it when the policy resolves links. A
// case-insensitive file system runs RM as rm.
func (g *gate) commandName(word, cwd string) string {
	if strings.Contains(word, "/") && g.policy.Resolve != nil {
		if abs, ok := g.path(word, cwd); ok {
			word = g.policy.Resolve(abs)
		}
	}
	return strings.ToLower(filepath.Base(word))
}

// stripWrappers drops the programs that only change how the next one
// runs, with the options the gate knows for them. Any other option
// asks, so the program after it is never misplaced.
func stripWrappers(words []shellWord) ([]shellWord, string) {
	for len(words) > 0 {
		name := strings.ToLower(filepath.Base(words[0].text))
		rest := words[1:]
		var ok bool
		switch name {
		case "!", "nohup":
			words, ok = rest, true
		case "time":
			words, ok = knownFlags(rest, []string{"-p"}, nil)
		case "command":
			if len(rest) > 0 && (rest[0].text == "-v" || rest[0].text == "-V") {
				return nil, ""
			}
			words, ok = knownFlags(rest, []string{"-p"}, nil)
		case "nice":
			if len(rest) > 0 && negativeNice.MatchString(rest[0].text) {
				rest = rest[1:]
			}
			words, ok = knownFlags(rest, nil, []string{"-n"})
		case "timeout":
			words, ok = knownFlags(rest, []string{"-v", "--verbose", "--preserve-status", "--foreground"}, []string{"-s", "-k"})
			if ok {
				if len(words) == 0 || !duration.MatchString(words[0].text) {
					return nil, RuleShellSyntax
				}
				words = words[1:]
			}
		default:
			return words, ""
		}
		if !ok {
			return nil, RuleShellSyntax
		}
	}
	return words, ""
}

// knownFlags drops leading options that are in plain or, with their
// next word, in withValue. ok is false at any other option.
func knownFlags(words []shellWord, plain, withValue []string) ([]shellWord, bool) {
	for len(words) > 0 && strings.HasPrefix(words[0].text, "-") && words[0].text != "-" {
		text := words[0].text
		switch {
		case text == "--":
			return words[1:], true
		case slices.Contains(plain, text):
			words = words[1:]
		case slices.Contains(withValue, text) && len(words) > 1:
			words = words[2:]
		default:
			return nil, false
		}
	}
	return words, true
}

// runsProgram reports the words after a subcommand that runs another
// program, as in uv run or bundle exec. An option before the program
// asks, since it may take a value the gate would read as the program.
func runsProgram(name string, args []shellWord) ([]shellWord, bool) {
	runVerbs := map[string]string{
		"uv": "run", "poetry": "run", "pdm": "run", "pipenv": "run", "bundle": "exec", "bundler": "exec",
	}
	verb, ok := runVerbs[name]
	if !ok || len(args) < 2 || args[0].text != verb {
		return nil, false
	}
	if strings.HasPrefix(args[1].text, "-") {
		return []shellWord{{text: "-", glob: true}}, true
	}
	return args[1:], true
}

var (
	deleteCommands = set("rm", "rmdir", "shred", "dd", "truncate", "unlink", "wipefs", "srm", "fdisk", "sfdisk",
		"parted", "trash", "grm", "grmdir", "gshred", "gdd", "gtruncate", "gunlink")

	privilegeCommands = set("sudo", "sudoedit", "su", "doas", "pkexec", "runuser", "chown", "chgrp", "kill", "pkill",
		"killall", "launchctl", "systemctl", "service", "crontab", "at", "batch", "shutdown", "reboot", "halt",
		"poweroff", "mount", "umount", "diskutil", "csrutil", "spctl", "xattr", "chflags", "defaults", "chroot",
		"unshare", "nsenter", "setfacl", "visudo", "passwd", "dscl", "usermod", "useradd", "userdel", "groupadd",
		"sysctl", "gchown", "gchgrp")

	secretCommands = set("aws-vault", "dotenv", "envchain", "security", "pass", "gopass", "op", "env", "genv",
		"printenv", "gprintenv", "ssh-add", "ssh-agent", "keychain", "gpg", "gpg2", "secret-tool", "vault", "bw",
		"lpass", "doppler", "chamber", "sops", "age", "ps")

	networkCommands = set("torsocks", "proxychains", "proxychains4", "curl", "wget", "ssh", "scp", "sftp", "nc",
		"ncat", "netcat", "telnet", "ftp", "tftp", "lftp", "socat", "http", "https", "xh", "aria2c", "lynx", "w3m",
		"links", "mosh", "open", "xdg-open", "npx", "pnpx", "bunx", "uvx", "pipx", "ngrok", "tailscale",
		"cloudflared", "rclone", "nmap", "apt", "apt-get", "yum", "dnf", "pacman", "apk", "zypper", "port", "choco",
		"winget", "scoop", "nix-env", "snap", "flatpak", "brew", "rustup", "pip", "pip3")

	publishCommands = set("twine", "aws", "gcloud", "gsutil", "az", "fly", "flyctl", "vercel", "netlify",
		"heroku", "firebase", "wrangler", "doctl", "eksctl", "ansible", "ansible-playbook", "serverless", "sls",
		"cdk", "sam", "release-it", "semantic-release", "changeset", "lerna", "np", "glab", "hg", "svn", "goreleaser")

	// readOnlyCommands read the paths they are given and write nothing,
	// so naming a protected path is fine for them.
	readOnlyCommands = set("cat", "head", "tail", "grep", "egrep", "fgrep", "rg", "ag", "ls", "wc", "diff", "stat",
		"bat", "jq", "cmp", "cut", "tr", "column", "nl", "od", "hexdump", "strings", "md5", "md5sum", "sha1sum",
		"sha256sum", "sha512sum", "shasum", "basename", "dirname", "realpath", "readlink", "test", "[", "echo",
		"printf", "true", "false", ":", "pwd", "which", "type", "du", "df")

	// plainPrograms run with any arguments; the gate still checks every
	// argument that may name a path.
	plainPrograms = set("date", "sleep", "seq", "uname", "whoami", "id", "base64", "gzip", "gunzip", "zcat",
		"bzip2", "bunzip2", "xz", "unxz", "unzip", "gofmt", "goimports", "golangci-lint", "staticcheck",
		"shellcheck", "shfmt", "prettier", "eslint", "tsc", "jest", "vitest", "mocha", "pytest", "ruff", "black",
		"mypy", "pylint", "flake8", "isort", "rustfmt", "zola", "hugo", "actionlint", "yamllint", "markdownlint",
		"markdownlint-cli2", "hadolint", "typos", "codespell", "gotestsum", "javac", "clang", "clang++", "gcc",
		"g++", "cc", "cmake", "ninja", "protoc", "buf", "rustc", "rspec", "rubocop")

	// shellSyntaxCommands run text as shell code, change the shell, or
	// run another program the gate cannot place.
	shellSyntaxCommands = set("eval", "exec", "source", ".", "alias", "unalias", "function", "trap", "xargs",
		"parallel", "watch", "builtin", "noglob", "nocorrect", "repeat", "-", "unset", "declare", "typeset",
		"local", "readonly", "shopt", "setopt", "unsetopt", "hash", "enable", "disable", "autoload", "zmodload",
		"emulate", "if", "then", "else", "elif", "fi", "for", "foreach", "while", "until", "do", "done", "case",
		"esac", "select", "coproc", "[[", "]]", "pushd", "popd", "let", "read", "mapfile", "readarray", "fc",
		"history", "ulimit", "umask", "exit", "return", "xcrun", "arch", "setsid", "script", "flock")

	releaseWord = regexp.MustCompile(`(?i)release|publish|deploy`)

	runners = set("make", "gmake", "just", "task", "mage", "rake", "mvn", "mvnw", "gradle", "gradlew")

	// interpreterName matches programs that run code; only python, node
	// and the common shells have a checked form, every other one asks.
	interpreterName = regexp.MustCompile(`^(python[0-9]*\.[0-9.]+|pypy[0-9.]*|node[0-9]+|nodejs|ruby[0-9.]*|irb|perl[0-9.]*|php[0-9.]*|lua[0-9.]*|luajit|rscript|r|julia|tclsh|wish|groovy|scala|kotlin|jshell|swift|ghci|runghc|elixir|iex|erl|escript|guile|racket|sbcl|clisp|ocaml|pwsh|powershell|osascript|jjs|qjs|sqlite3|m4|dc|bc|expect|fish|csh|tcsh|ksh|mksh|ash|yash|nu|elvish|xonsh|busybox|toybox)$`)
)

func set(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// commandRule checks a program the gate knows. A program it does not
// know asks, unless it is a file inside the project: running the
// project's own scripts is what the gate cannot judge.
func (g *gate) commandRule(name string, program shellWord, args []shellWord, c simpleCommand, cwd string) string {
	switch {
	case deleteCommands[name] || strings.HasPrefix(name, "mkfs"):
		return RuleDelete
	case privilegeCommands[name]:
		return RulePrivilege
	case secretCommands[name]:
		return RuleSecret
	case networkCommands[name]:
		return RuleNetwork
	case publishCommands[name] || releaseWord.MatchString(name):
		return RulePublish
	case interpreterName.MatchString(name) || shellSyntaxCommands[name]:
		return RuleShellSyntax
	}
	if rule, known := g.knownProgram(name, args, c, cwd); known {
		return rule
	}
	if readOnlyCommands[name] || plainPrograms[name] {
		return ""
	}
	if g.projectProgram(program, cwd) {
		return ""
	}
	return RuleUnknownCommand
}

// projectProgram reports a program named by a path to a file inside
// the project, such as ./scripts/check.sh.
func (g *gate) projectProgram(program shellWord, cwd string) bool {
	if !strings.Contains(program.text, "/") {
		return false
	}
	abs, ok := g.path(program.text, cwd)
	return ok && g.inside(abs) && !g.protected(abs)
}

// knownProgram runs the rule of a program with one; known is false for
// any other.
func (g *gate) knownProgram(name string, args []shellWord, c simpleCommand, cwd string) (rule string, known bool) {
	switch name {
	case "sh", "bash", "zsh", "dash":
		rule = shellInterpreter(args, c)
	case "python", "python3", "node":
		rule = interpreter(name, args, c)
	case "set":
		rule = setBuiltin(args)
	case "export":
		rule = RuleShellSyntax
		if len(args) == 0 || args[0].text == "-p" {
			rule = RuleSecret
		}
	case "chmod", "gchmod":
		rule = g.targets(args, cwd, RuleOutsideProject)
		if flagged(args, []string{"--recursive"}, "R") {
			rule = RulePrivilege
		}
	case "mv", "cp", "install", "ditto", "gmv", "gcp", "ginstall":
		rule = g.targets(args, cwd, RuleDelete)
	case "rsync":
		rule = rsyncRule(args)
		if rule == "" {
			rule = g.targets(args, cwd, RuleDelete)
		}
	case "ln", "touch", "mkdir", "tee", "gln", "gtouch", "gmkdir", "gtee":
		rule = g.targets(args, cwd, RuleOutsideProject)
	case "find", "gfind":
		rule = findRule(args)
	case "awk", "gawk", "mawk", "nawk":
		rule = awkRule(args)
	case "sed", "gsed":
		rule = g.sedRule(args, cwd)
	case "jq":
		rule = jqRule(args)
	case "tar", "gtar", "bsdtar":
		rule = tarRule(args)
	case "zip":
		if flagged(args, []string{"--unzip-command"}, "") || slices.ContainsFunc(args, func(w shellWord) bool { return strings.HasPrefix(w.text, "-TT") }) {
			rule = RuleShellSyntax
		}
	case "sort", "gsort":
		if flagged(args, []string{"--compress-program", "--output", "--files0-from"}, "o") {
			rule = RuleShellSyntax
		}
	case "tree":
		if flagged(args, nil, "o") {
			rule = RuleShellSyntax
		}
	case "fd", "fdfind":
		if flagged(args, []string{"--exec", "--exec-batch"}, "xX") {
			rule = RuleShellSyntax
		}
	case "rg":
		if flagged(args, []string{"--pre", "--hostname-bin", "--search-zip"}, "z") {
			rule = RuleShellSyntax
		}
	case "git":
		rule = gitRule(args)
	case "gh":
		rule = ghRule(args)
	case "docker", "podman":
		rule = dockerRule(args)
	case "kubectl", "oc":
		rule = allowVerbs(args, "get", "describe", "logs", "explain", "version", "top", "api-resources", "api-versions", "cluster-info", "diff")
	case "helm":
		rule = allowVerbs(args, "list", "ls", "status", "get", "show", "template", "lint", "version", "search", "history")
	case "terraform", "tofu":
		rule = allowVerbs(args, "fmt", "validate", "plan", "show", "version", "providers", "graph")
	case "npm", "pnpm", "yarn", "bun":
		rule = packageRule(name, args)
	case "deno":
		rule = denoRule(args)
	case "uv", "poetry", "pdm", "pipenv", "bundle", "bundler":
		rule = runOnlyRule(args)
	case "go":
		rule = goRule(args)
	case "cargo":
		rule = cargoRule(args)
	case "dotnet":
		rule = allowVerbs(args, "build", "test", "format")
	case "agnostic-ai":
		rule = agnosticRule(args)
	default:
		if !runners[name] {
			return "", false
		}
		rule = runnerRule(args, c)
	}
	return rule, true
}

func runnerRule(args []shellWord, c simpleCommand) string {
	if (c.piped || readsInput(c)) && slices.ContainsFunc(args, func(w shellWord) bool {
		return w.text == "-" || w.text == "/dev/stdin" || strings.HasPrefix(w.text, "/dev/fd/")
	}) {
		return RuleShellSyntax
	}
	if flagged(args, []string{"--print-data-base", "--eval"}, "pE") {
		return RuleSecret
	}
	for _, a := range args {
		if !strings.HasPrefix(a.text, "-") && releaseWord.MatchString(a.text) {
			return RulePublish
		}
	}
	return ""
}

func agnosticRule(args []shellWord) string {
	if len(args) > 0 && args[0].text == "hook" {
		return RulePolicy
	}
	if slices.ContainsFunc(args, func(w shellWord) bool { return w.text == "--global" || w.text == "-g" }) {
		return RuleProtectedPath
	}
	return ""
}

// argsRule checks every argument of a command: a credential path, a
// path outside the project, a glob that may reach either, or, for a
// command that may write, a protected path.
func (g *gate) argsRule(name string, args []shellWord, cwd string) string {
	text := textArgs(name, args)
	for i, a := range args {
		if text[i] {
			continue
		}
		if a.glob {
			if rule := g.globRule(a.text, cwd); rule != "" {
				return rule
			}
			continue
		}
		for _, value := range argValues(a) {
			if rule := g.argValueRule(name, value, cwd); rule != "" {
				return rule
			}
		}
	}
	return ""
}

// textArgs are the arguments a command reads as text, not as paths:
// what echo prints, a search pattern, a sed or awk program, a jq filter,
// a commit message.
func textArgs(name string, args []shellWord) map[int]bool {
	text := map[int]bool{}
	switch name {
	case "echo", "printf":
		for i := range args {
			text[i] = true
		}
	case "grep", "egrep", "fgrep", "rg", "ag", "sed", "gsed":
		markPattern(text, args, "e", []string{"--regexp", "--expression"}, []string{"--file"})
	case "awk", "gawk", "mawk", "nawk":
		markPattern(text, args, "", nil, []string{"--file"})
	case "jq":
		markPattern(text, args, "", nil, []string{"--from-file"})
	case "git":
		for i, a := range args {
			if (a.text == "-m" || a.text == "--message") && i+1 < len(args) {
				text[i+1] = true
			}
		}
	}
	return text
}

// markPattern marks a command's pattern or program as text: the value
// of its pattern option, or else its first operand. A pattern option in
// any form, or a file option, means no operand is the pattern, so every
// operand stays a path to check.
func markPattern(text map[int]bool, args []shellWord, patternShort string, patternLong, fileLong []string) {
	const fileShort = "f"
	explicit := false
	for i, a := range args {
		w := a.text
		switch {
		case strings.HasPrefix(w, "--"):
			name, _, hasValue := strings.Cut(w, "=")
			if optionPrefixOf(name, patternLong) {
				explicit = true
				if !hasValue && i+1 < len(args) {
					text[i+1] = true
				}
			} else if optionPrefixOf(name, fileLong) {
				explicit = true
			}
		case strings.HasPrefix(w, "-") && len(w) > 1:
			letters := w[1:]
			if patternShort != "" && strings.Contains(letters, patternShort) {
				explicit = true
				if strings.HasSuffix(letters, patternShort) && i+1 < len(args) {
					text[i+1] = true
				}
			}
			if strings.Contains(letters, fileShort) {
				explicit = true
			}
		}
	}
	if explicit {
		return
	}
	for i, a := range args {
		if !strings.HasPrefix(a.text, "-") {
			text[i] = true
			return
		}
	}
}

// argValues are the parts of a word that may name a path: the word, the
// value of a --name=value option, or a value glued to a short option.
func argValues(a shellWord) []string {
	text := a.text
	if text == "" || text == "-" || text == "--" {
		return nil
	}
	if strings.HasPrefix(text, "-") {
		if _, value, ok := strings.Cut(text, "="); ok && value != "" {
			return []string{value}
		}
		// A short option may carry a path: -o.git/hooks/pre-commit.
		if !strings.HasPrefix(text, "--") && len(text) > 2 {
			if value := text[2:]; strings.ContainsRune(value, '/') || strings.HasPrefix(value, ".") || strings.HasPrefix(value, "~") {
				return []string{value}
			}
		}
		return nil
	}
	return []string{text}
}

func (g *gate) argValueRule(name, value, cwd string) string {
	if strings.Contains(value, "://") {
		return ""
	}
	abs, ok := g.path(value, cwd)
	if !ok {
		if pathLike(value) || strings.HasPrefix(value, "~") {
			return RuleOutsideProject
		}
		if secretName(filepath.Base(value)) {
			return RuleSecret
		}
		return ""
	}
	if g.secret(abs) {
		return RuleSecret
	}
	if isDevice(abs) {
		return ""
	}
	if !g.inside(abs) {
		return RuleOutsideProject
	}
	if !readOnlyCommands[name] && (g.protected(abs) || g.protectedBelow(g.relative(abs))) {
		return RuleProtectedPath
	}
	return ""
}

// pathLike is a word that names a path outside the working folder when
// read as one: absolute, from home, or climbing with "..".
func pathLike(word string) bool {
	return strings.HasPrefix(word, "/") || strings.HasPrefix(word, "~") || word == ".." ||
		strings.HasPrefix(word, "../") || strings.Contains(word, "/../") || strings.HasSuffix(word, "/..") ||
		strings.HasPrefix(word, `\\`) || isWindowsPath(word)
}

// targets checks every non-option argument of a command that writes,
// as a path it writes.
func (g *gate) targets(args []shellWord, cwd, outsideRule string) string {
	for _, a := range args {
		if a.glob {
			return RuleShellSyntax
		}
		for _, value := range argValues(a) {
			if strings.Contains(value, "://") || hostPath(value) {
				return RuleNetwork
			}
			if rule := g.targetRule(value, cwd, outsideRule); rule != "" {
				return rule
			}
		}
	}
	return ""
}

func (g *gate) changeDir(args []shellWord, cwd *string) string {
	if len(args) != 1 || args[0].glob || strings.HasPrefix(args[0].text, "-") {
		return RuleOutsideProject
	}
	abs, ok := g.path(args[0].text, *cwd)
	if !ok || !g.inside(abs) {
		return RuleOutsideProject
	}
	*cwd = abs
	return ""
}

func setBuiltin(args []shellWord) string {
	if len(args) == 0 {
		return RuleSecret
	}
	for i := 0; i < len(args); i++ {
		text := args[i].text
		switch {
		case (strings.HasPrefix(text, "-") || strings.HasPrefix(text, "+")) && strings.HasSuffix(text, "o") && setFlags.MatchString(strings.TrimSuffix(text, "o")+"e"):
			if i+1 >= len(args) || args[i+1].text != "pipefail" {
				return RuleShellSyntax
			}
			i++
		case setFlags.MatchString(text):
		default:
			return RuleShellSyntax
		}
	}
	return ""
}

// flagged reports whether args hold one of the long options, alone, as
// --name=value or as a prefix GNU tools accept, or a short option
// letter, alone or in a bundle such as -xvf.
func flagged(args []shellWord, longs []string, shorts string) bool {
	for _, a := range args {
		w := a.text
		switch {
		case w == "--":
			return false
		case strings.HasPrefix(w, "--"):
			name, _, _ := strings.Cut(w, "=")
			if optionPrefixOf(name, longs) {
				return true
			}
		case strings.HasPrefix(w, "-") && len(w) > 1 && shorts != "":
			if strings.ContainsAny(w[1:], shorts) {
				return true
			}
		}
	}
	return false
}

// optionPrefixOf reports whether name is one of longs or a prefix of
// one long enough to be read as it.
func optionPrefixOf(name string, longs []string) bool {
	if len(name) < 4 {
		return false
	}
	for _, long := range longs {
		if strings.HasPrefix(long, name) {
			return true
		}
	}
	return false
}
