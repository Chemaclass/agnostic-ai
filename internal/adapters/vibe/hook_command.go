package vibe

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
)

func hookCommand(command, platform string) string {
	return emit.NativeHookCommand(command, target, platform)
}
func DeclaredHookCommand(command string) string {
	return emit.DeclaredNativeHookCommand(command, target)
}
func nativeHookCommand(command string, args []string, platform string) (string, error) {
	if platform == "windows" && !emit.PowerShellReadsFold(command, args) {
		return "", fmt.Errorf("arguments cannot keep their exact values in Windows PowerShell; put them in a native command instead")
	}
	return hookCommand(emit.ExecFormCommand(command, args), platform), nil
}
