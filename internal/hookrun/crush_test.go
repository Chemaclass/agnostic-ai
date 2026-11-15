package hookrun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// crushGuard has no shebang, so Crush runs it in its own shell on every
// platform, Windows included.
const crushGuard = `read -r payload
case "$payload" in
  *'rm -rf'*) echo "no recursive delete" >&2; exit 2 ;;
  *shutdown*) echo "stop the turn" >&2; exit 49 ;;
  *'git push'*) exit 1 ;;
esac
echo "{\"context\":\"tool $CRUSH_TOOL_NAME in $CRUSH_EVENT\"}"
`

func crushGuardRun(t *testing.T, script, bash string) Result {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "guard.sh"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Build("crush", "PreToolUse", "^bash$", dir, Input{Bash: bash})
	if err != nil || !p.Fires {
		t.Fatalf("Build = %+v, %v", p, err)
	}
	return RunCrush("export AGNOSTIC_AI_TARGET=crush; ./guard.sh", dir, CrushEnv(os.Environ(), dir, p.Body), p.Body, 10*time.Second, runtime.GOOS)
}

func TestRunCrush_RunsAGuardAgainstCrushsPayload(t *testing.T) {
	for _, tc := range []struct {
		bash  string
		exit  int
		want  Decision
		halts bool
	}{
		{"rm -rf /", 2, Block, false},
		{"shutdown now", 49, Block, true},
		{"git push", 1, Error, false},
		{"ls", 0, Allow, false},
	} {
		t.Run(tc.bash, func(t *testing.T) {
			r := crushGuardRun(t, crushGuard, tc.bash)
			if r.Exit != tc.exit || r.StartErr != nil || r.TimedOut {
				t.Fatalf("result = %+v, want exit %d", r, tc.exit)
			}
			if got := DecideHandler("crush", "PreToolUse", Handler{}, r); got != tc.want {
				t.Errorf("decision = %s, want %s", got, tc.want)
			}
			if CrushHalts(r) != tc.halts {
				t.Errorf("CrushHalts = %t, want %t", CrushHalts(r), tc.halts)
			}
		})
	}
	r := crushGuardRun(t, crushGuard, "ls")
	if strings.TrimSpace(r.Stdout) != `{"context":"tool bash in PreToolUse"}` || !AddsContext("crush", "PreToolUse", r) {
		t.Errorf("the hook must see Crush's env, and its context reaches the tool result: %q", r.Stdout)
	}
}

func TestRunCrush_RunsAShebangScriptThroughItsInterpreter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs /bin/sh")
	}
	r := crushGuardRun(t, "#!/bin/sh\n"+crushGuard, "rm -rf /")
	if r.Exit != 2 || strings.TrimSpace(r.Stderr) != "no recursive delete" {
		t.Errorf("result = %+v", r)
	}
}

func TestRunCrush_DoesNotRunAScriptWithoutShebangUnlessItsPathStartsWithDot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exec format errors are a Unix kernel rule")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hooks", "guard.sh"), []byte("exit 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if r := RunCrush("hooks/guard.sh", dir, os.Environ(), nil, 5*time.Second, runtime.GOOS); r.Exit != 1 {
		t.Errorf("Crush's exec handler fails on an exec format error: %+v", r)
	}
	if r := RunCrush("./hooks/guard.sh", dir, os.Environ(), nil, 5*time.Second, runtime.GOOS); r.Exit != 2 {
		t.Errorf("a ./ path runs as shell source: %+v", r)
	}
}

func TestRunCrush_TimesOutAndDoesNotBlock(t *testing.T) {
	start := time.Now()
	r := RunCrush("while true; do :; done", t.TempDir(), nil, nil, 100*time.Millisecond, runtime.GOOS)
	if !r.TimedOut || DecideHandler("crush", "PreToolUse", Handler{}, r) != Timeout {
		t.Errorf("result = %+v", r)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("took %s; Crush gives up a second after the timeout", time.Since(start))
	}
}

func TestRunCrush_ParseErrorDoesNotBlock(t *testing.T) {
	r := RunCrush("if then", t.TempDir(), nil, nil, time.Second, runtime.GOOS)
	if r.StartErr == nil || DecideHandler("crush", "PreToolUse", Handler{}, r) != Error {
		t.Errorf("result = %+v", r)
	}
}

func TestRunCrush_NamesCrushBuiltinsItTookFromPath(t *testing.T) {
	r := RunCrush("provider add x; jq --version >/dev/null 2>&1; true", t.TempDir(), os.Environ(), nil, 5*time.Second, runtime.GOOS)
	if r.Exit != 0 || !slices.Equal(r.FromPath, []string{"jq"}) {
		t.Fatalf("config builtins are no-ops and jq is recorded: %+v", r)
	}
	got := CrushAssumptions(r)
	if len(got) != 1 || got[0].Item != "jq" || got[0].Reason == "" {
		t.Errorf("assumptions = %+v", got)
	}
}

