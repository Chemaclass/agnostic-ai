//go:build !darwin && !linux

package conversations

func ownSocket(string) bool { return false }
