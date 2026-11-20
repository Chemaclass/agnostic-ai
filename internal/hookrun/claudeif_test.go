package hookrun

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func toolBody(t *testing.T, tool string, input map[string]any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"tool_name": tool, "tool_input": input})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// Each want is what the hook `if` matcher of Claude Code 2.1.328 returned
// for the rule and command, run outside Claude Code.
func TestClaudeIfRuns_BashRules(t *testing.T) {
	for _, tc := range []struct {
		rule, command string
		want          bool
	}{
		{"Bash(git *)", "(git push)", true},
		{"Bash(git *)", "if true; then git push; fi", true},
		{"Bash(git *)", "! git push", true},
		{"Bash(git *)", "{ git push; }", true},
		{"Bash(git *)", "cat <(git log)", true},
		{"Bash(git *)", "echo hi > >(git push)", true},
		{"Bash(git *)", "> >(cat) git push", true},
		{"Bash(git *)", "{fd}>/dev/null git push", true},
		{"Bash(rm *)", "cat <<X\na \\\nb\nX", true},
		{"Bash(rm *)", "cat <<'X'\na\\\nb\nX", true},
		{"Bash(git *)", "echo $(cat <<X\n)'\nX\n); git push #'", true},
		{"Bash(git *)", "time -p git push", false},
		{"Bash(git *)", "env git push", false},
		{"Bash(git *)", "env FOO=1 git push", false},
		{"Bash(git *)", "bash -c 'git push'", false},
		{"Bash(git *)", "sh -c 'git push'", false},
		{"Bash(git *)", "bash <<< 'git push'", false},
		{"Bash(git *)", "timeout 5 git push", false},
		{"Bash(git *)", "nohup git push", false},
		{"Bash(git *)", "nice git push", false},
		{"Bash(git *)", "FOO=1 git push", true},
		{"Bash(git *)", "ls | git push", true},
		{"Bash(git *)", "git push &", true},
		{"Bash(git *)", "echo $(git push)", true},
		{"Bash(git push)", "git push &>/dev/null", true},
		{"Bash(make *)", "make&>log", true},
		{"Bash(\"/opt/my tool/run\" *)", "\"/opt/my tool/run\" x", false},
		{"Bash(/opt/my tool/run *)", "\"/opt/my tool/run\" x", true},
		{"Bash(\"git\" *)", "\"git\" push", false},
		{"Bash(\\git *)", "\\git push", false},
		{"Bash(git *)", "\"git\" push", true},
		{"Bash(* git *)", "nohup >log git push", true},
		{"Bash(*>*)", ">out echo hi", false},
		{"Bash(rm *)", "cat <<EOF\n$(date)\nEOF", true},
		{"Bash(rm *)", "cat <<'EOF'\n$(rm x)\nEOF", false},
		{"Bash(git *)", "cat <<-'X'\n\tX\ngit push", true},
		{"Bash(git *)", "cat <<-'X'\n  X\ngit push", false},
		{"Bash(git *)", "cat <<'X'\nX\ngit push", true},
		{"Bash(git *)", "echo a#b; git push", true},
		{"Bash(git *)", "echo a #b; git push", false},
		{"Bash(git *)", "for x in a b; do git push; done", true},
		{"Bash(git *)", "while true; do git push; done", true},
		{"Bash(git *)", "case x in a) git push;; esac", true},
		{"Bash(git *)", "f() { git push; }", true},
		{"Bash(git *)", "((x=1)) && git push", true},
		{"Bash(git *)", "xargs git push", true},
		{"Bash(git *)", "sudo git push", false},
		{"Bash(xargs *)", "git push", false},
		{"Bash(git:*)", "xargs git push", true},
		{"Bash(git push *)", "echo $(git push)", true},
		{"Bash(git:*)", "echo $(git push)", true},
		{"Bash(git push:*)", "echo $(git push)", true},
		{"Bash(git*)", "echo $(gitk)", true},
		{"Bash(git *)", "echo 'a' $(git push)", true},
		{"Bash(git *)", "git push >out x", true},
		{"Bash(git *)", "echo hi >out 2>&1; git push", true},
		{"Bash(git *)", "cat <<<x git", false},
		{"Bash(git *)", "echo a{b,c}; git push", true},
		{"Bash(git *)", "echo {}; git push", true},
		{"Bash(git *)", "git push \\\n--force", true},
		{"Bash(git *)", "echo hi\r; git push", true},
		{"Bash(echo a *)", "echo \"a $(date)\"", true},
		{"Bash(date)", "X=$(date)", true},
		{"Bash(rm *)", "X=$(rm -rf x)", true},
		{"Bash(git *)", "X=$(a; b); ls", true},
		{"Bash(git *)", "echo \"$(git push)\"", true},
		{"Bash(git *)", "echo \"$x\"; ls", true},
		{"Bash(git *)", "echo $x; git push", true},
		{"Bash(git *)", "echo $x", false},
		{"Bash(e *)", "echo $x", false},
		{"Bash(echo *)", "echo $x", true},
		{"Bash(git *)", "echo ${x}; git push", true},
		{"Bash(git *)", "echo $((1+2)); git push", true},
		{"Bash(git *)", "echo $'a'; git push", true},
		{"Bash(git *)", "[[ -n x ]] && git push", true},
		{"Bash(git *)", "(cd x && git push)", true},
		{"Bash(git *)", "git log --format='%H'", true},
		{"Bash(git log --format=%H)", "git log --format='%H'", true},
		{"Bash(npm run *)", "npm  run   build", true},
		{"Bash(npm run *)", "npm\trun build", true},
		{"Bash(git \\* x)", "git * x", true},
		{"Bash(git \\* x)", "git a x", false},
		{"Bash(cat **/a)", "cat x/y/a", true},
		{"Bash(cat /**/a)", "cat /x/y/a", true},
		{"Bash(cat /**/a)", "cat /a", true},
		{"Bash(git *)", "", false},
		{"Bash(git *)", "x=1", false},
		{"Bash(git *)", "cat < in x; git push", true},
		{"Bash(git *)", "echo $(echo $(echo $(git push)))", true},
		{"Bash(git *)", "echo `date`; git push", true},
		{"Bash(git *)", "git push | `x`", true},
		{"Bash(git *)", "echo x$(git push)", true},
		{"Bash(ls *)", "ls $(pwd)", true},
		{"Bash(ls *)", "ls \"$(pwd)\"", true},
		{"Bash(ls *)", "ls 'a' \"$(pwd)\"", true},
		{"Bash(python *)", "python $(x)", true},
		{"Bash(python *)", "python x", true},
		{"Bash(git *)", "git push 2>/dev/null | tee $(date)", true},
		{"Bash(git *)", "X=1 git $(push)", true},
		{"Bash(git *)", "git $(push) >out", true},
		{"Bash(git *)", "git \"a$x\" $(y)", true},
		{"Bash(git *)", "git a$x $(y)", true},
		{"Bash(git *)", "git a\\b $(y)", true},
		{"Bash(git *)", "git $(y) &", true},
		{"Bash(git *)", "git $(y)\n", true},
		{"Bash(git *)", "git $(y) # c", true},
		{"Bash(git *)", "! git $(y)", true},
		{"Bash(git *)", "(git $(y))", true},
		{"Bash(git *)", "git $(a | b)", true},
		{"Bash(git *)", "git $(a && b)", true},
		{"Bash(git *)", "/usr/bin/git $(y)", false},
		{"Bash(/usr/bin/git *)", "/usr/bin/git $(y)", true},
		{"Bash(git *)", "echo \"a $(git push) b\"", true},
		{"Bash(git push *)", "echo \"a $(git push) b\"", true},
		// Rows from the review of #3742.
		{"Bash(rm *)", "ls\\", true},
		{"Bash(rm *)", "echo `a\\\\`", true},
		{"Bash(rm *)", "cat <<'EOF' > f\nhello\nEOF", true},
		{"Bash(rm *)", "cat <<'EOF' | grep h\nhello\nEOF", true},
		{"Bash(rm *)", "cat > f <<'EOF'\nhello\nEOF", false},
		{"Bash(rm *)", "make 2>&1 -j8", true},
		{"Bash(rm *)", "echo hi >&2 there", true},
		{"Bash(rm *)", "git status 2>&1 # c", true},
		{"Bash(rm *)", "ls >out <<<x", true},
		{"Bash(rm *)", "ls <<<x >out", false},
		{"Bash(rm *)", "grep a$ f", true},
		{"Bash(rm *)", "echo $", true},
		{"Bash(rm *)", "echo $\"hi\"", true},
		{"Bash(rm *)", "echo \"a$\"", false},
		{"Bash(rm *)", "i=$((i+1))", true},
		{"Bash(rm *)", "echo $((1+2))", false},
		{"Bash(rm *)", "echo \"$((1+1))\"", true},
		{"Bash(rm *)", "while read l; do echo x; done < f", true},
		{"Bash(rm *)", "(ls) 2>&1", true},
		{"Bash(ls *)", "for f in $(ls); do echo; done", true},
		{"Bash(rm *)", "for f in $x; do echo; done", true},
		{"Bash(rm *)", "for f in \"$(ls)\"; do echo; done", true},
		{"Bash(rm *)", "select x in a; do ls; done", true},
		{"Bash(rm *)", "test a = b", true},
		{"Bash(rm *)", "echo ==", false},
		{"Bash(rm *)", "ls =x", true},
		{"Bash(rm *)", "ls 2>&-", true},
		{"Bash(rm *)", "ls <>f", true},
		{"Bash(rm *)", "echo $x$y", true},
		{"Bash(rm *)", "a=b c=d", true},
		{"Bash(rm *)", "a[0]=1 ls", true},
		{"Bash(rm *)", "a=~/x ls", true},
		{"Bash(rm *)", "echo {a, b}", true},
		{"Bash(rm *)", "echo {a,b", true},
		{"Bash(rm *)", "echo '{a,b}'", false},
		{"Bash(rm *)", "echo \\{a,b}", true},
		{"Bash(rm *)", "git commit -m \"$(cat <<'EOF'\nmsg\nEOF\n)\"", false},
		{"Bash(rm *)", "git commit -m \"$(cat <<'EOF'\nmsg $(rm x)\nEOF\n)\" && rm x", true},
		{"Bash(rm *)", "git commit -m $(cat <<'EOF'\nmsg\nEOF\n)", true},
		{"Bash(rm *)", "x=$(cat <<'EOF'\nmsg\nEOF\n)", false},
		{"Bash(rm *)", "fi", true},
		{"Bash(rm *)", "ls; fi", true},
		{"Bash(rm *)", "do rm x", true},
		{"Bash(rm *)", "if true; then ls", false},
		{"Bash([[ *)", "[ -f x ]", true},
		{"Bash([ *)", "[ -f x ]", false},
		{"Bash(rm *)", "[ -f x", true},
		{"Bash(rm *)", "export a/b", true},
		{"Bash(rm *)", "export A=1 B", false},
		{"Bash(rm *)", "declare -a x", true},
		{"Bash(rm *)", "command ls *.go", false},
		{"Bash(rm *)", "command ?", true},
		{"Bash(rm *)", "<<<x", true},
		{"Bash(*)", ">out", true},
		{"Bash(rm *)", "\"$(ls) \" x", true},
		{"Bash(rm *)", "echo `a \\$x`", true},
		// Rows from before #3742, rechecked against the same matcher.
		{"Bash(git *)", "FOO=bar git push", true},
		{"Bash(git *)", "npm test && git push", true},
		{"Bash(rm *)", "echo $(rm -rf /)", true},
		{"Bash(rm *)", "echo `rm -rf /`", true},
		{"Bash(rm *)", "echo $(date)", false},
		{"Bash(git push *)", "echo $(date)", true},
		{"Bash(npm run build)", "npm run build", true},
		{"Bash(npm run build)", "npm run build --watch", false},
		{"Bash(npm run *)", "npm run", true},
		{"Bash(npm run *)", "npm install", false},
		{"Bash(git log * main)", "git log --oneline main", true},
		{"Bash(git log * main)", "git log main", false},
		{"Bash(* --version)", "node --version", true},
		{"Bash(ls *)", "ls", true},
		{"Bash(ls *)", "lsof", false},
		{"Bash(ls*)", "lsof", true},
		{"Bash(* --help *)", "npm --help", false},
		{"Bash(ls:*)", "ls -la", true},
		{"Bash(npm test *)", "timeout 30 npm test", false},
		{"Bash(git *)", "echo 'git push'", false},
		{"Bash(git *)", "ls | grep x; git status", true},
		{"Bash(git *)", "npm test &&", true},
		{"Bash", "anything at all", true},
		{"Bash(*)", "anything at all", true},
		{"Bash(git *)", "echo \"git push\"", false},
		{"Bash(git *)", "echo \"a; git push\"", false},
		{"Bash(git *)", "echo $(echo \"(\" ; git push)", true},
		{"Bash(git *)", "echo $(echo ')' ; git push)", true},
		{"Bash(git *)", "echo $(echo $(git push))", true},
		{"Bash(git *)", "echo $(echo \"(\" ; ls)", true},
		{"Bash(git *)", "echo $(echo ')' ; ls)", true},
		{"Bash(git *)", "echo $(echo $(ls))", false},
		{"Bash(git *)", "echo `echo \\`git push\\``", true},
		{"Bash(git *)", "echo `echo \\`ls\\``", true},
		{"Bash(git *)", "echo `echo \\$(git push)`", true},
		{"Bash(git *)", "echo `echo \\\\; git push`", true},
		{"Bash(git *)", "echo \"`echo \\\" ' \\\" ; git push ; echo \\\" ' \\\"`\"", true},
		{"Bash(git *)", "echo `echo \\` ; ls`", true},
		{"Bash(git *)", "echo `echo \\` ; git push`", true},
		{"Bash(git *)", "cat <<X\n\"`echo \\\" ; git push ; echo \\\"`\"\nX", true},
		{"Bash(git *)", "echo a # \"\necho `echo \\\" ; git push ; echo \\\"` # \"", true},
		{"Bash(git *)", "\\git push", true},
		{"Bash(git *)", "\\\ngit push", true},
		{"Bash(git *)", "echo `git \\\npush`", true},
		{"Bash(git *)", "``git push", true},
		{"Bash(git *)", "$()git push", true},
		{"Bash(git *)", "eval 'echo `git push`'", false},
		{"Bash(git *)", "echo a # don't\ngit push # '", true},
		{"Bash(git *)", "cat <<X\n'`git push`'\nX", true},
		{"Bash(git *)", "cat <<X\n$(git push)\nX\nls", true},
		{"Bash(git *)", "cat <<-X\n\t'\n\tX\ngit push", true},
		{"Bash(git *)", "cat <<'X'\n'\nX\ngit push", true},
		{"Bash(git *)", "cat <<X\nunterminated `git push`", true},
		{"Bash(git *)", "echo a#b ; git push", true},
		{"Bash(git *)", "echo $# ; git push", true},
		{"Bash(git *)", "cat <<<'git push'", false},
		{"Bash(git *)", "cat <<<x ; git push", true},
		{"Bash(git *)", "cat <<<x\ngit push", true},
		{"Bash(git *)", "cat <<X", true},
		{"Bash(git *)", "cat <<", true},
		{"Bash(git *)", "\\git push \"a\nb\"", true},
		{"Bash(git *)", "`echo` push", true},
		{"Bash(git *)", "$(echo git) push", true},
		{"Bash(git *)", "_git push", false},
		{"Bash(git *)", "<<<x git push", true},
		{"Bash(git *)", "2>&1 >\"out file\" git push", true},
		{"Bash(git *)", "<<<\"a\nb\" \\git push", true},
		{"Bash(git *)", ">out echo git push", false},
		{"Bash(git *)", "&>log git push", true},
		{"Bash(git *)", ">|log git push", true},
		{"Bash(git *)", "ls 2>&1 && git push", true},
		{"Bash(git *)", "ls &> log & git push", true},
		{"Bash(git *)", "<<<\\; git push", true},
		{"Bash(git *)", "''\\git push", true},
		{"Bash(git *)", "\"g\"it push", true},
		{"Bash(git *)", "echo a # ; git push", false},
		{"Bash(git *)", "cat <<X\ngit push\nX", true},
		{"Bash(git *)", "bash <<'X'\ngit push\nX", false},
		{"Bash(git *)", "(( x = 1 << 4 ))\ngit push", true},
		{"Bash(git *)", "echo ${x:- #}; git push", true},
		{"Bash(git *)", "echo a\u00a0#b; git push", true},
		{"Bash(git *)", "cat <<$(a)\n$(a)\ngit push", true},
		{"Bash(git *)", "(true)# don't\ngit push # '", true},
		{"Bash(git *)", "echo \\>& git push", true},
		{"Bash(git *)", "echo \\>| git push", true},
		{"Bash(git *)", "$x git push", true},
		{"Bash(git *)", "g${x}it push", true},
		{"Bash(git *)", "NOW=$(date)\necho hi", false},
		{"Bash(git *)", "x=$y\necho hi", true},
	} {
		got, err := ClaudeIfRuns(tc.rule, "PreToolUse", toolBody(t, "Bash", map[string]any{"command": tc.command}), "/p")
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%s on %q = %v, want %v", tc.rule, tc.command, got, tc.want)
		}
	}
}

