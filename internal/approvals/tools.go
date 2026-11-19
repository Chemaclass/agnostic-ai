package approvals

import (
	"regexp"
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

// options is the set of options a command may take: long options by
// their full name only, short letters that stand alone in a bundle, and
// short letters whose value is the rest of the bundle or the next word.
// Long options in withValue take the next word unless written --name=value.
type options struct {
	longs      []string
	withValue  []string
	shorts     string
	valueShort string
}

// check returns false at the first option not in o. Every word after
// "--" is an operand.
func (o options) check(args []shellWord) bool {
	for i := 0; i < len(args); i++ {
		text := args[i].text
		switch {
		case text == "--":
			return true
		case strings.HasPrefix(text, "--"):
			name, _, hasValue := strings.Cut(text, "=")
			switch {
			case slices.Contains(o.longs, text), slices.Contains(o.longs, name) && hasValue:
			case slices.Contains(o.withValue, name):
				if !hasValue {
					i++
				}
			default:
				return false
			}
		case strings.HasPrefix(text, "-") && len(text) > 1:
			for j, letter := range text[1:] {
				if letter >= '0' && letter <= '9' {
					continue
				}
				if strings.ContainsRune(o.valueShort, letter) {
					if j == len(text)-2 {
						i++
					}
					break
				}
				if !strings.ContainsRune(o.shorts, letter) {
					return false
				}
			}
		}
	}
	return true
}

// gitGlobalFlags are the git options before a subcommand the gate
// reads; -C takes a folder, which the argument check reads as a path.
var gitGlobalFlags = []string{"--no-pager", "-P", "--no-optional-locks", "--no-replace-objects", "--literal-pathspecs"}

var (
	gitLogOptions = options{
		longs: []string{"--oneline", "--graph", "--decorate", "--no-decorate", "--all", "--stat", "--shortstat", "--numstat",
			"--patch", "--no-patch", "--name-only", "--name-status", "--follow", "--no-merges", "--merges", "--first-parent",
			"--abbrev-commit", "--left-right", "--cherry-pick", "--reverse", "--color", "--no-color", "--invert-grep",
			"--all-match", "--regexp-ignore-case", "--source", "--boundary", "--topo-order", "--date-order", "--summary",
			"--simplify-by-decoration", "--show-signature", "--full-history", "--no-abbrev-commit", "--parents", "--quiet",
			"--relative-date", "--word-diff", "--raw", "--no-ext-diff", "--no-textconv"},
		withValue: []string{"--format", "--pretty", "--author", "--committer", "--since", "--until", "--after", "--before",
			"--grep", "--max-count", "--skip", "--date", "--abbrev", "--diff-filter", "--branches", "--tags", "--remotes", "--exclude"},
		shorts:     "psiqz",
		valueShort: "nL",
	}
	gitDiffOptions = options{
		longs: []string{"--stat", "--shortstat", "--numstat", "--name-only", "--name-status", "--cached", "--staged",
			"--color", "--no-color", "--word-diff", "--patch", "--no-patch", "--exit-code", "--quiet", "--check",
			"--ignore-all-space", "--ignore-space-change", "--ignore-blank-lines", "--minimal", "--relative", "--merge-base",
			"--summary", "--raw", "--color-words", "--no-ext-diff", "--no-textconv", "--text", "--find-renames",
			"--dirstat", "--no-renames", "--histogram", "--patience", "--full-index", "--binary", "--compact-summary"},
		withValue:  []string{"--unified", "--diff-filter", "--stat-width", "--word-diff-regex", "--src-prefix", "--dst-prefix", "--abbrev"},
		shorts:     "pswbRaz",
		valueShort: "UMC",
	}
	gitOptions = map[string]options{
		"status": {longs: []string{"--short", "--branch", "--porcelain", "--long", "--ignored", "--column", "--no-column",
			"--ahead-behind", "--no-ahead-behind", "--renames", "--no-renames", "--show-stash", "--verbose"},
			withValue: []string{"--untracked-files"}, shorts: "sbvz", valueShort: "u"},
		"log":         gitLogOptions,
		"whatchanged": gitLogOptions,
		"shortlog":    {longs: []string{"--summary", "--numbered", "--email", "--all"}, withValue: []string{"--format", "--since", "--until"}, shorts: "sne"},
		"show":        gitLogOptions,
		"diff":        gitDiffOptions,
		"blame": {longs: []string{"--porcelain", "--line-porcelain", "--show-email", "--show-number", "--show-name", "--root", "--show-stats"},
			withValue: []string{"--date", "--abbrev"}, shorts: "wesltnfcpb", valueShort: "LCM"},
		"describe": {longs: []string{"--tags", "--always", "--long", "--dirty", "--exact-match", "--contains", "--all", "--first-parent"},
			withValue: []string{"--abbrev", "--match", "--exclude", "--candidates"}},
		"rev-parse": {longs: []string{"--abbrev-ref", "--short", "--verify", "--show-toplevel", "--git-dir", "--is-inside-work-tree",
			"--symbolic-full-name", "--show-prefix", "--absolute-git-dir", "--git-common-dir", "--show-cdup", "--quiet",
			"--is-shallow-repository", "--sq", "--is-inside-git-dir", "--is-bare-repository", "--show-superproject-working-tree"}, shorts: "q"},
		"rev-list":     {longs: []string{"--count", "--all", "--left-right", "--no-merges", "--reverse", "--first-parent", "--objects"}, withValue: []string{"--max-count", "--since", "--until"}, valueShort: "n"},
		"ls-files":     {longs: []string{"--others", "--exclude-standard", "--cached", "--modified", "--deleted", "--stage", "--ignored", "--directory", "--full-name", "--error-unmatch", "--unmerged", "--killed"}, shorts: "ocmdsikutz"},
		"ls-tree":      {longs: []string{"--name-only", "--full-tree", "--name-status", "--long"}, shorts: "rtdlz"},
		"cat-file":     {longs: []string{"--batch-check"}, shorts: "tspe"},
		"show-ref":     {longs: []string{"--heads", "--tags", "--verify", "--quiet", "--hash"}, shorts: "qsd"},
		"for-each-ref": {withValue: []string{"--format", "--sort", "--count", "--contains", "--merged", "--no-merged", "--points-at"}},
		"merge-base":   {longs: []string{"--is-ancestor", "--all", "--fork-point", "--octopus"}, shorts: "a"},
		"check-ignore": {longs: []string{"--verbose", "--quiet", "--non-matching", "--no-index"}, shorts: "vqn"},
		"range-diff":   {longs: []string{"--no-color", "--color"}},
		"name-rev":     {longs: []string{"--name-only", "--tags", "--always"}},
		"version":      {},
		"help":         {},
		"add":          {longs: []string{"--all", "--update", "--verbose", "--dry-run", "--intent-to-add", "--force", "--ignore-errors", "--renormalize", "--no-all", "--ignore-removal"}, shorts: "AuvnNf"},
		"mv":           {longs: []string{"--force", "--verbose", "--dry-run"}, shorts: "fknv"},
		"merge": {longs: []string{"--no-ff", "--ff-only", "--ff", "--squash", "--no-commit", "--commit", "--abort", "--continue",
			"--no-edit", "--quit", "--stat", "--no-stat", "--signoff", "--allow-unrelated-histories", "--no-verify", "--autostash", "--quiet"},
			withValue: []string{"--message", "--strategy", "--strategy-option"}, shorts: "nqv", valueShort: "msX"},
		"cherry-pick": {longs: []string{"--continue", "--abort", "--skip", "--quit", "--no-commit", "--signoff", "--ff", "--allow-empty", "--keep-redundant-commits"},
			withValue: []string{"--mainline", "--strategy", "--strategy-option"}, shorts: "nxs", valueShort: "mX"},
		"revert":       {longs: []string{"--no-commit", "--no-edit", "--continue", "--abort", "--skip", "--quit", "--signoff"}, withValue: []string{"--mainline"}, shorts: "ns", valueShort: "m"},
		"format-patch": {longs: []string{"--stdout", "--numbered", "--cover-letter", "--root", "--no-stat"}, withValue: []string{"--output-directory", "--subject-prefix", "--reroll-count", "--base"}, shorts: "n", valueShort: "ov"},
		"init":         {longs: []string{"--quiet"}, withValue: []string{"--initial-branch"}, shorts: "q", valueShort: "b"},
		"commit": {longs: []string{"--all", "--signoff", "--no-verify", "--allow-empty", "--allow-empty-message", "--quiet", "--verbose",
			"--no-edit", "--no-gpg-sign", "--dry-run", "--short", "--porcelain", "--no-signoff", "--status", "--no-status"},
			withValue: []string{"--message", "--file", "--fixup", "--squash", "--author", "--date", "--reuse-message", "--cleanup", "--trailer", "--gpg-sign"},
			shorts:    "asnqv", valueShort: "mFCS"},
		"fetch": {longs: []string{"--all", "--prune", "--tags", "--no-tags", "--unshallow", "--dry-run", "--quiet", "--verbose", "--force", "--prune-tags", "--atomic", "--no-recurse-submodules"},
			withValue: []string{"--depth", "--deepen", "--shallow-since", "--jobs"}, shorts: "ptqvfa", valueShort: "j"},
		"pull": {longs: []string{"--rebase", "--no-rebase", "--ff-only", "--ff", "--no-ff", "--quiet", "--verbose", "--autostash", "--no-autostash",
			"--no-edit", "--prune", "--tags", "--all", "--squash", "--no-commit", "--no-recurse-submodules"},
			withValue: []string{"--depth"}, shorts: "rqvp"},
		"reset":  {longs: []string{"--soft", "--mixed", "--quiet", "--no-refresh"}, shorts: "qN"},
		"switch": {longs: []string{"--create", "--detach", "--no-track", "--track", "--guess", "--no-guess", "--quiet", "--orphan", "--merge"}, shorts: "dtqm", valueShort: "c"},
		"branch": {longs: []string{"--all", "--remotes", "--list", "--verbose", "--show-current", "--column", "--no-column", "--color",
			"--no-color", "--track", "--no-track", "--quiet", "--unset-upstream", "--no-abbrev", "--ignore-case"},
			withValue: []string{"--contains", "--no-contains", "--merged", "--no-merged", "--sort", "--format", "--points-at", "--set-upstream-to", "--abbrev"},
			shorts:    "arlvtqi", valueShort: "u"},
		"tag": {longs: []string{"--list", "--annotate", "--sign", "--column", "--ignore-case"},
			withValue: []string{"--message", "--file", "--sort", "--contains", "--no-contains", "--merged", "--points-at", "--format"},
			shorts:    "lasin", valueShort: "mF"},
		"rebase": {longs: []string{"--continue", "--abort", "--skip", "--quit", "--autostash", "--no-autostash", "--rebase-merges", "--root",
			"--keep-base", "--update-refs", "--no-update-refs", "--signoff", "--committer-date-is-author-date", "--ignore-date",
			"--no-verify", "--fork-point", "--no-fork-point", "--quiet", "--verbose", "--force-rebase", "--stat", "--no-stat"},
			withValue: []string{"--onto", "--strategy", "--strategy-option", "--empty"}, shorts: "rqvfn", valueShort: "sX"},
		"stash":    {longs: []string{"--include-untracked", "--keep-index", "--quiet", "--staged", "--all", "--stat", "--patch", "--index", "--name-only"}, withValue: []string{"--message"}, shorts: "ukqSap", valueShort: "m"},
		"bisect":   {},
		"worktree": {longs: []string{"--porcelain", "--verbose", "--detach", "--lock", "--checkout", "--no-checkout", "--quiet"}, shorts: "vq", valueShort: "b"},
		"remote":   {longs: []string{"--verbose"}, shorts: "v"},
		"config":   {longs: []string{"--get", "--get-all", "--get-regexp", "--list", "--global", "--local", "--system", "--show-origin", "--show-scope", "--name-only", "--null", "--bool", "--int", "--path"}, shorts: "lz"},
		"reflog":   {longs: []string{"--date", "--all"}, withValue: []string{"--format", "--pretty", "--max-count"}, valueShort: "n"},
		"restore":  {longs: []string{"--staged", "--quiet"}, shorts: "Sq"},
		"checkout": {longs: []string{"--quiet", "--track", "--no-track"}, shorts: "qt", valueShort: "bc"},
		"grep":     {longs: []string{"--line-number", "--count", "--ignore-case", "--word-regexp", "--files-with-matches", "--name-only", "--cached", "--untracked", "--extended-regexp", "--fixed-strings", "--heading", "--break", "--column", "--invert-match", "--all-match", "--quiet", "--recurse-submodules", "--full-name", "--no-index"}, withValue: []string{"--max-depth", "--threads"}, shorts: "nciwlLEFvhHqrz", valueShort: "efABCm"},
	}
)

// gitDenyRule names the harm of an option a subcommand does not allow.
var gitDenyRule = map[string]string{
	"reset": RuleDelete, "switch": RuleDelete, "branch": RuleDelete, "tag": RuleDelete, "restore": RuleDelete,
	"checkout": RuleDelete, "stash": RuleDelete, "worktree": RuleDelete, "commit": RulePublish,
}

// gitReads reports a git command that reads the paths it names, or
// stages them, and writes none of them.
func gitReads(args []shellWord) bool {
	for len(args) > 0 && strings.HasPrefix(args[0].text, "-") {
		if args[0].text == "-C" && len(args) > 1 {
			args = args[1:]
		}
		args = args[1:]
	}
	verb, _, ok := subcommand(args)
	return ok && slices.Contains([]string{"add", "diff", "log", "show", "blame", "status", "ls-files", "grep", "whatchanged", "shortlog"}, verb)
}

// gitRule reads git's options, then a subcommand from gitOptions with
// only the options listed for it, by their full names: any other
// option, an abbreviation included, asks. An unknown subcommand may be
// an alias that runs a shell command, so it asks too.
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
	}
	opts, known := gitOptions[sub]
	if !known {
		return RuleShellSyntax
	}
	if slices.ContainsFunc(rest, func(w shellWord) bool { return w.text == "--amend" }) {
		return RulePublish
	}
	if !opts.check(rest) {
		if rule, named := gitDenyRule[sub]; named {
			return rule
		}
		return RuleShellSyntax
	}
	return gitVerbRule(sub, rest)
}

