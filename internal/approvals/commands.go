package approvals

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// shell approves a shell text only in a small grammar the gate can
// check in full: one command, or commands joined by && alone, where
// each may pipe into read-only filters. A cd may come first. Any other
// separator, a redirection other than 2>&1, or an assignment asks.
func (g *gate) shell(command string) string {
	commands, err := parseShell(command)
	if err != nil {
		return RuleShellSyntax
	}
	if len(commands) == 0 {
		return RuleInput
	}
	if commands[0].words != nil && commands[0].words[0].text == "cd" {
		for _, c := range commands[:len(commands)-1] {
			if c.sep != sepAnd {
				return RuleShellSyntax
			}
		}
	}
	cwd := g.cwd
	for i, c := range commands {
		if c.sep != sepAnd && c.sep != sepPipe && c.sep != "" {
			return RuleShellSyntax
		}
		if len(c.assigns) > 0 {
			return RuleShellSyntax
		}
		for _, r := range c.redirects {
			if !r.dup || r.raw != "2>&1" {
				return RuleShellSyntax
			}
		}
		if len(c.words) == 0 {
			return RuleShellSyntax
		}
		if namesPolicyWords(c.words) {
			return RulePolicy
		}
		var rule string
		switch {
		case c.words[0].text == "cd":
			if i != 0 || c.sep == sepPipe {
				return RuleShellSyntax
			}
			rule = g.changeDir(c.words[1:], &cwd)
		case c.piped:
			rule = g.filter(c.words, cwd)
		default:
			rule = g.program(c.words, cwd)
		}
		if rule != "" {
			return rule
		}
	}
	return ""
}

// namesPolicyWords reports a command that names the approvals policy
// once the shell has removed its quoting.
func namesPolicyWords(words []shellWord) bool {
	for i, w := range words {
		if namesPolicy(w.text) {
			return true
		}
		base := strings.ToLower(filepath.Base(strings.TrimRight(w.text, "/")))
		if base != "agnostic-ai" && base != "agnostic-ai.exe" {
			continue
		}
		for _, later := range words[i+1:] {
			if later.text == "approvals" || later.text == "hook" {
				return true
			}
		}
	}
	return false
}

// changeDir allows cd to one relative folder inside the project.
func (g *gate) changeDir(args []shellWord, cwd *string) string {
	if len(args) != 1 || strings.HasPrefix(args[0].text, "-") || pathLike(args[0].text) || args[0].tilde {
		return RuleOutsideProject
	}
	abs, ok := g.path(args[0].text, *cwd)
	if !ok || !g.inside(abs) {
		return RuleOutsideProject
	}
	if g.protected(abs) || g.secret(abs) {
		return RuleProtectedPath
	}
	if rule := g.specRule(abs); rule != "" {
		return rule
	}
	*cwd = abs
	return ""
}

// rtkPassThrough are the rtk verbs that run the program of the same
// name with the rest of the line.
var rtkPassThrough = []string{"git", "go", "cargo", "npm", "pytest"}

// program checks the first command of a pipeline against the table.
func (g *gate) program(words []shellWord, cwd string) string {
	if words[0].text == "rtk" && len(words) > 1 && slices.Contains(rtkPassThrough, words[1].text) {
		words = words[1:]
	}
	name := words[0].text
	if form, ok := g.lookup(name, words[1:]); ok {
		return g.check(form, words[1:], cwd)
	}
	return deniedRule(name, words[1:])
}

// filter checks a later stage of a pipeline: only filters that read
// their input and print.
func (g *gate) filter(words []shellWord, cwd string) string {
	form, ok := filters[words[0].text]
	if !ok {
		return RuleShellSyntax
	}
	return g.check(form, words[1:], cwd)
}

// lookup finds the form for a program and, for a program with verbs,
// its verb. ok is false when the table has no entry.
func (g *gate) lookup(name string, args []shellWord) (form, bool) {
	if f, ok := viewers[name]; ok {
		return f, true
	}
	verbs, ok := programs[name]
	if !ok || len(args) == 0 {
		return form{}, false
	}
	f, ok := verbs[args[0].text]
	if !ok {
		return form{}, false
	}
	f.skip = 1
	return f, true
}

// deniedRule names why a program outside the table asks.
func deniedRule(name string, args []shellWord) string {
	lower := strings.ToLower(filepath.Base(name))
	verb := ""
	if len(args) > 0 {
		verb = args[0].text
	}
	switch {
	case deleteCommands[lower] || strings.HasPrefix(lower, "mkfs"):
		return RuleDelete
	case privilegeCommands[lower]:
		return RulePrivilege
	case secretCommands[lower]:
		return RuleSecret
	case networkCommands[lower]:
		return RuleNetwork
	case publishCommands[lower] || releaseWord.MatchString(lower):
		return RulePublish
	case wrappers[lower] || interpreterName.MatchString(lower):
		return RuleShellSyntax
	}
	if verbRules, ok := namedVerbs[lower]; ok {
		if rule, ok := verbRules[verb]; ok {
			return rule
		}
	}
	if releaseWord.MatchString(verb) {
		return RulePublish
	}
	return RuleUnknownCommand
}

