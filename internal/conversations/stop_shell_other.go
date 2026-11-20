//go:build !darwin && !linux

package conversations

func signalShellGroup(int, bool) error {
	return ErrShellRefused
}
