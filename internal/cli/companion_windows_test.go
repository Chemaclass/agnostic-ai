package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
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
	if err := key.SetStringValue(companionName, `"C:\gone\agnostic-companion.exe"`); err != nil {
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

func TestReadLegacyRunValue_CanRestoreTheEarlierCommandAndType(t *testing.T) {
	path := fmt.Sprintf(`Software\agnostic-ai-test-%d`, time.Now().UnixNano())
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = key.Close()
		_ = registry.DeleteKey(registry.CURRENT_USER, path)
	})
	const command = `"%APPDATA%\agnostic-ai\companion\agnostic-ai-companion.exe"`
	if err := key.SetExpandStringValue(legacyCompanionName, command); err != nil {
		t.Fatal(err)
	}
	if err := key.SetStringValue(companionName, `"C:\new\agnostic-companion.exe"`); err != nil {
		t.Fatal(err)
	}
	value, err := readLegacyRunValue(registry.CURRENT_USER, path, path+`\Approval`)
	if err != nil || value.remove == nil || value.restore == nil {
		t.Fatalf("no earlier value: %v", err)
	}
	if err := value.remove(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := key.GetStringValue(legacyCompanionName); !errors.Is(err, registry.ErrNotExist) {
		t.Errorf("earlier value remains: %v", err)
	}
	if err := value.restore(); err != nil {
		t.Fatal(err)
	}
	if got, kind, err := key.GetStringValue(legacyCompanionName); got != command || kind != registry.EXPAND_SZ || err != nil {
		t.Errorf("restored command = %q (%d), %v", got, kind, err)
	}
	if got, _, err := key.GetStringValue(companionName); got != `"C:\new\agnostic-companion.exe"` || err != nil {
		t.Errorf("new value = %q, %v", got, err)
	}
}

func TestReadLegacyRunValue_KeepsWindowsStartupApprovalUntouched(t *testing.T) {
	path := fmt.Sprintf(`Software\agnostic-ai-test-%d`, time.Now().UnixNano())
	approvalPath := path + `\Approval`
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = key.Close()
		_ = registry.DeleteKey(registry.CURRENT_USER, path)
	})
	approval, _, err := registry.CreateKey(registry.CURRENT_USER, approvalPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = approval.Close()
		_ = registry.DeleteKey(registry.CURRENT_USER, approvalPath)
	})
	for _, tc := range []struct {
		name    string
		data    []byte
		enabled bool
		invalid bool
	}{
		{name: "enabled", data: []byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, enabled: true},
		{name: "disabled", data: []byte{3, 0, 0, 0, 12, 34, 56, 78, 90, 0, 0, 0}},
		{name: "unrecognized", data: []byte{1}, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := key.SetStringValue(legacyCompanionName, `C:\old\agnostic-ai-companion.exe`); err != nil {
				t.Fatal(err)
			}
			if err := approval.SetBinaryValue(legacyCompanionName, tc.data); err != nil {
				t.Fatal(err)
			}
			value, err := readLegacyRunValue(registry.CURRENT_USER, path, approvalPath)
			if tc.invalid {
				if err == nil || value.remove != nil {
					t.Errorf("unrecognized approval was used: %v", err)
				}
				return
			}
			if err != nil || value.remove == nil || value.enabled != tc.enabled {
				t.Fatalf("enabled = %t, %v, want %t", value.enabled, err, tc.enabled)
			}
			if err := value.remove(); err != nil {
				t.Fatal(err)
			}
			if data, _, err := approval.GetBinaryValue(legacyCompanionName); err != nil || !bytes.Equal(data, tc.data) {
				t.Errorf("Windows startup approval changed to %v: %v", data, err)
			}
		})
	}
}

func TestLockCompanionFolder_BlocksAnotherWindowsWriter(t *testing.T) {
	folder := t.TempDir()
	release, err := lockCompanionFolder(folder)
	if err != nil {
		t.Fatal(err)
	}
	other := make(chan error, 1)
	go func() {
		releaseOther, err := lockCompanionFolder(folder)
		if err == nil {
			releaseOther()
		}
		other <- err
	}()
	err = <-other
	release()
	if err == nil || !strings.Contains(err.Error(), "another install or update") {
		t.Errorf("second writer = %v, want a lock conflict", err)
	}
	release, err = lockCompanionFolder(folder)
	if err != nil {
		t.Fatal(err)
	}
	release()
}
