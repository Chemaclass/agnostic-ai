//go:build !darwin && !linux

package conversations

// environmentReader is nil here: this system has no reader for another
// process's environment.
var environmentReader environmentRead
