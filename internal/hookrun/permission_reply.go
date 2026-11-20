package hookrun

// permissionAllowReplies are the PermissionRequest replies that allow
// one call and nothing else. Codex refuses updatedInput,
// updatedPermissions and interrupt, so no reply carries them.
var permissionAllowReplies = map[string]string{
	"claude": `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`,
	"codex":  `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`,
	// Kiro's reply has no hookEventName (kiro.dev/docs/hooks/types.md).
	"kiro": `{"hookSpecificOutput":{"decision":{"behavior":"allow"}}}`,
	// Qwen Code reads the same shape (docs/users/features/hooks.md).
	"qwen": `{"hookSpecificOutput":{"decision":{"behavior":"allow"}}}`,
}

// PermissionAllowReply is target's PermissionRequest reply that allows
// one call. ok is false for a target with no such reply.
func PermissionAllowReply(target string) (reply []byte, ok bool) {
	text, ok := permissionAllowReplies[target]
	return []byte(text), ok
}
