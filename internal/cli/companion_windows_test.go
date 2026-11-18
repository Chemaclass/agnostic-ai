package cli

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

func TestRemoveRunValue_DeletesTheValueAndReportsIt(t *testing.T) {
	path := fmt.Sprintf(`Software\agnostic-ai-test-%d`, time.Now().UnixNano())
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = key.Close()
		_ = registry.DeleteKey(registry.CURRENT_USER, path)
	})
	if err := key.SetStringValue(companionName, `"C:\gone\agnostic-ai-companion.exe"`); err != nil {
		t.Fatal(err)
	}

	removed, err := removeRunValue(registry.CURRENT_USER, path, companionName)

	if err != nil || !removed {
		t.Fatalf("removed %v, err %v", removed, err)
	}
	if _, _, err := key.GetStringValue(companionName); !errors.Is(err, registry.ErrNotExist) {
		t.Errorf("the value is still there: %v", err)
	}
	removed, err = removeRunValue(registry.CURRENT_USER, path, companionName)
	if err != nil || removed {
		t.Errorf("a second removal: removed %v, err %v", removed, err)
	}
}

func TestRemoveRunValue_SucceedsWithoutTheKey(t *testing.T) {
	removed, err := removeRunValue(registry.CURRENT_USER, fmt.Sprintf(`Software\agnostic-ai-test-missing-%d`, time.Now().UnixNano()), companionName)

	if err != nil || removed {
		t.Errorf("removed %v, err %v", removed, err)
	}
}
