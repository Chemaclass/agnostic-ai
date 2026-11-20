package approvals

import (
	"maps"
	"slices"
	"strings"
)

// optionKind says how an option takes its value: flag takes none,
// separate takes the next word, glued takes the rest of the word
// (--name=value, or -nvalue for a short option). Kinds combine.
type optionKind int

const (
	flag optionKind = 1 << iota
	separate
	glued
)

// option is one allowed option. text marks a value that is a message,
// pattern or count rather than a path; every other value is checked
// as a path. pattern marks an option that gives the search pattern, so
// the first operand is then a path.
type option struct {
	kind    optionKind
	text    bool
	pattern bool
}

type operandKind int

const (
	operandPath operandKind = iota
	operandText
	operandName
	operandPackage
	operandNone
)

// form is what the gate allows for a program, or a program's verb:
// the options by exact name, short letters that may be bundled as
// flags, and what its operands are. Anything else asks.
type form struct {
	opts        map[string]option
	bundle      string
	digits      bool
	operand     operandKind
	pattern     bool
	dashes      bool
	minOperands int
	maxOperands int
	verify      func(operands []string) string
	requires    string
	// wordOptions marks a program whose single-dash options are whole
	// words, as Go's are: -overlay is not -o with a value.
	wordOptions bool
	skip        int
}

func opts(kind optionKind, text bool, names ...string) map[string]option {
	m := make(map[string]option, len(names))
	for _, n := range names {
		m[n] = option{kind: kind, text: text}
	}
	return m
}

func merge(parts ...map[string]option) map[string]option {
	m := map[string]option{}
	for _, part := range parts {
		maps.Copy(m, part)
	}
	return m
}

