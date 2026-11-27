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
	// Qoder requires hookEventName in hookSpecificOutput (docs.qoder.com/cli/hooks).
	"qoder": `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`,
}

// PermissionAllowReply is target's PermissionRequest reply that allows
// one call. ok is false for a target with no such reply.
func PermissionAllowReply(target string) (reply []byte, ok bool) {
	text, ok := permissionAllowReplies[target]
	return []byte(text), ok
}

// CompanionDenyMessage is what the agent reads when the person denies a
// request in the companion.
const CompanionDenyMessage = "Denied in Agnostic Companion"

// permissionDenyReplies block one call with CompanionDenyMessage. Only
// the tools whose deny was measured have one (decision 022).
var permissionDenyReplies = map[string]string{
	"claude": `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"deny","message":"` + CompanionDenyMessage + `"}}}`,
	"codex":  `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"deny","message":"` + CompanionDenyMessage + `"}}}`,
}

// PermissionDenyReply is target's PermissionRequest reply that blocks
// one call. ok is false for a target with no such reply.
func PermissionDenyReply(target string) (reply []byte, ok bool) {
	text, ok := permissionDenyReplies[target]
	return []byte(text), ok
}
