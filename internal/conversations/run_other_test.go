//go:build !darwin && !linux

package conversations

import (
	"errors"
	"testing"
)

func TestExecTool_RejectsUnsupportedSystems(t *testing.T) {
	if err := ExecTool("unused", nil, nil); !errors.Is(err, errNotHere) {
		t.Errorf("ExecTool error = %v, want %v", err, errNotHere)
	}
}
