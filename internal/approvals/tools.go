package approvals

import (
	"slices"
	"strings"
)

func rsyncRule(args []shellWord) string {
	if flagged(args, []string{"--delete", "--delete-before", "--delete-during", "--delete-delay", "--delete-after", "--delete-excluded", "--remove-source-files"}, "") {
		return RuleDelete
	}
	if flagged(args, []string{"--rsh", "--rsync-path"}, "e") {
		return RuleNetwork
	}
	for _, a := range args {
		if !strings.HasPrefix(a.text, "-") && (strings.Contains(a.text, "://") || hostPath(a.text)) {
			return RuleNetwork
		}
	}
	return ""
}

// hostPath is host:path, the remote form scp and rsync take.
func hostPath(word string) bool {
	colon := strings.IndexRune(word, ':')
	if colon <= 0 {
		return false
	}
	slash := strings.IndexRune(word, '/')
	return slash < 0 || colon < slash
}

// subcommand is a tool's first word when it names a subcommand. A tool
// option before it may take a value the gate would read as the
// subcommand, so ok is false then.
func subcommand(args []shellWord) (verb string, rest []shellWord, ok bool) {
	if len(args) == 0 || strings.HasPrefix(args[0].text, "-") || strings.HasPrefix(args[0].text, "+") {
		return "", nil, false
	}
	return args[0].text, args[1:], true
}

// isVersionOrHelp is a tool run only to print its version or help.
func isVersionOrHelp(args []shellWord) bool {
	return len(args) == 1 && slices.Contains([]string{"--version", "-v", "-V", "version", "--help", "-h", "help"}, args[0].text)
}

// allowVerbs allows the listed subcommands only; anything else asks as
// a publish, the usual harm of a write through these tools.
func allowVerbs(args []shellWord, allowed ...string) string {
	if isVersionOrHelp(args) {
		return ""
	}
	verb, rest, ok := subcommand(args)
	if !ok {
		return RulePublish
	}
	if slices.Contains(allowed, verb) {
		return ""
	}
	next, _, _ := subcommand(rest)
	if verb == "config" && slices.Contains([]string{"view", "get-contexts", "current-context"}, next) {
		return ""
	}
	if verb == "auth" && next == "can-i" {
		return ""
	}
	return RulePublish
}

// gitGlobalFlags are the git options before a subcommand the gate
// reads; -C takes a folder, which the argument check reads as a path.
var gitGlobalFlags = []string{"--no-pager", "-P", "--no-optional-locks", "--no-replace-objects", "--literal-pathspecs"}

// gitRules are the subcommands git may run, with the long options and
// short letters that make each one ask, and the rule they ask under.
type gitCheck struct {
	longs  []string
	shorts string
	rule   string
}

var gitRules = map[string][]gitCheck{
	"status": nil, "diff": nil, "log": nil, "show": nil, "blame": nil, "shortlog": nil, "describe": nil,
	"rev-parse": nil, "rev-list": nil, "ls-files": nil, "ls-tree": nil, "cat-file": nil, "show-ref": nil,
	"for-each-ref": nil, "merge-base": nil, "check-ignore": nil, "range-diff": nil, "whatchanged": nil,
	"version": nil, "help": nil, "add": nil,
	"init": {{longs: []string{"--template", "--separate-git-dir"}, rule: RuleShellSyntax}}, "mv": nil, "merge": nil, "cherry-pick": nil,
	"revert": nil, "format-patch": nil, "name-rev": nil,
	"grep":   {{longs: []string{"--open-files-in-pager"}, shorts: "O", rule: RuleShellSyntax}},
	"commit": {{longs: []string{"--amend"}, rule: RulePublish}},
	"fetch":  {{longs: []string{"--upload-pack"}, rule: RuleShellSyntax}},
	"pull":   {{longs: []string{"--upload-pack"}, rule: RuleShellSyntax}},
	"reset":  {{longs: []string{"--hard", "--merge", "--keep"}, rule: RuleDelete}},
	"switch": {{longs: []string{"--discard-changes", "--force", "--force-create"}, shorts: "fC", rule: RuleDelete}},
	"branch": {{longs: []string{"--delete", "--move", "--copy", "--force", "--edit-description"}, shorts: "dDmMcCf", rule: RuleDelete}},
	"tag":    {{longs: []string{"--delete", "--force"}, shorts: "df", rule: RuleDelete}},
	"rebase": {{longs: []string{"--exec", "--interactive", "--edit-todo"}, shorts: "xi", rule: RuleShellSyntax}},
	"stash":  nil,
	"bisect": nil,
}

