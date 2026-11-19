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
	for _, c := range commands {
		if rule := g.command(c, &cwd); rule != "" {
			return rule
		}
	}
	return ""
}

// riskyAssignment matches variables that change which program runs or
// what it loads.
var riskyAssignment = regexp.MustCompile(`^(PATH|IFS|ENV|BASH_ENV|PROMPT_COMMAND|SHELLOPTS|BASHOPTS|LD_.*|DYLD_.*|GIT_.*|NODE_OPTIONS|NODE_PATH|PYTHONPATH|PYTHONSTARTUP|PYTHONHOME|PERL5OPT|PERL5LIB|PERLLIB|RUBYOPT|RUBYLIB|GOFLAGS|EDITOR|VISUAL|PAGER|GIT_PAGER|SSH_ASKPASS|SUDO_ASKPASS|HOME|CDPATH|GLOBIGNORE|BASH_.*|PWD|OLDPWD|TMPDIR|XDG_.*|ZDOTDIR)=`)

func (g *gate) command(c simpleCommand, cwd *string) string {
	for _, a := range c.assigns {
		if riskyAssignment.MatchString(strings.ToUpper(a.text)) {
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
		target := r.target.text
		if r.target.tilde && !strings.HasPrefix(target, "~") {
			return RuleShellSyntax
		}
		var rule string
		if r.output {
			rule = g.targetRule(target, *cwd, RuleDelete)
		} else {
			rule = g.readRule(target, *cwd)
		}
		if rule != "" {
			return rule
		}
	}
	return g.program(c.words, c, cwd)
}

// program checks one program and its arguments, after the wrappers
// that only change how it runs.
func (g *gate) program(words []shellWord, c simpleCommand, cwd *string) string {
	words, rule := stripWrappers(words)
	if rule != "" {
		return rule
	}
	if len(words) == 0 {
		return ""
	}
	if words[0].glob {
		return RuleShellSyntax
	}
	name := g.commandName(words[0].text, *cwd)
	args := words[1:]
	if rule := g.commandRule(name, args, c, cwd); rule != "" {
		return rule
	}
	if rule := g.argsRule(name, args, *cwd); rule != "" {
		return rule
	}
	if rest, runs := runsProgram(name, args); runs {
		return g.anyProgram(rest, c, *cwd)
	}
	return ""
}

// anyProgram checks words whose program the gate cannot place, because
// options before it may take values: each word that is no option is
// tried as the program, and any that asks makes the whole ask.
func (g *gate) anyProgram(words []shellWord, c simpleCommand, cwd string) string {
	for i, w := range words {
		if strings.HasPrefix(w.text, "-") {
			continue
		}
		dir := cwd
		if rule := g.program(words[i:], c, &dir); rule != "" {
			return rule
		}
	}
	return ""
}

// runsProgram reports the words after a subcommand that runs another
// program, as in uv run, bundle exec or mise exec.
func runsProgram(name string, args []shellWord) ([]shellWord, bool) {
	v := verbs(args)
	if len(v) == 0 {
		return nil, false
	}
	runVerbs := map[string][]string{
		"uv": {"run"}, "poetry": {"run"}, "pdm": {"run"}, "pipenv": {"run"}, "hatch": {"run"}, "rye": {"run"},
		"pixi": {"run", "exec"}, "conda": {"run"}, "mamba": {"run"}, "micromamba": {"run"},
		"bundle": {"exec"}, "bundler": {"exec"}, "pnpm": {"exec"}, "yarn": {"exec", "run"}, "mise": {"exec", "x"},
		"asdf": {"exec"}, "direnv": {"exec"}, "rtx": {"exec", "x"}, "devbox": {"run"}, "flox": {"activate"},
	}
	if !slices.Contains(runVerbs[name], v[0]) {
		return nil, false
	}
	for i, a := range args {
		if a.text == v[0] {
			return args[i+1:], true
		}
	}
	return nil, false
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

// wrappers run the rest of the line as a command; their options are
// skipped so the command after them is checked.
func stripWrappers(words []shellWord) ([]shellWord, string) {
	for len(words) > 0 {
		name := strings.ToLower(filepath.Base(words[0].text))
		rest := words[1:]
		switch name {
		case "!", "nohup", "builtin", "busybox", "toybox":
			words = rest
		case "time":
			words = skipFlags(rest, nil)
		case "command":
			if len(rest) > 0 && (rest[0].text == "-v" || rest[0].text == "-V") {
				return nil, ""
			}
			words = skipFlags(rest, nil)
		case "nice":
			words = skipFlags(rest, []string{"-n"})
		case "arch", "setsid", "valgrind", "xcrun", "chronic", "unbuffer", "fakeroot", "linux32", "linux64", "catchsegv", "entr", "ifne", "pv":
			words = skipFlags(rest, []string{"-arch", "-sdk", "--sdk", "--toolchain", "-s", "-d"})
		case "stdbuf":
			words = skipFlags(rest, []string{"-i", "-o", "-e"})
		case "timeout":
			words = skipFlags(rest, []string{"-s", "-k"})
			if len(words) == 0 {
				return nil, RuleInput
			}
			words = words[1:]
		default:
			return words, ""
		}
	}
	return words, ""
}

// skipFlags drops leading options; those in withValue take the next word.
func skipFlags(words []shellWord, withValue []string) []shellWord {
	for len(words) > 0 && strings.HasPrefix(words[0].text, "-") && words[0].text != "-" {
		if words[0].text == "--" {
			return words[1:]
		}
		if slices.Contains(withValue, words[0].text) && len(words) > 1 {
			words = words[2:]
			continue
		}
		words = words[1:]
	}
	return words
}

var (
	deleteCommands = set("rm", "rmdir", "shred", "dd", "truncate", "unlink", "wipefs", "srm", "fdisk", "sfdisk", "parted")

	privilegeCommands = set("sudo", "su", "doas", "pkexec", "runuser", "chown", "chgrp", "kill", "pkill", "killall",
		"launchctl", "systemctl", "service", "crontab", "at", "batch", "shutdown", "reboot", "halt", "poweroff",
		"mount", "umount", "diskutil", "csrutil", "spctl", "xattr", "chflags", "defaults", "chroot", "unshare",
		"nsenter", "setfacl", "visudo", "passwd", "dscl", "usermod", "useradd", "userdel", "groupadd", "sysctl",
		"chrt", "ionice", "taskset", "caffeinate")

	secretCommands = set("aws-vault", "dotenv", "envchain", "security", "pass", "gopass", "op", "env", "printenv", "ssh-add", "ssh-agent", "keychain",
		"gpg", "gpg2", "secret-tool", "vault", "bw", "lpass", "doppler", "chamber", "sops", "age")

	networkCommands = set("torsocks", "proxychains", "proxychains4", "curl", "wget", "ssh", "scp", "sftp", "nc", "ncat", "netcat", "telnet", "ftp", "tftp",
		"socat", "http", "https", "xh", "aria2c", "lynx", "w3m", "links", "mosh", "open", "xdg-open", "npx", "bunx",
		"uvx", "ngrok", "tailscale", "cloudflared", "nmap", "dig", "nslookup", "host", "whois", "ping", "traceroute",
		"apt", "apt-get", "yum", "dnf", "pacman", "apk", "zypper", "port", "choco", "winget", "scoop", "nix-env",
		"snap", "flatpak", "sdk", "asdf", "mise", "rustup", "nvm", "pyenv", "rbenv")

	publishCommands = set("twine", "aws", "gcloud", "gsutil", "az", "fly", "flyctl", "vercel", "netlify", "heroku",
		"firebase", "wrangler", "doctl", "eksctl", "ansible", "ansible-playbook", "serverless", "sls", "cdk", "sam",
		"gh-release", "release-it", "semantic-release", "changeset", "lerna", "np")

	shellSyntaxCommands = set("eval", "exec", "source", ".", "alias", "unalias", "function", "trap", "xargs",
		"parallel", "watch", "script", "expect", "flock", "tmux", "screen", "osascript", "hash", "shopt", "enable",
		"readonly", "declare", "typeset", "local", "unset", "coproc", "fc", "history", "cmd", "pwsh", "powershell",
		"vi", "vim", "nvim", "ex", "ed", "emacs", "nano", "gdb", "lldb", "strace", "dtrace", "ltrace", "patch",
		"if", "then", "else", "elif", "fi", "for", "while", "until", "do", "done", "case", "esac", "select", "[[", "]]",
		"popd", "pushd", "let", "mapfile", "readarray", "complete", "bind", "ulimit", "umask", "jobs", "fg", "bg",
		"disown", "wait", "suspend", "logout", "exit", "return", "break", "continue", "shift", "getopts", "read",
		"firejail", "bwrap", "sandbox-exec", "nix-shell", "nix", "hyperfine", "watchexec", "nodemon", "concurrently",
		"npm-run-all", "run-p", "run-s", "sg", "newgrp", "chpst", "daemonize", "start-stop-daemon", "systemd-run")

	shells = set("sh", "bash", "zsh", "dash", "ksh", "mksh", "fish", "csh", "tcsh", "ash", "yash", "rbash", "nu", "elvish", "xonsh")

	// readOnlyCommands read the paths they are given and write nothing,
	// so naming a protected path is fine for them.
	readOnlyCommands = set("cat", "less", "more", "head", "tail", "grep", "egrep", "fgrep", "rg", "ag", "ls", "wc",
		"diff", "stat", "bat", "jq", "cmp", "cut", "tr", "column", "nl", "od",
		"hexdump", "strings", "md5", "md5sum", "sha1sum", "sha256sum", "shasum", "basename", "dirname", "realpath",
		"readlink", "test", "[", "echo", "printf", "true", "false", "pwd", "which", "type", "whereis", "du", "df")

	releaseWord = regexp.MustCompile(`(?i)release|publish|deploy`)

	runners = set("make", "gmake", "just", "task", "mage", "rake", "mvn", "mvnw", "gradle", "gradlew", "ant", "bazel", "bazelisk", "earthly", "nx", "turbo")
)

func set(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// interpreterName matches programs that run code given inline, from
// stdin, or from a file named on the line.
var interpreterName = regexp.MustCompile(`^(python[0-9.]*|pypy[0-9.]*|node[0-9]*|nodejs|deno|bun|ruby[0-9.]*|irb|perl[0-9.]*|php[0-9.]*|lua[0-9.]*|luajit|rscript|r|julia|tclsh|wish|groovy|scala|kotlin|jshell|swift|ghci|runghc|elixir|iex|erl|escript|guile|racket|sbcl|clisp|ocaml|pwsh|powershell|osascript|jjs|qjs)$`)

func (g *gate) commandRule(name string, args []shellWord, c simpleCommand, cwd *string) string {
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
	case shellSyntaxCommands[name]:
		return RuleShellSyntax
	case shells[name]:
		return shellInterpreter(args, c)
	case interpreterName.MatchString(name):
		return interpreter(name, args, c)
	}
	switch name {
	case "cd":
		return g.changeDir(args, cwd)
	case "export":
		if len(args) == 0 || args[0].text == "-p" {
			return RuleSecret
		}
		return RuleShellSyntax
	case "set":
		return setBuiltin(args)
	case "chmod":
		if slices.ContainsFunc(args, func(w shellWord) bool {
			return w.text == "--recursive" || strings.HasPrefix(w.text, "-") && !strings.HasPrefix(w.text, "--") && strings.Contains(w.text, "R")
		}) {
			return RulePrivilege
		}
		return g.targets(args, *cwd, RuleOutsideProject)
	case "mv", "cp", "install", "rsync", "ditto":
		if name == "rsync" {
			if rule := rsyncRule(args); rule != "" {
				return rule
			}
		}
		return g.targets(args, *cwd, RuleDelete)
	case "ln", "touch", "mkdir", "tee", "mkfifo", "split", "csplit":
		return g.targets(args, *cwd, RuleOutsideProject)
	case "find":
		return findRule(args)
	case "fd", "fdfind":
		if hasOption(args, "-x", "-X", "--exec", "--exec-batch") {
			return RuleShellSyntax
		}
	case "rg":
		if hasOption(args, "--pre", "--hostname-bin", "--search-zip", "-z") {
			return RuleShellSyntax
		}
	case "tar", "gtar", "bsdtar":
		if hasOption(args, "--to-command", "--use-compress-program", "-I", "--checkpoint-action", "--info-script", "-F",
			"--new-volume-script", "-P", "--absolute-names", "--unlink-first", "--recursive-unlink", "--remove-files") {
			return RuleShellSyntax
		}
		// Bundled short options, with or without a dash: -xPf, xPf.
		for i, a := range args {
			bundle := strings.HasPrefix(a.text, "-") && !strings.HasPrefix(a.text, "--") || i == 0
			if bundle && strings.ContainsAny(a.text, "PIF") {
				return RuleShellSyntax
			}
		}
	case "zip":
		if hasOption(args, "-TT", "--unzip-command") {
			return RuleShellSyntax
		}
	case "sort":
		if hasOption(args, "--compress-program") {
			return RuleShellSyntax
		}
	case "man":
		if hasOption(args, "-P", "--pager", "-H", "--html") {
			return RuleShellSyntax
		}
	case "awk", "gawk", "mawk", "nawk":
		return awkRule(args)
	case "sed", "gsed":
		return g.sedRule(args, *cwd)
	case "git":
		return gitRule(args)
	case "gh":
		return ghRule(args)
	case "docker", "podman", "nerdctl":
		return dockerRule(args)
	case "kubectl", "oc", "k":
		return allowVerbs(args, "get", "describe", "logs", "explain", "version", "top", "api-resources", "api-versions", "cluster-info", "diff")
	case "helm":
		return allowVerbs(args, "list", "ls", "status", "get", "show", "template", "lint", "version", "search", "history", "dependency")
	case "terraform", "tofu", "terragrunt":
		return allowVerbs(args, "fmt", "validate", "plan", "show", "version", "providers", "graph")
	case "pulumi":
		return allowVerbs(args, "preview", "version", "about")
	case "goreleaser":
		return allowVerbs(args, "check", "healthcheck", "build")
	case "npm", "pnpm", "yarn", "bun":
		return packageRule(name, args)
	case "pip", "pip3", "pipx", "uv", "poetry", "pdm", "conda", "mamba", "micromamba", "hatch", "flit", "rye", "pixi":
		return pythonPackageRule(args)
	case "go":
		return goRule(args)
	case "cargo":
		return cargoRule(args)
	case "gem", "bundle", "bundler", "composer", "mix", "dotnet", "nuget", "brew":
		return toolchainRule(name, args)
	case "agnostic-ai":
		if slices.ContainsFunc(args, func(w shellWord) bool { return w.text == "--global" || w.text == "-g" }) {
			return RuleProtectedPath
		}
	}
	if runners[name] {
		if (c.piped || readsInput(c)) && slices.ContainsFunc(args, func(w shellWord) bool {
			return w.text == "-" || w.text == "/dev/stdin" || strings.HasPrefix(w.text, "/dev/fd/")
		}) {
			return RuleShellSyntax
		}
		for _, a := range args {
			if !strings.HasPrefix(a.text, "-") && releaseWord.MatchString(a.text) {
				return RulePublish
			}
		}
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
	// firstOperand marks the pattern or program: the value of a text
	// flag, or else the first operand. A file flag means the program
	// comes from that file, which stays a path to check.
	firstOperand := func(textFlags, fileFlags []string) {
		explicit := false
		for i := 0; i < len(args); i++ {
			switch {
			case slices.Contains(textFlags, args[i].text):
				text[i+1] = true
				explicit = true
				i++
			case slices.Contains(fileFlags, args[i].text):
				explicit = true
				i++
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
	switch name {
	case "echo", "printf":
		for i := range args {
			text[i] = true
		}
	case "grep", "egrep", "fgrep", "rg", "ag":
		firstOperand([]string{"-e", "--regexp"}, []string{"-f", "--file"})
	case "sed", "gsed":
		firstOperand([]string{"-e", "--expression"}, []string{"-f", "--file"})
	case "awk", "gawk", "mawk", "nawk":
		firstOperand(nil, []string{"-f", "--file"})
	case "jq":
		firstOperand(nil, []string{"-f", "--from-file"})
	case "git":
		for i, a := range args {
			if (a.text == "-m" || a.text == "--message") && i+1 < len(args) {
				text[i+1] = true
			}
		}
	}
	return text
}

// argValues are the parts of a word that may name a path: the word, or
// the value of a --name=value option. Other options name none.
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
	words := skipFlags(args, nil)
	if len(words) != 1 || words[0].glob || words[0].text == "-" {
		return RuleOutsideProject
	}
	abs, ok := g.path(words[0].text, *cwd)
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
		case (strings.HasPrefix(text, "-") || strings.HasPrefix(text, "+")) && strings.HasSuffix(text, "o"):
			i++
		case strings.HasPrefix(text, "-") || strings.HasPrefix(text, "+"):
		default:
			return RuleShellSyntax
		}
	}
	return ""
}

// hasOption reports whether args hold one of the options, alone or as
// --name=value.
func hasOption(args []shellWord, options ...string) bool {
	return slices.ContainsFunc(args, func(w shellWord) bool {
		name, _, _ := strings.Cut(w.text, "=")
		if slices.Contains(options, name) {
			return true
		}
		// A short option may carry its value: -Icmd.
		for _, o := range options {
			if len(o) == 2 && o[0] == '-' && o != "-z" && o != "-P" && o != "-H" && strings.HasPrefix(w.text, o) && !strings.HasPrefix(w.text, "--") {
				return true
			}
		}
		return false
	})
}
