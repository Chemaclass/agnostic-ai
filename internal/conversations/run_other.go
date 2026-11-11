//go:build !darwin && !linux

package conversations

import "os"

// ownedByMe has no owner to compare on these systems; the briefs
// folder sits in the user's own config folder.
func ownedByMe(os.FileInfo) bool { return true }

// ExecTool cannot replace a process here, and no tab opens here either.
func ExecTool(string, []string, []string) error { return errNotHere }
