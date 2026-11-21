//go:build !windows

package conversations

// startInNewConsole has no console to open outside Windows.
func startInNewConsole(string, ...string) error { return errNotHere }
