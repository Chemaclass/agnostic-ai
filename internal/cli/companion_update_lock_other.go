//go:build !darwin && !linux && !windows

package cli

// lockCompanionFolder takes no lock on systems without a companion build.
func lockCompanionFolder(string) (func(), error) {
	return func() {}, nil
}