// gitRule reads git's options, then a subcommand from gitRules. An
// unknown subcommand may be an alias that runs a shell command, so it
// asks. Long options are matched by prefix and short letters inside
// bundles, as git reads them.
func gitRule(args []shellWord) string {
	for len(args) > 0 && strings.HasPrefix(args[0].text, "-") {
		switch {
		case args[0].text == "-C" && len(args) > 1:
			args = args[2:]
		case slices.Contains(gitGlobalFlags, args[0].text):
			args = args[1:]
		default:
			return RuleShellSyntax
		}
	}
	if isVersionOrHelp(args) || len(args) == 0 {
		return ""
	}
	sub, rest := args[0].text, args[1:]
	for _, a := range rest {
		if strings.Contains(a.text, "ext::") || strings.HasPrefix(a.text, "fd::") {
			return RuleShellSyntax
		}
	}
	switch sub {
	case "push", "send-email", "request-pull", "svn", "p4":
		return RulePublish
	case "clean", "rm", "filter-branch", "filter-repo", "replace", "update-ref", "symbolic-ref", "gc", "prune":
		return RuleDelete
	case "credential", "credential-cache", "credential-store":
		return RuleSecret
	case "daemon", "instaweb", "clone", "ls-remote", "archive":
		return RuleNetwork
	case "checkout":
		return gitCheckout(rest)
	case "restore":
		if flagged(rest, []string{"--staged"}, "S") && !flagged(rest, []string{"--worktree", "--source", "--patch"}, "Wsp") {
			return ""
		}
		return RuleDelete
	case "stash":
		verb, _, _ := subcommand(rest)
		if slices.Contains([]string{"drop", "clear"}, verb) {
			return RuleDelete
		}
		return ""
	case "bisect":
		if verb, _, _ := subcommand(rest); verb == "run" {
			return RuleShellSyntax
		}
		return ""
	case "worktree":
		if verb, _, _ := subcommand(rest); verb == "list" || verb == "add" && !flagged(rest, []string{"--force"}, "fB") {
			return ""
		}
		return RuleDelete
	case "remote":
		if len(rest) == 0 || isVersionOrHelp(rest) || rest[0].text == "-v" || rest[0].text == "show" || rest[0].text == "get-url" {
			return ""
		}
		return RulePublish
	case "config":
		return gitConfig(rest)
	case "reflog":
		if verb, _, _ := subcommand(rest); len(rest) == 0 || verb == "show" {
			return ""
		}
		return RuleDelete
	}
	checks, known := gitRules[sub]
	if !known {
		return RuleShellSyntax
	}
	for _, check := range checks {
		if flagged(rest, check.longs, check.shorts) {
			return check.rule
		}
	}
	return ""
}

// gitCheckout allows only creating a branch with -b or -c: any other
// form may overwrite files with a branch's or commit's version.
func gitCheckout(rest []shellWord) string {
	if len(rest) == 0 || rest[0].text != "-b" && rest[0].text != "-c" {
		return RuleDelete
	}
	if flagged(rest, []string{"--force", "--ours", "--theirs", "--patch", "--pathspec-from-file", "--merge", "--overwrite-ignore"}, "fpmB") {
		return RuleDelete
	}
	operands := 0
	for _, a := range rest[1:] {
		if a.text == "--" {
			return RuleDelete
		}
		if !strings.HasPrefix(a.text, "-") {
			operands++
		}
	}
	if operands < 1 || operands > 2 {
		return RuleDelete
	}
	return ""
}

// gitConfig allows reading a value only; any write may make git run a
// command.
func gitConfig(rest []shellWord) string {
	read := false
	for _, a := range rest {
		text := a.text
		switch {
		case optionPrefixOf(strings.SplitN(text, "=", 2)[0], []string{"--get", "--get-all", "--get-regexp", "--list"}) || text == "-l":
			read = true
		case slices.Contains([]string{"--global", "--local", "--system", "--worktree", "--show-origin", "--show-scope", "--name-only", "--null", "-z", "--bool", "--int", "--path"}, text):
		case strings.HasPrefix(text, "-"):
			return RuleShellSyntax
		}
	}
	if len(rest) > 0 && (rest[0].text == "get" || rest[0].text == "list") {
		read = true
	}
	if !read {
		return RuleShellSyntax
	}
	return ""
}

// ghRule allows the gh commands that only read; any other asks as a
// publish, and auth commands ask as secrets.
func ghRule(args []shellWord) string {
	if isVersionOrHelp(args) {
		return ""
	}
	group, rest, ok := subcommand(args)
	if !ok {
		return RulePublish
	}
	switch group {
	case "auth", "secret", "ssh-key", "gpg-key", "variable":
		return RuleSecret
	case "status", "search":
		return ""
	case "api":
		if flagged(rest, []string{"--method", "--field", "--raw-field", "--input"}, "XfF") {
			return RulePublish
		}
		return ""
	case "codespace", "cs", "extension", "ext", "alias", "config", "browse":
		return RulePublish
	}
	verb, _, ok := subcommand(rest)
	if !ok || !slices.Contains([]string{"view", "list", "ls", "status", "diff", "checks", "watch"}, verb) {
		return RulePublish
	}
	if flagged(rest, []string{"--web"}, "w") {
		return RuleNetwork
	}
	return ""
}

