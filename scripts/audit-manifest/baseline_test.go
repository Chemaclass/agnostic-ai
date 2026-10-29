package main

import (
	"path/filepath"
	"testing"
)

func TestStartResolvesAPerTargetBuiltinBaseline(t *testing.T) {
	a, _ := fixture(t)
	head := call(t, a, "start", "--out", filepath.Join(t.TempDir(), "probe"), "--builtins-since", "HEAD", "alpha")
	m, err := load(head)
	if err != nil {
		t.Fatal(err)
	}
	commit := m.BuiltinsSince

	run := call(t, a, "start", "--out", filepath.Join(t.TempDir(), "run"), "--builtins-since", "beta=HEAD,alpha=HEAD", "alpha", "beta")
	m, err = load(run)
	if err != nil {
		t.Fatal(err)
	}
	if want := "beta=" + commit + ",alpha=" + commit; m.BuiltinsSince != want {
		t.Errorf("builtins_since = %q, want resolved commits %q", m.BuiltinsSince, want)
	}
}

func TestStartRejectsAnInvalidPerTargetBuiltinBaseline(t *testing.T) {
	a, _ := fixture(t)
	for _, baseline := range []string{"gamma=HEAD", "alpha=no-such-rev", "alpha=HEAD,alpha=HEAD", "alpha=", "alpha=HEAD,"} {
		if _, err := a.run([]string{"start", "--out", filepath.Join(t.TempDir(), "run"), "--builtins-since", baseline, "alpha", "beta"}); err == nil {
			t.Errorf("start accepted built-in baseline %q", baseline)
		}
	}
}