// check reads args against a form: each option must be listed, each
// operand is read as the form says, and every value is checked as a
// path unless the option marks it as text.
func (g *gate) check(f form, args []shellWord, cwd string) string {
	args = args[f.skip:]
	operandIndex := 0
	afterDashes := false
	hasPatternOption := false
	for _, a := range args {
		if o, ok := f.opts[a.text]; ok && o.pattern {
			hasPatternOption = true
		}
	}
	var operands []string
	for i := 0; i < len(args); i++ {
		text := args[i].text
		if !afterDashes && text == "--" && f.dashes {
			afterDashes = true
			continue
		}
		if !afterDashes && strings.HasPrefix(text, "-") && text != "-" {
			value, isText, consumed, ok := f.option(text, args[i+1:])
			if !ok {
				return RuleUnknownCommand
			}
			i += consumed
			if value != nil && !isText {
				if rule := g.pathValue(*value, cwd); rule != "" {
					return rule
				}
			}
			continue
		}
		kind := f.operand
		if f.pattern && operandIndex == 0 && !hasPatternOption {
			kind = operandText
		}
		operandIndex++
		operands = append(operands, text)
		if rule := g.operandRule(kind, args[i], cwd); rule != "" {
			return rule
		}
	}
	if f.requires != "" && !slices.ContainsFunc(args, func(w shellWord) bool { return w.text == f.requires }) {
		return RuleUnknownCommand
	}
	if len(operands) < f.minOperands || f.maxOperands >= 0 && len(operands) > f.maxOperands {
		return RuleUnknownCommand
	}
	if f.verify != nil {
		return f.verify(operands)
	}
	return ""
}

// option reads one option word. value is the option's value, if it
// takes one; consumed counts the words after it that the value used.
func (f form) option(text string, rest []shellWord) (value *string, isText bool, consumed int, ok bool) {
	if o, found := f.opts[text]; found {
		if o.kind&flag != 0 {
			return nil, false, 0, true
		}
		if o.kind&separate != 0 && len(rest) > 0 {
			return &rest[0].text, o.text, 1, true
		}
		return nil, false, 0, false
	}
	if name, v, hasEq := strings.Cut(text, "="); hasEq {
		if o, found := f.opts[name]; found && o.kind&glued != 0 {
			return &v, o.text, 0, true
		}
		return nil, false, 0, false
	}
	if !f.wordOptions && !strings.HasPrefix(text, "--") && len(text) > 2 {
		if o, found := f.opts[text[:2]]; found && o.kind&glued != 0 {
			v := text[2:]
			return &v, o.text, 0, true
		}
	}
	if f.digits && regexp.MustCompile(`^-[0-9]+$`).MatchString(text) {
		return nil, false, 0, true
	}
	if f.bundle != "" && !strings.HasPrefix(text, "--") {
		for _, letter := range text[1:] {
			if !strings.ContainsRune(f.bundle, letter) {
				return nil, false, 0, false
			}
		}
		return nil, false, 0, true
	}
	return nil, false, 0, false
}

func (g *gate) operandRule(kind operandKind, w shellWord, cwd string) string {
	switch kind {
	case operandText:
		return ""
	case operandName:
		if !plainName.MatchString(w.text) || strings.Contains(w.text, "..") {
			return RuleUnknownCommand
		}
		return ""
	case operandPackage:
		pkg := strings.TrimSuffix(strings.TrimSuffix(w.text, "/..."), "...")
		if w.text != "." && !strings.HasPrefix(w.text, "./") {
			return RuleUnknownCommand
		}
		if pkg == "" || pkg == "." {
			return ""
		}
		return g.pathValue(pkg, cwd)
	case operandNone:
		return RuleUnknownCommand
	}
	// git reads :path as a pathspec, where :(magic) and :stage: change
	// its meaning, and rev:path as a file of a commit.
	if rest, ok := strings.CutPrefix(w.text, ":"); ok {
		if strings.HasPrefix(rest, "(") || strings.Contains(rest, ":") {
			return RuleUnknownCommand
		}
		return g.pathValue(rest, cwd)
	}
	if i := strings.LastIndex(w.text, ":"); i >= 0 {
		if rule := g.pathValue(w.text[i+1:], cwd); rule != "" {
			return rule
		}
	}
	return g.pathValue(w.text, cwd)
}

