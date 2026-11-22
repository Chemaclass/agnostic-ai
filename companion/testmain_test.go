package main

import (
	"iter"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// A test never runs the agnostic-ai command of the machine it runs on;
	// fakeTerminalCommand names one for a test that needs it.
	terminalCommand = func() (string, error) { return "", errNoBinary }
	separateCommands = func() iter.Seq[string] { return func(func(string) bool) {} }
	os.Exit(m.Run())
}
