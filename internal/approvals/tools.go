package approvals

import (
	"slices"
	"strings"
)

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

func rsyncRule(args []shellWord) string {
	for _, a := range args {
		text := a.text
		switch {
		case strings.HasPrefix(text, "--delete") || strings.HasPrefix(text, "--remove-source-files"):
			return RuleDelete
		case text == "-e" || strings.HasPrefix(text, "--rsh"):
			return RuleNetwork
		case strings.HasPrefix(text, "-"):
		case strings.Contains(text, "://") || hostPath(text):
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

// verbs are the non-option words of args, in order.
func verbs(args []shellWord) []string {
	var out []string
	for _, a := range args {
		if !strings.HasPrefix(a.text, "-") {
			out = append(out, a.text)
		}
	}
	return out
}

func allowVerbs(args []shellWord, allowed ...string) string {
	v := verbs(args)
	if len(v) == 0 {
		if len(args) == 0 {
			return ""
		}
		if slices.ContainsFunc(args, func(w shellWord) bool {
			return w.text == "--version" || w.text == "--help" || w.text == "-h" || w.text == "version"
		}) {
			return ""
		}
		return RulePublish
	}
	if slices.Contains(allowed, v[0]) {
		return ""
	}
	if v[0] == "config" && len(v) > 1 && slices.Contains([]string{"view", "get-contexts", "current-context"}, v[1]) {
		return ""
	}
	if v[0] == "auth" && len(v) > 1 && v[1] == "can-i" {
		return ""
	}
	return RulePublish
}

var gitSubcommands = set("add", "am", "annotate", "apply", "archive", "bisect", "blame", "branch", "bundle",
	"cat-file", "check-attr", "check-ignore", "checkout", "cherry", "cherry-pick", "clean", "clone", "commit",
	"config", "count-objects", "credential", "describe", "diff", "diff-tree", "diff-index", "diff-files",
	"difftool", "fetch", "filter-branch", "for-each-ref", "format-patch", "fsck", "gc", "grep", "hash-object",
	"help", "hook", "init", "lfs", "log", "ls-files", "ls-remote", "ls-tree", "maintenance", "merge", "merge-base",
	"mergetool", "mv", "name-rev", "notes", "pull", "push", "range-diff", "rebase", "reflog", "remote", "repack",
	"replace", "reset", "restore", "rev-list", "rev-parse", "revert", "rm", "send-email", "shortlog", "show",
	"show-branch", "show-ref", "sparse-checkout", "stash", "status", "submodule", "switch", "symbolic-ref", "tag",
	"update-index", "update-ref", "var", "verify-commit", "verify-tag", "version", "whatchanged", "worktree",
	"daemon", "filter-repo", "svn", "instaweb", "request-pull", "citool", "gui", "gitk")

// gitRule reads git's own options, then its subcommand. An unknown
// subcommand may be an alias that runs a shell command, so it asks.
func gitRule(args []shellWord) string {
	i := 0
	for i < len(args) {
		text := args[i].text
		switch {
		case text == "-C":
			i += 2
			continue
		case text == "-c" || strings.HasPrefix(text, "--config-env") || strings.HasPrefix(text, "--exec-path") ||
			strings.HasPrefix(text, "--git-dir") || strings.HasPrefix(text, "--work-tree") || strings.HasPrefix(text, "--namespace") ||
			strings.HasPrefix(text, "--super-prefix") || text == "--list-cmds":
			return RuleShellSyntax
		case strings.HasPrefix(text, "-"):
			i++
			continue
		}
		break
	}
	if i >= len(args) {
		return ""
	}
	sub := args[i].text
	rest := args[i+1:]
	has := func(flags ...string) bool {
		return slices.ContainsFunc(rest, func(w shellWord) bool {
			for _, f := range flags {
				if w.text == f || strings.HasPrefix(f, "--") && strings.HasPrefix(w.text, f+"=") {
					return true
				}
			}
			return false
		})
	}
	shortHas := func(letter string) bool {
		return slices.ContainsFunc(rest, func(w shellWord) bool {
			return strings.HasPrefix(w.text, "-") && !strings.HasPrefix(w.text, "--") && strings.Contains(w.text, letter)
		})
	}
	restVerbs := verbs(rest)
	if !gitSubcommands[sub] {
		return RuleShellSyntax
	}
	switch sub {
	case "push", "send-email", "request-pull", "svn":
		return RulePublish
	case "daemon", "instaweb":
		return RuleNetwork
	case "credential":
		return RuleSecret
	case "remote":
		if len(restVerbs) == 0 || slices.Contains([]string{"show", "get-url"}, restVerbs[0]) {
			return ""
		}
		return RulePublish
	case "archive":
		if has("--remote", "--exec") {
			return RuleNetwork
		}
	case "commit":
		if has("--amend") {
			return RulePublish
		}
	case "tag":
		if has("-d", "--delete") {
			return RuleDelete
		}
	case "reset":
		if has("--hard", "--merge", "--keep") {
			return RuleDelete
		}
	case "clean", "rm", "filter-branch", "filter-repo", "replace", "update-ref", "symbolic-ref":
		return RuleDelete
	case "checkout":
		if has("--", ".", "-f", "--force", "--ours", "--theirs", "-p", "--patch", "--pathspec-from-file") || shortHas("f") {
			return RuleDelete
		}
	case "restore":
		if has("--staged", "-S") && !has("--worktree", "-W") {
			return ""
		}
		return RuleDelete
	case "switch":
		if has("--discard-changes", "-f", "--force", "-C", "--force-create") {
			return RuleDelete
		}
	case "branch":
		if has("-D", "-M", "-C") || has("-d", "--delete", "-m", "--move", "-c", "--copy") && has("-f", "--force") {
			return RuleDelete
		}
	case "stash":
		if len(restVerbs) > 0 && slices.Contains([]string{"drop", "clear"}, restVerbs[0]) {
			return RuleDelete
		}
	case "worktree":
		if len(restVerbs) > 0 && slices.Contains([]string{"remove", "prune"}, restVerbs[0]) {
			return RuleDelete
		}
	case "gc":
		if has("--prune", "--aggressive") {
			return RuleDelete
		}
	case "reflog":
		if len(restVerbs) > 0 && slices.Contains([]string{"expire", "delete", "drop"}, restVerbs[0]) {
			return RuleDelete
		}
	case "config":
		readFlags := []string{"--get", "--get-all", "--get-regexp", "--get-urlmatch", "--list", "-l", "--show-origin", "--show-scope", "--name-only", "--null", "-z", "--global", "--local", "--system", "--worktree", "--type", "--bool", "--int", "--path"}
		isRead := has("--get", "--get-all", "--get-regexp", "--get-urlmatch", "--list", "-l") ||
			len(restVerbs) > 0 && slices.Contains([]string{"get", "list"}, restVerbs[0])
		for _, w := range rest {
			if strings.HasPrefix(w.text, "-") && !slices.Contains(readFlags, strings.SplitN(w.text, "=", 2)[0]) {
				return RuleShellSyntax
			}
		}
		if !isRead {
			return RuleShellSyntax
		}
	case "submodule":
		if len(restVerbs) > 0 && restVerbs[0] == "foreach" {
			return RuleShellSyntax
		}
	case "bisect":
		if len(restVerbs) > 0 && restVerbs[0] == "run" {
			return RuleShellSyntax
		}
	case "rebase":
		if has("--exec", "-x", "-i", "--interactive") || shortHas("x") || shortHas("i") {
			return RuleShellSyntax
		}
	case "difftool", "mergetool", "hook", "citool", "gui", "gitk", "apply", "am":
		return RuleShellSyntax
	}
	return ""
}

// ghRule allows the gh commands that only read; any other asks as a
// publish, and auth commands ask as secrets.
func ghRule(args []shellWord) string {
	v := verbs(args)
	if len(v) == 0 {
		return ""
	}
	switch v[0] {
	case "auth", "secret", "ssh-key", "gpg-key", "token":
		return RuleSecret
	case "status", "search", "version", "help":
		return ""
	case "api":
		for i, a := range args {
			text := a.text
			switch {
			case text == "-X" || text == "--method":
				if i+1 >= len(args) || !strings.EqualFold(args[i+1].text, "GET") {
					return RulePublish
				}
			case strings.HasPrefix(text, "--method=") && !strings.EqualFold(strings.TrimPrefix(text, "--method="), "GET"),
				strings.HasPrefix(text, "-X") && len(text) > 2 && !strings.EqualFold(text[2:], "GET"),
				text == "-f", text == "-F", strings.HasPrefix(text, "--field"), strings.HasPrefix(text, "--raw-field"),
				strings.HasPrefix(text, "--input"), strings.HasPrefix(text, "-f"), strings.HasPrefix(text, "-F"):
				return RulePublish
			}
		}
		return ""
	}
	if len(v) > 1 && slices.Contains([]string{"view", "list", "ls", "status", "diff", "checks", "watch"}, v[1]) &&
		!slices.Contains([]string{"codespace", "extension", "alias", "config", "cs"}, v[0]) {
		if slices.ContainsFunc(args, func(w shellWord) bool { return w.text == "--web" || w.text == "-w" }) {
			return RuleNetwork
		}
		return ""
	}
	return RulePublish
}

func dockerRule(args []shellWord) string {
	v := verbs(args)
	if len(v) == 0 {
		return ""
	}
	switch v[0] {
	case "build", "images", "ps", "logs", "inspect", "version", "info", "history":
		return ""
	case "pull", "search", "login":
		return RuleNetwork
	case "push":
		return RulePublish
	case "compose":
		if len(v) > 1 && slices.Contains([]string{"ps", "logs", "config", "ls", "version", "images"}, v[1]) {
			return ""
		}
	}
	return RulePrivilege
}

func packageRule(name string, args []shellWord) string {
	v := verbs(args)
	if len(v) == 0 {
		if name == "yarn" {
			return RuleNetwork
		}
		return ""
	}
	switch v[0] {
	case "publish", "unpublish", "deprecate", "dist-tag", "owner", "access", "team", "star", "unstar", "pack":
		return RulePublish
	case "install", "i", "in", "ins", "inst", "add", "ci", "update", "up", "upgrade", "dlx", "exec", "x", "create", "init", "link", "ln", "rebuild", "dedupe", "audit", "outdated", "remove", "rm", "uninstall", "un", "unlink", "r", "patch", "global":
		return RuleNetwork
	case "login", "adduser", "add-user", "logout", "token", "whoami", "config", "set", "get", "profile":
		return RuleSecret
	case "ls", "list", "ll", "la", "view", "info", "show", "explain", "why", "help", "bin", "root", "prefix", "fund", "query":
		return ""
	case "run", "run-script", "rum", "urn", "test", "t", "tst", "start", "stop", "restart", "build", "lint", "format", "fmt", "typecheck", "check":
		for _, w := range v {
			if releaseWord.MatchString(w) {
				return RulePublish
			}
		}
		return ""
	}
	if name == "yarn" || name == "bun" || name == "pnpm" {
		// yarn <script> runs a package script by name.
		if releaseWord.MatchString(v[0]) {
			return RulePublish
		}
		return ""
	}
	return RulePublish
}

func pythonPackageRule(args []shellWord) string {
	v := verbs(args)
	if len(v) == 0 {
		return ""
	}
	if slices.Contains([]string{"publish", "upload"}, v[0]) {
		return RulePublish
	}
	if v[0] == "run" && len(v) > 1 && !strings.Contains(v[1], "://") {
		return ""
	}
	if slices.Contains([]string{"list", "show", "freeze", "check", "--version", "version", "lock", "tree", "env", "info", "search"}, v[0]) && len(v) == 1 {
		return ""
	}
	return RuleNetwork
}

func goRule(args []shellWord) string {
	if hasOption(args, "-exec", "-toolexec", "-vettool", "--exec", "--toolexec", "--vettool") {
		return RuleShellSyntax
	}
	v := verbs(args)
	if len(v) == 0 {
		return ""
	}
	switch v[0] {
	case "install", "get":
		return RuleNetwork
	case "run", "tool":
		for _, w := range v[1:] {
			if strings.Contains(w, "@") {
				return RuleNetwork
			}
		}
	case "env":
		if slices.ContainsFunc(args, func(w shellWord) bool { return w.text == "-w" || w.text == "-u" }) {
			return RuleShellSyntax
		}
	}
	return ""
}

func cargoRule(args []shellWord) string {
	v := verbs(args)
	if len(v) == 0 {
		return ""
	}
	switch v[0] {
	case "install", "add", "uninstall", "remove":
		return RuleNetwork
	case "publish", "yank", "owner", "release":
		return RulePublish
	case "login", "logout":
		return RuleSecret
	}
	return ""
}

func toolchainRule(name string, args []shellWord) string {
	v := verbs(args)
	if len(v) == 0 {
		return ""
	}
	switch v[0] {
	case "push", "publish", "release", "owner", "yank", "hex.publish":
		return RulePublish
	case "signin", "signout", "login", "logout":
		return RuleSecret
	case "install", "update", "upgrade", "add", "require", "reinstall", "tap", "uninstall", "remove", "get", "deps.get", "restore", "tool", "fetch", "outdated", "lock":
		return RuleNetwork
	}
	if name == "brew" && !slices.Contains([]string{"list", "info", "search", "doctor", "config", "deps", "leaves", "uses", "desc", "--version", "--prefix", "--cellar", "--repository"}, v[0]) {
		return RuleNetwork
	}
	return ""
}