func TestRunCrush_RecordsCoreUtilsOnlyWhereCrushServesThem(t *testing.T) {
	t.Setenv("CRUSH_CORE_UTILS", "")
	if r := RunCrush("ls >/dev/null 2>&1; true", t.TempDir(), os.Environ(), nil, 5*time.Second, "linux"); len(r.FromPath) != 0 {
		t.Errorf("Crush runs ls from PATH off Windows: %v", r.FromPath)
	}
	if r := RunCrush("ls >/dev/null 2>&1; true", t.TempDir(), os.Environ(), nil, 5*time.Second, "windows"); !slices.Equal(r.FromPath, []string{"ls"}) {
		t.Errorf("Crush serves ls itself on Windows: %v", r.FromPath)
	}
	t.Setenv("CRUSH_CORE_UTILS", "false")
	if r := RunCrush("ls >/dev/null 2>&1; true", t.TempDir(), os.Environ(), nil, 5*time.Second, "windows"); len(r.FromPath) != 0 {
		t.Errorf("CRUSH_CORE_UTILS=false turns them off: %v", r.FromPath)
	}
}

func TestDecideCrush_FollowsTheSource(t *testing.T) {
	for name, tc := range map[string]struct {
		r     Result
		want  Decision
		halts bool
	}{
		"exit 2 blocks":                                {Result{Exit: 2}, Block, false},
		"exit 2 ignores a JSON halt":                   {Result{Exit: 2, Stdout: `{"continue":false}`}, Block, false},
		"exit 49 halts":                                {Result{Exit: 49}, Block, true},
		"another exit does not block":                  {Result{Exit: 1}, Error, false},
		"another exit with text does not block":        {Result{Exit: 1, Stdout: "oops"}, Error, false},
		"a timeout does not block":                     {Result{TimedOut: true}, Timeout, false},
		"no output allows":                             {Result{}, Allow, false},
		"text allows":                                  {Result{Stdout: "ok"}, Allow, false},
		"decision deny blocks":                         {Result{Stdout: `{"decision":"DENY"}`}, Block, false},
		"decision block blocks":                        {Result{Stdout: `{"decision":"block","reason":"no"}`}, Block, false},
		"decision approve allows":                      {Result{Stdout: `{"decision":"approve"}`}, Allow, false},
		"halt blocks":                                  {Result{Stdout: `{"halt":true}`}, Block, true},
		"continue false halts":                         {Result{Stdout: `{"continue":false,"stopReason":"stop"}`}, Block, true},
		"continue true allows":                         {Result{Stdout: `{"continue":true}`}, Allow, false},
		"decision allow allows":                        {Result{Stdout: `{"decision":"allow"}`}, Allow, false},
		"permissionDecision deny blocks":               {Result{Stdout: `{"hookSpecificOutput":{"permissionDecision":"deny"}}`}, Block, false},
		"hookSpecificOutput keeps the top-level deny":  {Result{Stdout: `{"decision":"deny","hookSpecificOutput":{}}`}, Block, false},
		"hookSpecificOutput keeps continue false":      {Result{Stdout: `{"hookSpecificOutput":{"permissionDecision":"allow"},"continue":false}`}, Block, true},
		"permissionDecision overrides decision":        {Result{Stdout: `{"decision":"deny","hookSpecificOutput":{"permissionDecision":"allow"}}`}, Allow, false},
		"a JSON deny on another exit blocks":           {Result{Exit: 1, Stdout: `{"decision":"deny","reason":"no"}`}, Block, false},
		"a JSON halt on another exit halts":            {Result{Exit: 1, Stdout: `{"halt":true}`}, Block, true},
		"a JSON object on another exit has no opinion": {Result{Exit: 1, Stdout: `{}`}, Allow, false},
		"a non-string reason voids the reply":          {Result{Stdout: `{"decision":"deny","reason":{}}`}, Allow, false},
		"a non-int version voids the reply":            {Result{Stdout: `{"decision":"deny","version":"1"}`}, Allow, false},
		"a non-bool halt voids the reply":              {Result{Stdout: `{"halt":"yes"}`}, Allow, false},
		"a non-string decision voids it":               {Result{Stdout: `{"decision":true}`}, Allow, false},
		"a void reply on another exit is an error":     {Result{Exit: 1, Stdout: `{"halt":"yes"}`}, Error, false},
		"keys match case-insensitively":                {Result{Stdout: `{"Decision":"deny"}`}, Block, false},
		"a null hookSpecificOutput keeps the deny":     {Result{Stdout: `{"decision":"deny","hookSpecificOutput":null}`}, Block, false},
		"a bad hookSpecificOutput is skipped":          {Result{Stdout: `{"hookSpecificOutput":{"permissionDecision":1}}`}, Allow, false},
		"a bad hookSpecificOutput keeps the top level": {Result{Stdout: `{"decision":"deny","hookSpecificOutput":{"permissionDecision":"deny","permissionDecisionReason":[]}}`}, Block, false},
		"a JSON array is no reply":                     {Result{Stdout: `[{"decision":"deny"}]`}, Allow, false},
		"null is no reply":                             {Result{Stdout: `null`}, Allow, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := decideCrush(tc.r); got != tc.want {
				t.Errorf("decideCrush = %s, want %s", got, tc.want)
			}
			if got := CrushHalts(tc.r); got != tc.halts {
				t.Errorf("CrushHalts = %t, want %t", got, tc.halts)
			}
		})
	}
}

