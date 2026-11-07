// Built-in hooks run agnostic-ai through a launcher that finds the project
// root and its pinned binary first. The part a reader needs is the final call.

const SH_CALL = /exec "\$bin" (.+)'$/
const POWERSHELL_CALL = /& \$bin ([^;]+); exit \$LASTEXITCODE$/

/** "agnostic-ai project -- hook memory --target codex" for a launcher command, or undefined for any other command. */
export function launcherCall(command: string): string | undefined {
  const call = command.match(SH_CALL)?.[1] ?? command.match(POWERSHELL_CALL)?.[1]
  return call && command.includes('node_modules/.bin/agnostic-ai') ? `agnostic-ai ${call}` : undefined
}
