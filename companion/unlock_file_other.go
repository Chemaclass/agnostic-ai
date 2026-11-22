//go:build !darwin && !linux

package main

import (
	"errors"
	"io/fs"
)

// ownUnlockFile refuses every file: the server writes none where a file
// mode does not keep other accounts out.
func ownUnlockFile(fs.FileInfo) error {
	return errors.New("unlock files are not used on this system")
}