func TestCrushAddsContext_OnAnyExitThatAllows(t *testing.T) {
	if !crushAddsContext(Result{Exit: 1, Stdout: `{"context":"note"}`}) {
		t.Error("a JSON reply on another exit still carries its context")
	}
	if !crushAddsContext(Result{Stdout: `{"context":"a","hookSpecificOutput":{"additionalContext":"b"}}`}) {
		t.Error("additionalContext adds to context")
	}
	if crushAddsContext(Result{Stdout: `{"context":"a","decision":"block"}`}) {
		t.Error("a blocked tool gets no context")
	}
}

func TestBuildCrush_WritesTheToolInputCrushSends(t *testing.T) {
	p, err := buildCrush("pre_tool_use", "^(edit|write)$", "/project", Input{Edit: "a.go"})
	if err != nil || !p.Fires || p.Trigger != "edit" {
		t.Fatalf("Build = %+v, %v", p, err)
	}
	var doc struct {
		Event, SessionID, Cwd, ToolName string
		ToolInput                       map[string]any `json:"tool_input"`
	}
	if err := json.Unmarshal(p.Body, &doc); err != nil || doc.Event != "PreToolUse" || doc.ToolInput["file_path"] != filepath.Join("/project", "a.go") {
		t.Errorf("payload = %s", p.Body)
	}
	if p, _ := buildCrush("PreToolUse", "^write$", "/project", Input{Edit: "a.go"}); p.Trigger != "write" || !p.Fires {
		t.Errorf("a write matcher fires on write: %+v", p)
	}
	if p, _ := buildCrush("PreToolUse", "Bash", "/project", Input{Bash: "ls"}); p.Fires {
		t.Errorf("Crush's tool names are lowercase: %+v", p)
	}
	if _, err := buildCrush("PreToolUse", "(", "/project", Input{Bash: "ls"}); err == nil {
		t.Error("a matcher that does not compile must be reported")
	}
	if _, err := buildCrush("PreToolUse", "", "/project", Input{Prompt: "hi"}); err == nil {
		t.Error("Crush has no prompt event")
	}
}

func TestCrushEnv_SetsCrushVariablesAndForcesNonInteractive(t *testing.T) {
	env := CrushEnv([]string{"EDITOR=vim", "HOME=/h"}, "/project", []byte(`{"tool_name":"bash","tool_input":{"command":"ls"}}`))
	for _, want := range []string{"HOME=/h", "CRUSH=1", "AGENT=crush", "CRUSH_TOOL_NAME=bash", "CRUSH_PROJECT_DIR=/project", "CRUSH_TOOL_INPUT_COMMAND=ls", "EDITOR=false"} {
		if !slices.Contains(env, want) {
			t.Errorf("env misses %s: %v", want, env)
		}
	}
	if slices.Contains(env, "EDITOR=vim") {
		t.Errorf("Crush replaces EDITOR: %v", env)
	}
}

func TestParseCrushShebang_FollowsKernelAndEnvRules(t *testing.T) {
	for line, want := range map[string][]string{
		"#!/bin/sh\n":                  {"/bin/sh"},
		"#! /bin/bash -eu\r\n":         {"/bin/bash", "-eu"},
		"#!/usr/bin/env bash\n":        {"bash"},
		"#!/usr/bin/env bash -x\n":     {"bash", "-x"},
		"#!/usr/bin/env -S bash -x -e": {"bash", "-x", "-e"},
		"#!/usr/bin/env python3 -u -W": {"python3", "-u -W"},
	} {
		program, args, err := parseCrushShebang([]byte(line))
		if err != nil || !slices.Equal(append([]string{program}, args...), want) {
			t.Errorf("%q = %s %q, %v; want %q", line, program, args, err, want)
		}
	}
	if _, _, err := parseCrushShebang([]byte("#!/usr/bin/env -i bash")); err == nil {
		t.Error("Crush rejects env flags other than -S")
	}
}