func TestClaudeIfRuns_FileRules(t *testing.T) {
	root := filepath.FromSlash("/p")
	for _, tc := range []struct {
		rule, tool, file string
		want             bool
	}{
		{"Edit(*.ts)", "Write", "/p/src/app.ts", true},
		{"Edit(*.ts)", "Edit", "/p/src/app.go", false},
		{"Edit(/src/**)", "Edit", "/p/src/x/a.ts", true},
		{"Edit(/src/**)", "Edit", "/p/vendor/src/a.ts", false},
		{"Edit(**/src/**)", "MultiEdit", "/p/vendor/pkg/src/lib.js", true},
		{"Edit(src/**)", "Edit", "/p/vendor/pkg/src/lib.js", false},
		{"Edit(src/components/**)", "Edit", "/p/vendor/src/components/a.ts", false},
		{"Edit(.env)", "Write", "/p/a/b/.env", true},
		{"Edit(//tmp/scratch.txt)", "Write", "/tmp/scratch.txt", true},
		{"Edit(./docs/*.md)", "Write", "/p/docs/a.md", true},
		{"Edit(./docs/*.md)", "Write", "/p/docs/x/a.md", false},
		{"Edit(docs/**)", "Bash", "", false},
		{"Write", "Write", "/p/a", true},
		{"Write", "Edit", "/p/a", false},
	} {
		input := map[string]any{"command": "ls"}
		if tc.file != "" {
			input = map[string]any{"file_path": filepath.FromSlash(tc.file)}
		}
		got, err := ClaudeIfRuns(tc.rule, "PostToolUse", toolBody(t, tc.tool, input), root)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%s on %s %s = %v, want %v", tc.rule, tc.tool, tc.file, got, tc.want)
		}
	}
}