func dockerRule(args []shellWord) string {
	if isVersionOrHelp(args) {
		return ""
	}
	verb, rest, ok := subcommand(args)
	if !ok {
		return RulePrivilege
	}
	switch verb {
	case "build", "images", "ps", "logs", "inspect", "info", "history":
		return ""
	case "pull", "search", "login":
		return RuleNetwork
	case "push":
		return RulePublish
	case "compose":
		if next, _, ok := subcommand(rest); ok && slices.Contains([]string{"ps", "logs", "config", "ls", "version", "images"}, next) {
			return ""
		}
	}
	return RulePrivilege
}

// packageRule allows a package manager to run tests, scripts and read
// commands. A script named like a release, or the env script, asks;
// installs ask as network and anything else as a publish.
func packageRule(name string, args []shellWord) string {
	if isVersionOrHelp(args) {
		return ""
	}
	verb, rest, ok := subcommand(args)
	if !ok {
		if len(args) == 0 && name != "yarn" && name != "bun" {
			return ""
		}
		return RuleNetwork
	}
	switch verb {
	case "test", "t", "tst", "lint", "build", "typecheck", "format", "fmt", "check", "start":
		return scriptRule(verb)
	case "run", "run-script":
		script, _, ok := subcommand(rest)
		if !ok {
			return RuleShellSyntax
		}
		return scriptRule(script)
	case "ls", "list", "ll", "la", "view", "info", "show", "explain", "why", "outdated":
		return ""
	case "install", "i", "in", "ins", "inst", "add", "ci", "update", "up", "upgrade", "dlx", "exec", "x", "create",
		"init", "link", "ln", "rebuild", "dedupe", "audit", "remove", "rm", "uninstall", "un", "unlink", "patch":
		return RuleNetwork
	case "login", "adduser", "logout", "token", "whoami", "config", "set", "get", "profile", "npm":
		return RuleSecret
	}
	return RulePublish
}

func scriptRule(script string) string {
	switch {
	case releaseWord.MatchString(script):
		return RulePublish
	case script == "env":
		return RuleSecret
	}
	return ""
}

func denoRule(args []shellWord) string {
	if isVersionOrHelp(args) {
		return ""
	}
	verb, rest, ok := subcommand(args)
	if !ok {
		return RuleShellSyntax
	}
	switch verb {
	case "test", "fmt", "lint", "check":
		if flagged(rest, []string{"--allow-all", "--allow-run", "--allow-net", "--allow-env", "--allow-write", "--allow-read"}, "A") {
			return RuleShellSyntax
		}
		return ""
	case "task":
		script, _, ok := subcommand(rest)
		if !ok {
			return RuleShellSyntax
		}
		return scriptRule(script)
	case "publish":
		return RulePublish
	}
	return RuleShellSyntax
}

// runOnlyRule allows uv, poetry, pdm, pipenv and bundle only to run a
// program, which the gate then checks; anything else may install.
func runOnlyRule(args []shellWord) string {
	if isVersionOrHelp(args) {
		return ""
	}
	verb, _, ok := subcommand(args)
	if ok && (verb == "run" || verb == "exec") {
		return ""
	}
	if ok && slices.Contains([]string{"publish", "upload"}, verb) {
		return RulePublish
	}
	return RuleNetwork
}

func goRule(args []shellWord) string {
	if isVersionOrHelp(args) {
		return ""
	}
	verb, rest, ok := subcommand(args)
	if !ok {
		return RuleShellSyntax
	}
	if flagged(rest, []string{"-exec", "-toolexec", "-vettool", "--exec", "--toolexec", "--vettool"}, "") ||
		slices.ContainsFunc(rest, func(w shellWord) bool {
			name, _, _ := strings.Cut(w.text, "=")
			return slices.Contains([]string{"-exec", "-toolexec", "-vettool", "-overlay", "-modfile", "-C"}, name)
		}) {
		return RuleShellSyntax
	}
	switch verb {
	case "build", "test", "vet", "fmt", "list", "doc", "version", "help", "generate", "mod", "work", "clean", "bug":
		if verb == "mod" || verb == "work" {
			if next, _, _ := subcommand(rest); next == "download" || next == "get" {
				return RuleNetwork
			}
		}
		return ""
	case "run", "tool":
		for _, w := range rest {
			if strings.Contains(w.text, "@") {
				return RuleNetwork
			}
		}
		return ""
	case "env":
		if flagged(rest, nil, "wu") {
			return RuleShellSyntax
		}
		return ""
	case "install", "get":
		return RuleNetwork
	}
	return RuleShellSyntax
}

func cargoRule(args []shellWord) string {
	if isVersionOrHelp(args) {
		return ""
	}
	verb, rest, ok := subcommand(args)
	if !ok {
		return RuleShellSyntax
	}
	if flagged(rest, []string{"--config"}, "Z") {
		return RuleShellSyntax
	}
	switch verb {
	case "build", "b", "check", "c", "test", "t", "fmt", "clippy", "doc", "d", "run", "r", "bench", "tree", "metadata", "clean":
		return ""
	case "install", "add", "uninstall", "remove", "update", "fetch":
		return RuleNetwork
	case "publish", "yank", "owner", "release":
		return RulePublish
	case "login", "logout":
		return RuleSecret
	}
	return RuleShellSyntax
}
