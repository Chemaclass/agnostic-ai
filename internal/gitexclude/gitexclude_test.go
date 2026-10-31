package gitexclude

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestUpdatePreservesConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "info", "exclude")
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := Update(path, func(content string) string { return content + fmt.Sprintf("entry-%d\n", i) }); err != nil {
				t.Errorf("update %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		if !strings.Contains(string(data), fmt.Sprintf("entry-%d\n", i)) {
			t.Errorf("missing entry %d", i)
		}
	}
}

func TestGuardUpdatesUnderHeldLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "info", "exclude")
	guard, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Release()
	if err := guard.Update(path, func(string) string { return "private/\n" }); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "private/\n" {
		t.Errorf("got %q", data)
	}
	other := filepath.Join(t.TempDir(), "info", "exclude")
	if err := guard.Update(other, func(string) string { return "unsafe" }); err == nil {
		t.Error("expected an error for a different lock path")
	}
}

func TestUpdateReplacesTheFileWhole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exclude")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	held := filepath.Join(dir, "held")
	if err := os.Link(path, held); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if err := Update(path, func(c string) string { return c + "new\n" }); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(held); string(got) != "old\n" {
		t.Errorf("file was rewritten in place: %q", got)
	}
}
