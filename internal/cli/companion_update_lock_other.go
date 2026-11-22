//go:build !darwin && !linux

package cli

// lockCompanionFolder takes no lock: this system has no flock.
func lockCompanionFolder(string) (func(), error) {
	return func() {}, nil
}
