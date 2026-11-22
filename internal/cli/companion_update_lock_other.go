//go:build !darwin && !linux

package cli

// lockCompanionUpdate takes no lock: update-app runs on macOS only.
func lockCompanionUpdate(string) (func(), error) {
	return func() {}, nil
}