// gitVerbRule checks the subcommands whose own verbs or operands decide
// the harm.
func gitVerbRule(sub string, rest []shellWord) string {
	verb, _, _ := subcommand(rest)
	switch sub {
	case "checkout":
		return gitCheckout(rest)
	case "restore":
		if !slices.ContainsFunc(rest, func(w shellWord) bool { return w.text == "--staged" || w.text == "-S" }) {
			return RuleDelete
		}
	case "stash":
		if slices.Contains([]string{"drop", "clear"}, verb) {
			return RuleDelete
		}
	case "bisect":
		if verb == "run" {
			return RuleShellSyntax
		}
	case "worktree":
		if verb != "list" && verb != "add" {
			return RuleDelete
		}
	case "remote":
		if len(rest) > 0 && rest[0].text != "-v" && rest[0].text != "--verbose" && verb != "show" && verb != "get-url" {
			return RulePublish
		}
	case "config":
		read := len(rest) > 0 && (rest[0].text == "get" || rest[0].text == "list")
		for _, a := range rest {
			name, _, _ := strings.Cut(a.text, "=")
			if slices.Contains([]string{"--get", "--get-all", "--get-regexp", "--list", "-l"}, name) {
				read = true
			}
		}
		if !read {
			return RuleShellSyntax
		}
		// Stored tokens live in remote URLs, credential helpers and extra
		// headers, which a listing prints.
		for _, a := range rest {
			if a.text == "--list" || a.text == "-l" || a.text == "list" || a.text == "--get-regexp" || gitSecretKey.MatchString(a.text) {
				return RuleSecret
			}
		}
	case "reflog":
		if len(rest) > 0 && verb != "show" && verb != "" {
			return RuleDelete
		}
	}
	return ""
}

