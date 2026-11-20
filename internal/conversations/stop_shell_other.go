//go:build !darwin && !linux

package conversations

func signalShellGroup(int, bool) error {
	return ErrShellRefused
}

func signalSession(int, bool) error {
	return ErrSessionRefused
}
