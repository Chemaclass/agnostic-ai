package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const projectLockName = ".command-lock"

func acquireProjectLock(root, command string) (*os.File, error) {
	dir := filepath.Join(root, defaultBaseDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	path := filepath.Join(dir, projectLockName)
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := tryLockProjectFile(file); err != nil {
		defer func() { _ = file.Close() }()
		if !projectLockContended(err) {
			return nil, fmt.Errorf("lock %s: %w", path, err)
		}
		holder := projectLockHolder(file)
		return nil, fmt.Errorf("%s: project is locked by agnostic-ai %s; retry when it finishes: %w", path, holder, err)
	}
	// Byte zero is reserved for Windows locking so contenders can read the holder.
	metadata := fmt.Sprintf("%-128s", fmt.Sprintf("%s (pid %d)", command, os.Getpid()))
	if _, err := file.WriteAt([]byte(metadata), 1); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("write lock holder %s: %w", path, err)
	}
	if err := ensureManagedGitignore(root); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("ignore project lock: %w", err)
	}
	return file, nil
}

func projectLockHolder(file *os.File) string {
	for range 10 {
		data := make([]byte, 128)
		n, err := file.ReadAt(data, 1)
		if err == nil || err == io.EOF {
			if holder := strings.TrimSpace(string(data[:n])); holder != "" {
				return holder
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return "another command"
}