// gitCheckout allows only creating a branch with -b or -c: any other
// form may overwrite files with a branch's or commit's version, and
// without reading the repository the gate cannot tell a branch name
// from a path.
func gitCheckout(rest []shellWord) string {
	if len(rest) == 0 || rest[0].text != "-b" && rest[0].text != "-c" {
		return RuleDelete
	}
	n := 0
	for _, a := range rest[1:] {
		if a.text == "--" {
			return RuleDelete
		}
		if !strings.HasPrefix(a.text, "-") {
			n++
		}
	}
	if n < 1 || n > 2 {
		return RuleDelete
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

// scriptOptions are the options a package manager reads after its
// subcommand; anything after "--" goes to the script.
var scriptOptions = options{
	longs:      []string{"--silent", "--if-present", "--workspaces", "--ws", "--include-workspace-root", "--recursive", "--parallel", "--stream", "--watch", "--coverage", "--bail"},
	withValue:  []string{"--workspace", "--filter", "--reporter"},
	shorts:     "sr",
	valueShort: "w",
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
	case "test", "t", "tst", "lint", "build", "typecheck", "format", "fmt", "check", "start", "dev", "serve", "watch", "e2e":
		if !scriptOptions.check(rest) {
			return RuleShellSyntax
		}
		return scriptRule(verb)
	case "run", "run-script":
		script, scriptArgs, ok := subcommand(rest)
		if !ok || !scriptOptions.check(scriptArgs) {
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
		if !(options{longs: []string{"--check", "--quiet", "--watch"}, shorts: "q"}).check(rest) {
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

// goBuildOptions are the go build, test, vet and run flags the gate
// allows. Flags such as -exec, -toolexec, -overlay, -modfile and -mod
// may run or fetch other code, so they are left out and ask.
var goBuildOptions = map[string]bool{
	"-v": true, "-race": true, "-count": true, "-run": true, "-bench": true, "-benchmem": true, "-cover": true,
	"-coverprofile": true, "-covermode": true, "-coverpkg": true, "-short": true, "-timeout": true, "-tags": true,
	"-failfast": true, "-json": true, "-o": true, "-p": true, "-parallel": true, "-cpu": true, "-trimpath": true,
	"-x": true, "-n": true, "-a": true, "-buildvcs": true, "-benchtime": true, "-shuffle": true, "-skip": true,
	"-fuzz": true, "-fuzztime": true, "-list": true, "-cpuprofile": true, "-memprofile": true, "-ldflags": true,
	"-gcflags": true, "-vet": true, "-msan": true, "-asan": true, "-work": true, "-outputdir": true, "-blockprofile": true,
	"-mutexprofile": true, "-trace": true, "-benchtime=": true, "-m": true, "-u": true, "-f": true, "-e": true, "-deps": true,
	"-find": true, "-test": true, "-compiled": true, "-export": true,
}

// goValueFlags take the next word as their value when not written
// -flag=value.
var goValueFlags = []string{"-count", "-run", "-bench", "-coverprofile", "-covermode", "-coverpkg", "-timeout", "-tags",
	"-o", "-p", "-parallel", "-cpu", "-benchtime", "-shuffle", "-skip", "-fuzz", "-fuzztime", "-list", "-cpuprofile",
	"-memprofile", "-ldflags", "-gcflags", "-vet", "-outputdir", "-blockprofile", "-mutexprofile", "-trace", "-f"}

var gitSecretKey = regexp.MustCompile(`(?i)url|credential|token|password|header|auth`)

var goExternalLinker = regexp.MustCompile(`(?i)extld|linkmode|-X\s*main|toolexec`)

func goRule(args []shellWord) string {
	if isVersionOrHelp(args) {
		return ""
	}
	verb, rest, ok := subcommand(args)
	if !ok {
		return RuleShellSyntax
	}
	switch verb {
	case "build", "test", "vet", "list", "run":
		return goFlags(verb, rest)
	case "mod":
		if next, _, _ := subcommand(rest); slices.Contains([]string{"tidy", "verify", "why", "graph"}, next) && len(rest) == 1 {
			return ""
		}
		return RuleNetwork
	case "env":
		if len(rest) == 0 || !strings.HasPrefix(rest[0].text, "-") {
			return ""
		}
		if rest[0].text == "-json" {
			return ""
		}
		return RuleShellSyntax
	case "fmt", "doc", "help":
		return ""
	case "install", "get":
		return RuleNetwork
	}
	return RuleShellSyntax
}

// goFlags checks go's flags up to -args or the first operand after which
// test binary flags follow; a flag go does not list asks.
func goFlags(verb string, rest []shellWord) string {
	for i := 0; i < len(rest); i++ {
		text := rest[i].text
		if text == "-args" || text == "--args" {
			return ""
		}
		if !strings.HasPrefix(text, "-") {
			if verb == "run" && strings.Contains(text, "@") {
				return RuleNetwork
			}
			if verb == "run" {
				// Everything after the package goes to the program.
				return ""
			}
			continue
		}
		name, value, hasValue := strings.Cut(strings.Replace(text, "--", "-", 1), "=")
		if !goBuildOptions[name] {
			return RuleShellSyntax
		}
		if !hasValue && slices.Contains(goValueFlags, name) && i+1 < len(rest) {
			i++
			value = rest[i].text
		}
		if (name == "-ldflags" || name == "-gcflags") && goExternalLinker.MatchString(value) {
			return RuleShellSyntax
		}
	}
	return ""
}

var cargoOptions = options{
	longs: []string{"--release", "--all", "--workspace", "--all-features", "--no-default-features", "--lib", "--bins", "--tests",
		"--examples", "--benches", "--all-targets", "--locked", "--frozen", "--offline", "--quiet", "--verbose", "--no-run",
		"--doc", "--check", "--no-fail-fast", "--future-incompat-report", "--keep-going"},
	withValue: []string{"--package", "--features", "--bin", "--test", "--example", "--bench", "--target", "--jobs",
		"--message-format", "--color", "--profile", "--manifest-path", "--exclude", "--target-dir"},
	shorts:     "qv",
	valueShort: "pjF",
}

func cargoRule(args []shellWord) string {
	if isVersionOrHelp(args) {
		return ""
	}
	verb, rest, ok := subcommand(args)
	if !ok {
		return RuleShellSyntax
	}
	switch verb {
	case "build", "b", "check", "c", "test", "t", "fmt", "clippy", "doc", "d", "run", "r", "bench", "tree", "metadata":
		if !cargoOptions.check(rest) {
			return RuleShellSyntax
		}
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