// plainName is a branch, remote or script name.
var plainName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// pathValue asks for a word that, read as a path, leaves the project,
// is under a protected path, or names a credential.
func (g *gate) pathValue(value, cwd string) string {
	if value == "" {
		return ""
	}
	abs, ok := g.path(value, cwd)
	if !ok {
		return RuleOutsideProject
	}
	if g.secret(abs) {
		return RuleSecret
	}
	if !g.inside(abs) {
		return RuleOutsideProject
	}
	if g.protected(abs) {
		return RuleProtectedPath
	}
	return g.specRule(abs)
}

func pathLike(word string) bool {
	return strings.HasPrefix(word, "/") || strings.HasPrefix(word, "~") || word == ".." ||
		strings.HasPrefix(word, "../") || strings.Contains(word, "/../") || strings.HasSuffix(word, "/..") ||
		strings.HasPrefix(word, `\\`) || isWindowsPath(word)
}

func set(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// The sets below only name the rule a program outside the table asks
// with; every program outside the table asks.
var (
	deleteCommands = set("rm", "rmdir", "shred", "dd", "truncate", "unlink", "wipefs", "srm", "fdisk", "sfdisk",
		"parted", "trash", "grm", "grmdir", "gshred", "gdd", "gtruncate", "gunlink", "mv", "cp", "install", "ln")

	privilegeCommands = set("sudo", "sudoedit", "su", "doas", "pkexec", "runuser", "chown", "chgrp", "chmod", "kill",
		"pkill", "killall", "launchctl", "systemctl", "service", "crontab", "at", "batch", "shutdown", "reboot",
		"halt", "poweroff", "mount", "umount", "diskutil", "csrutil", "spctl", "xattr", "chflags", "defaults",
		"chroot", "unshare", "nsenter", "setfacl", "visudo", "passwd", "dscl", "usermod", "useradd", "userdel",
		"groupadd", "sysctl")

	secretCommands = set("aws-vault", "dotenv", "envchain", "security", "pass", "gopass", "op", "printenv",
		"ssh-add", "ssh-agent", "keychain", "gpg", "gpg2", "secret-tool", "vault", "bw", "lpass", "doppler",
		"chamber", "sops", "age", "ps")

	networkCommands = set("curl", "wget", "ssh", "scp", "sftp", "nc", "ncat", "netcat", "telnet", "ftp", "socat",
		"http", "https", "xh", "aria2c", "open", "xdg-open", "npx", "pnpx", "bunx", "uvx", "pipx", "ngrok",
		"rclone", "apt", "apt-get", "yum", "dnf", "pacman", "apk", "brew", "port", "rustup", "pip", "pip3")

	publishCommands = set("twine", "aws", "gcloud", "gsutil", "az", "fly", "flyctl", "vercel", "netlify",
		"heroku", "firebase", "wrangler", "doctl", "kubectl", "helm", "terraform", "tofu", "docker", "podman",
		"goreleaser", "glab")

	// wrappers run another program or shell text.
	wrappers = set("env", "nohup", "nice", "timeout", "time", "rtk", "xargs", "command", "builtin", "exec",
		"eval", "source", ".", "sh", "bash", "zsh", "dash", "fish", "ksh", "!", "watch", "parallel", "script",
		"flock", "setsid", "stdbuf", "unbuffer", "caffeinate", "arch", "xcrun", "sudo")

	interpreterName = regexp.MustCompile(`^(python[0-9.]*|node|nodejs|ruby|perl|php|lua|deno|bun|osascript|awk|gawk|mawk|nawk|tclsh|Rscript|java|pwsh|powershell)$`)

	releaseWord = regexp.MustCompile(`(?i)release|publish|deploy`)

	// namedVerbs name the rule for verbs of table programs that the
	// table leaves out.
	namedVerbs = map[string]map[string]string{
		"git":   {"push": RulePublish, "clean": RuleDelete, "rm": RuleDelete, "reset": RuleDelete, "restore": RuleDelete, "checkout": RuleDelete, "clone": RuleNetwork},
		"go":    {"install": RuleNetwork, "get": RuleNetwork},
		"cargo": {"publish": RulePublish, "install": RuleNetwork, "add": RuleNetwork},
		"npm":   {"publish": RulePublish, "install": RuleNetwork, "i": RuleNetwork, "ci": RuleNetwork, "add": RuleNetwork, "exec": RuleNetwork},
		"pnpm":  {"publish": RulePublish, "install": RuleNetwork, "i": RuleNetwork, "add": RuleNetwork, "dlx": RuleNetwork, "exec": RuleNetwork},
		"yarn":  {"publish": RulePublish, "install": RuleNetwork, "add": RuleNetwork, "dlx": RuleNetwork, "exec": RuleNetwork},
	}
)
