package main

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// A test never runs the agnostic-ai command of the machine it runs on;
	// fakeTerminalCommand names one for a test that needs it.
	terminalCommand = func() (string, error) { return "", errNoBinary }
	os.Exit(m.Run())
}