// A command Claude Code cannot parse runs the hook, whatever the rule.
func TestClaudeIfRuns_BashRuleRunsOnACommandItCannotParse(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		"echo `git push",
		"echo $(git push",
		"echo `npm test &&`",
		"echo $(npm test &&)",
	} {
		got, err := ClaudeIfRuns("Bash(rm *)", "PreToolUse", toolBody(t, "Bash", map[string]any{"command": command}), "/p")
		if err != nil {
			t.Fatal(err)
		}
		if !got {
			t.Errorf("Bash(rm *) on %q = false, want true", command)
		}
	}
}

func TestClaudeIfRuns_FileRulesOutsideTheAnchorAndOnOtherTools(t *testing.T) {
	t.Parallel()
	root := filepath.FromSlash("/p")
	for _, tc := range []struct {
		name, rule, tool string
		input            map[string]any
		want             bool
	}{
		{"a file outside the project root", "Edit(/src/**)", "Edit", map[string]any{"file_path": filepath.FromSlash("/elsewhere/src/a.ts")}, false},
		{"a bare name on a file outside the project root", "Edit(*.ts)", "Edit", map[string]any{"file_path": filepath.FromSlash("/elsewhere/a.ts")}, false},
		{"a notebook edit with only notebook_path", "Edit(*.ipynb)", "NotebookEdit", map[string]any{"notebook_path": filepath.FromSlash("/p/nb/a.ipynb")}, true},
		{"a spec on a tool without spec rules", "Grep(foo)", "Grep", map[string]any{"pattern": "foo"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ClaudeIfRuns(tc.rule, "PreToolUse", toolBody(t, tc.tool, tc.input), root)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("%s on %s %v = %v, want %v", tc.rule, tc.tool, tc.input, got, tc.want)
			}
		})
	}
}

