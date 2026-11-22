//go:build darwin || linux

package main

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// ownUnlockFile accepts only what the server writes: a regular file,
// not a link, that only this account can read.
func ownUnlockFile(info fs.FileInfo) error {
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return errors.New("not a private regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Getuid() {
		return errors.New("owned by another account")
	}
	return nil
}