func TestCrushHandlers_ReadTheFlatHooksList(t *testing.T) {
	body := []byte(`{"hooks":{"PreToolUse":[{"name":"g","matcher":"^bash$","command":"g.sh","timeout":5}]}}`)
	handlers, err := CrushHandlers(body)
	if err != nil || len(handlers) != 1 || handlers[0].Command != "g.sh" || handlers[0].Timeout != 5*time.Second {
		t.Fatalf("handlers = %+v, %v", handlers, err)
	}
	drift, err := Drift("crush", body, "PreToolUse", "^bash$", "linux", handlers, nil)
	if err != nil || len(drift) != 0 {
		t.Errorf("no drift expected: %+v %v", drift, err)
	}
	drift, _ = Drift("crush", body, "PreToolUse", "^edit$", "linux", handlers, nil)
	if len(drift) != 1 || !strings.Contains(drift[0].Reason, `matcher "^bash$", not "^edit$"`) {
		t.Errorf("drift = %+v", drift)
	}
	if DefaultTimeout("crush", "PreToolUse") != 30*time.Second {
		t.Error("Crush defaults to 30 seconds")
	}
}

func TestCrushAddsContext_ReadsContextAsCrushDoes(t *testing.T) {
	for out, want := range map[string]bool{
		`{"context":"note"}`:                                    true,
		`{"context":["", "note"]}`:                              true,
		`{"context":["", ""]}`:                                  false,
		`{"context":[1]}`:                                       false,
		`{"context":5}`:                                         false,
		`{"context":"note","reason":5}`:                         false,
		`{"context":"note","decision":"deny"}`:                  false,
		`{"hookSpecificOutput":{"additionalContext":"note"}}`:   true,
		`{"hookSpecificOutput":null,"context":"note"}`:          true,
		`{"hookSpecificOutput":{"additionalContext":["note"]}}`: false,
	} {
		if got := AddsContext("crush", "PreToolUse", Result{Stdout: out}); got != want {
			t.Errorf("%s: AddsContext = %t, want %t", out, got, want)
		}
	}
}

func TestCrushJoin_KeepsRootedPathsOnWindows(t *testing.T) {
	dir := filepath.Join("project", "root")
	for _, tc := range []struct {
		goos, path, want string
	}{
		{"windows", "/scripts/guard.sh", "/scripts/guard.sh"},
		{"windows", `\scripts\guard.sh`, `\scripts\guard.sh`},
		{"windows", "./guard.sh", filepath.Join(dir, "guard.sh")},
		{"linux", "./guard.sh", filepath.Join(dir, "guard.sh")},
		{runtime.GOOS, "../guard.sh", filepath.Join(dir, "..", "guard.sh")},
	} {
		if got := crushJoin(tc.goos, dir, tc.path); got != tc.want {
			t.Errorf("crushJoin(%s, %q) = %q, want %q", tc.goos, tc.path, got, tc.want)
		}
	}
	if runtime.GOOS == "windows" {
		abs := filepath.Join(t.TempDir(), "guard.sh")
		if got := crushJoin("windows", dir, abs); got != abs {
			t.Errorf("a drive path stays as is: %q", got)
		}
	}
}

func TestRunCrushHooks_RunsEachCommandOnceAndAllAtOnce(t *testing.T) {
	dir := t.TempDir()
	count := "echo x >> count"
	handlers := CrushDedupe([]Handler{
		{Command: count},
		{Command: ": > a; while [ ! -f b ]; do :; done; exit 2"},
		{Command: count},
		{Command: ": > b; while [ ! -f a ]; do :; done"},
	})
	if len(handlers) != 3 {
		t.Fatalf("handlers = %+v", handlers)
	}
	results := RunCrushHooks(handlers, dir, os.Environ(), nil, 5*time.Second, runtime.GOOS)
	if results[1].Exit != 2 || results[2].Exit != 0 || results[1].TimedOut || results[2].TimedOut {
		t.Errorf("the two waiting handlers must run at once, in order: %+v", results)
	}
	got, err := os.ReadFile(filepath.Join(dir, "count"))
	if err != nil || string(got) != "x\n" {
		t.Errorf("a repeated command runs once: %q %v", got, err)
	}
}
