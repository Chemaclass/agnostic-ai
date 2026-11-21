package mailbox

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const lockWait = time.Second

var ErrBusy = errors.New("the mailbox is busy")

func (s Store) lock(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := privateDir(s.Dir); err != nil {
		return nil, err
	}
	path := filepath.Join(s.Dir, ".lock")
	info, err := os.Lstat(path)
	switch {
	case err == nil && !info.Mode().IsRegular():
		return nil, fmt.Errorf("%s: expected a regular lock file", path)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if info == nil || info.Mode().Perm() != 0o600 {
		if err := file.Chmod(0o600); err != nil {
			_ = file.Close()
			return nil, err
		}
	}
	waiting, cancel := context.WithTimeout(ctx, lockWait)
	defer cancel()
	retry := time.NewTicker(10 * time.Millisecond)
	defer retry.Stop()
	for waiting.Err() == nil {
		release, err := tryLock(file)
		if err == nil {
			if err := s.recoverTransfer(); err != nil {
				release()
				return nil, err
			}
			return release, nil
		}
		if !lockHeldElsewhere(err) {
			_ = file.Close()
			return nil, fmt.Errorf("lock %s: %w", path, err)
		}
		select {
		case <-waiting.Done():
		case <-retry.C:
		}
	}
	_ = file.Close()
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return nil, fmt.Errorf("lock %s: %w", path, ErrBusy)
}

func (s Store) lockForRead(ctx context.Context) (func(), bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if err := checkDir(s.Dir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	release, err := s.lock(ctx)
	if err != nil {
		return nil, false, err
	}
	return release, true, nil
}

func checkDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s: expected a directory", path)
	}
	return nil
}

func privateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s: expected a directory", path)
	}
	if info.Mode().Perm() != 0o700 {
		return os.Chmod(path, 0o700)
	}
	return nil
}
