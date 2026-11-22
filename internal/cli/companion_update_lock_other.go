//go:build !darwin && !linux

package cli

// lockCompanionFolder takes no lock: no lock is taken off macOS and Linux.
func lockCompanionFolder(string) (func(), error) {
	return func() {}, nil
}
