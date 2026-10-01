package adapters

import (
	"strings"
	"testing"
)

func TestResolve_UnknownTargetSuggestsClosestBuiltIn(t *testing.T) {
	_, err := Resolve("claud")
	if err == nil {
		t.Fatal("expected an error for an unknown target")
	}
	if !strings.Contains(err.Error(), "did you mean claude?") {
		t.Errorf("error lacks a suggestion: %v", err)
	}
	if !strings.Contains(err.Error(), "agnostic-ai-adapter-claud on PATH") {
		t.Errorf("error lost the external adapter hint: %v", err)
	}
}