// Sets HOME, so it cannot run in parallel.
func TestClaudeIfRuns_HomeAnchoredFileRule(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, tc := range []struct {
		name, rule, file string
		want             bool
	}{
		{"a file under home", "Read(~/notes/**)", filepath.Join(home, "notes", "a.md"), true},
		{"a file outside home", "Read(~/notes/**)", filepath.Join(t.TempDir(), "notes", "a.md"), false},
		{"no home folder", "Read(~/**)", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := tc.file
			if file == "" {
				file = filepath.Join(home, "notes", "a.md")
				t.Setenv("HOME", "")
				t.Setenv("USERPROFILE", "")
			}
			got, err := ClaudeIfRuns(tc.rule, "PreToolUse", toolBody(t, "Read", map[string]any{"file_path": file}), "/p")
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("%s on %s = %v, want %v", tc.rule, file, got, tc.want)
			}
		})
	}
}

func TestClaudeIfRuns_OnlyOnToolEvents(t *testing.T) {
	body := toolBody(t, "Bash", map[string]any{"command": "git push"})
	if got, _ := ClaudeIfRuns("Bash(git *)", "SessionStart", body, "/p"); got {
		t.Error("a hook with if ran on SessionStart")
	}
	if got, _ := ClaudeIfRuns("", "SessionStart", body, "/p"); !got {
		t.Error("a hook without if did not run")
	}
	if got, _ := ClaudeIfRuns("Bash(git *)", "PermissionRequest", body, "/p"); !got {
		t.Error("if was not evaluated on PermissionRequest")
	}
}

func TestClaudeIfRuns_ParameterRule(t *testing.T) {
	body := toolBody(t, "Bash", map[string]any{"command": "sleep 9", "run_in_background": true})
	if got, _ := ClaudeIfRuns("Bash(run_in_background:true)", "PreToolUse", body, "/p"); !got {
		t.Error("parameter rule did not match")
	}
	if got, _ := ClaudeIfRuns("Bash(timeout:*)", "PreToolUse", body, "/p"); got {
		t.Error("a parameter the call leaves unset matched")
	}
}
