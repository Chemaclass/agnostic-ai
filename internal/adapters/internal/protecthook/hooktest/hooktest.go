// Package hooktest runs a generated protect hook under every sh and awk
// the machine has, for the adapter tests that feed it real payloads.
package hooktest

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// Result is one hook run's exit code and output.
type Result struct {
	Code           int
	Stdout, Stderr string
}

// Runtime is one shell plus the PATH that picks its awk, and any other
// environment the run adds.
type Runtime struct {
	Name, Sh, Path string
	Env            []string
}

// Runtimes lists sh and dash, each with the system awk and with every
// other awk on PATH (mawk, gawk, busybox), so the script runs as macOS
// and Debian ship it.
func Runtimes(t *testing.T) []Runtime {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the generated hook needs a POSIX shell")
	}
	paths := map[string]string{"awk": "/usr/bin:/bin"}
	for _, name := range []string{"mawk", "gawk", "busybox"} {
		bin, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		dir := t.TempDir()
		link := filepath.Join(dir, "awk")
		if name == "busybox" {
			if err := os.WriteFile(link, []byte("#!/bin/sh\nexec "+bin+" awk \"$@\"\n"), 0o755); err != nil {
				t.Fatal(err)
			}
		} else if err := os.Symlink(bin, link); err != nil {
			t.Fatal(err)
		}
		paths[name] = dir + ":/usr/bin:/bin"
	}
	var out []Runtime
	for _, shell := range []string{"sh", "dash"} {
		sh, err := exec.LookPath(shell)
		if err != nil {
			continue
		}
		for awk, path := range paths {
			out = append(out, Runtime{Name: shell + "+" + awk, Sh: sh, Path: path})
		}
	}
	if len(out) == 0 {
		t.Skip("no sh on PATH")
	}
	slices.SortFunc(out, func(a, b Runtime) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// RunAll runs script from dir with payload on stdin under every
// runtime and fails the test when they disagree.
func RunAll(t *testing.T, script, dir string, payload []byte) Result {
	t.Helper()
	runtimes := Runtimes(t)
	first := Run(t, runtimes[0], script, dir, payload)
	for _, rt := range runtimes[1:] {
		if got := Run(t, rt, script, dir, payload); got != first {
			t.Errorf("%s: %+v, but %s: %+v", rt.Name, got, runtimes[0].Name, first)
		}
	}
	return first
}

// Run runs script under rt from dir with payload on stdin.
func Run(t *testing.T, rt Runtime, script, dir string, payload []byte) Result {
	t.Helper()
	cmd := exec.Command(rt.Sh, script)
	cmd.Dir = dir
	cmd.Env = append([]string{"PATH=" + rt.Path}, rt.Env...)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return Result{Code: 0, Stdout: stdout.String(), Stderr: stderr.String()}
	case errors.As(err, &exit):
		return Result{Code: exit.ExitCode(), Stdout: stdout.String(), Stderr: stderr.String()}
	}
	t.Fatalf("run hook: %v", err)
	return Result{}
}