var (
	gitLogOpts = merge(
		opts(flag, false, "--oneline", "--stat", "--graph", "--decorate", "-p", "--patch", "--name-only", "--name-status", "--no-merges", "--all", "--reverse", "--shortstat"),
		opts(separate|glued, true, "-n"),
		opts(glued, true, "--format", "--pretty", "--since", "--until", "--author", "--grep", "--max-count"),
	)

	gitVerbs = map[string]form{
		"status": {opts: opts(flag, false, "--short", "--branch", "--porcelain", "-s", "-b", "-sb"), dashes: true, maxOperands: -1},
		"diff":   {opts: opts(flag, false, "--stat", "--cached", "--staged", "--name-only", "--name-status", "--shortstat", "--numstat"), dashes: true, maxOperands: -1},
		"log":    {opts: gitLogOpts, digits: true, dashes: true, maxOperands: -1},
		"show":   {opts: gitLogOpts, dashes: true, maxOperands: -1},
		"add":    {opts: opts(flag, false, "-A", "--all", "-u", "--update"), dashes: true, maxOperands: -1},
		"commit": {opts: merge(opts(separate, true, "-m", "-am"), opts(flag, false, "-a")), operand: operandNone, maxOperands: 0},
		"switch": {opts: opts(separate, false, "-c"), operand: operandName, maxOperands: 1},
		"branch": {operand: operandName, maxOperands: 1},
		"stash":  {operand: operandText, maxOperands: 1, verify: oneOf("push", "pop", "list")},
		"pull":   {maxOperands: 0},
		"fetch":  {opts: opts(flag, false, "--prune", "--all"), operand: operandName, maxOperands: 1},
		"merge":  {operand: operandName, minOperands: 1, maxOperands: 1},
	}

	goTestOpts = merge(
		opts(flag, false, "-v", "-race", "-short", "-cover", "-failfast", "-json", "-benchmem"),
		opts(separate|glued, true, "-count", "-run", "-skip", "-timeout", "-tags", "-bench", "-cpu", "-parallel", "-p"),
		opts(separate|glued, false, "-coverprofile", "-o"),
	)

	goVerbs = map[string]form{
		"test":    {opts: goTestOpts, wordOptions: true, operand: operandPackage, maxOperands: -1},
		"build":   {opts: goTestOpts, wordOptions: true, operand: operandPackage, maxOperands: -1},
		"vet":     {opts: opts(separate|glued, true, "-tags"), wordOptions: true, operand: operandPackage, maxOperands: -1},
		"fmt":     {operand: operandPackage, maxOperands: -1},
		"run":     {opts: opts(flag, false, "-race"), wordOptions: true, operand: operandPackage, minOperands: 1, maxOperands: 1},
		"version": {maxOperands: 0},
	}

	cargoForm = form{
		opts: merge(
			opts(flag, false, "--release", "--all", "--workspace", "-q", "--quiet", "--all-features", "--locked", "--offline", "--all-targets"),
			opts(separate, false, "-p", "--package"),
		),
		maxOperands: 0,
	}

	cargoVerbs = map[string]form{
		"test": cargoForm, "build": cargoForm, "check": cargoForm, "clippy": cargoForm,
		"fmt": {opts: opts(flag, false, "--check", "--all"), maxOperands: 0},
	}

	// scripts are the package.json scripts a package manager may run.
	scripts = []string{"test", "build", "lint", "dev", "check", "typecheck", "format", "fmt"}

	packageVerbs = func() map[string]form {
		verbs := map[string]form{"run": {operand: operandText, minOperands: 1, maxOperands: 1, verify: oneOf(scripts...)}}
		for _, s := range scripts {
			verbs[s] = form{maxOperands: 0}
		}
		return verbs
	}()

	makeTargets = []string{"test", "build", "lint", "check", "fmt", "vet"}

	// programs have verbs: the first word after the program picks the form.
	programs = map[string]map[string]form{
		"git":   gitVerbs,
		"go":    goVerbs,
		"cargo": cargoVerbs,
		"npm":   packageVerbs,
		"pnpm":  packageVerbs,
		"yarn":  packageVerbs,
	}

	// viewers read the paths they are given and print.
	viewers = map[string]form{
		"ls":   {bundle: "lahRtrSd1AF", maxOperands: -1},
		"cat":  {opts: opts(flag, false, "-n"), maxOperands: -1},
		"head": {opts: opts(separate|glued, true, "-n"), digits: true, maxOperands: -1},
		"tail": {opts: opts(separate|glued, true, "-n"), digits: true, maxOperands: -1},
		"wc":   {bundle: "lwcm", maxOperands: -1},
		"pwd":  {maxOperands: 0},
		"grep": {
			opts:   map[string]option{"-e": {kind: separate, text: true, pattern: true}},
			bundle: "nirlLvcwEFHhoqsRx", pattern: true, dashes: true, maxOperands: -1,
		},
		"rg": {
			opts: merge(
				map[string]option{"-e": {kind: separate, text: true, pattern: true}, "--files": {kind: flag, pattern: true}},
				opts(separate, true, "-t", "--type"),
			),
			bundle: "nilwFcS", pattern: true, dashes: true, maxOperands: -1,
		},
		"find": {
			opts: merge(
				opts(separate, true, "-name", "-iname", "-path", "-ipath", "-type", "-maxdepth", "-mindepth"),
				opts(flag, false, "-print", "-o", "-a", "-not"),
			),
			maxOperands: -1,
		},
		"sed": {opts: opts(flag, false, "-n", "-E"), requires: "-n", pattern: true, minOperands: 1, maxOperands: -1},
		"jq": {
			opts:    opts(flag, false, "-r", "-c", "-n", "-s", "-e", "-S", "--raw-output", "--compact-output", "--slurp", "--null-input", "--sort-keys"),
			pattern: true, minOperands: 1, maxOperands: -1, verify: jqFilterReadable,
		},
		"pytest": {
			opts: merge(
				opts(flag, false, "--lf", "--ff", "-q", "-v", "-x", "-s", "-vv", "-qq"),
				opts(separate, true, "-k", "-m"),
				opts(glued, true, "--tb", "--maxfail"),
			),
			maxOperands: -1,
		},
		"make": {operand: operandText, minOperands: 1, maxOperands: -1, verify: oneOf(makeTargets...)},
	}

	// filters may follow a pipe: they read their input and print, and
	// take no file.
	filters = map[string]form{
		"sort":  {opts: opts(separate|glued, true, "-k", "-t"), bundle: "rnufhV", operand: operandNone, maxOperands: 0},
		"uniq":  {bundle: "cdui", operand: operandNone, maxOperands: 0},
		"head":  {opts: opts(separate|glued, true, "-n"), digits: true, operand: operandNone, maxOperands: 0},
		"tail":  {opts: opts(separate|glued, true, "-n"), digits: true, operand: operandNone, maxOperands: 0},
		"wc":    {bundle: "lwcm", operand: operandNone, maxOperands: 0},
		"grep":  {opts: map[string]option{"-e": {kind: separate, text: true, pattern: true}}, bundle: "nivcwEFoqxh", operand: operandNone, pattern: true, maxOperands: 1},
		"cut":   {opts: opts(separate|glued, true, "-d", "-f", "-c"), operand: operandNone, maxOperands: 0},
		"tr":    {bundle: "dsc", operand: operandText, minOperands: 1, maxOperands: 2},
		"comm":  {bundle: "123", maxOperands: 2},
		"paste": {opts: opts(separate|glued, true, "-d"), bundle: "s", maxOperands: -1},
	}
)

// verify hooks: sed reads its script and jq its filter; the first
// operand is the one the form reads as text.
func init() {
	sed := viewers["sed"]
	sed.verify = func(operands []string) string {
		if !sedScriptReadable(operands[0]) {
			return RuleUnknownCommand
		}
		return ""
	}
	viewers["sed"] = sed
}

func oneOf(allowed ...string) func([]string) string {
	return func(operands []string) string {
		for _, o := range operands {
			if releaseWord.MatchString(o) {
				return RulePublish
			}
			if !slices.Contains(allowed, o) {
				return RuleUnknownCommand
			}
		}
		return ""
	}
}

// jqFilterReadable asks for a filter that reads the environment, a
// module or a file name.
func jqFilterReadable(operands []string) string {
	lower := strings.ToLower(operands[0])
	for _, word := range []string{"env", "import", "include", "input_filename", "$__loc__", "modulemeta", "get_search_list", "$__prog"} {
		if strings.Contains(lower, word) {
			return RuleSecret
		}
	}
	return ""
}
