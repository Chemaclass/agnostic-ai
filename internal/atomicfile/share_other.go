//go:build !windows

package atomicfile

import "os"

func rename(from, to string) error {
	return os.Rename(from, to)
}

// OpenShared opens path for reading without blocking a Replace or Write of it.
func OpenShared(path string) (*os.File, error) {
	return os.Open(path)
}
