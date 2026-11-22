//go:build !windows

package main

import "os"

func readFileShared(path string) ([]byte, error) {
	return os.ReadFile(path)
}
