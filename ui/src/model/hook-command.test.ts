import { readdirSync, readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { launcherCall } from './hook-command'

const hooksDir = new URL('../../../internal/builtins/data/memory/hooks/', import.meta.url)

/** Every double-quoted command and commandWindows the built-in memory hooks ship, top level and per tool. */
function shippedCommands(): string[] {
  const commands: string[] = []
  for (const file of readdirSync(hooksDir)) {
    for (const line of readFileSync(new URL(file, hooksDir), 'utf8').split('\n')) {
      const quoted = line.match(/^\s*command(?:Windows)?: (".*")$/)?.[1]
      if (quoted) commands.push(JSON.parse(quoted) as string)
    }
  }
  return commands
}

describe('launcherCall', () => {
  it('reads the agnostic-ai call at the end of every shipped launcher, in sh and PowerShell', () => {
    const launchers = shippedCommands().filter((command) => command.includes('node_modules/.bin/agnostic-ai'))
    expect(launchers.some((command) => command.startsWith('sh -c'))).toBe(true)
    expect(launchers.some((command) => command.startsWith('$d = (Get-Location)'))).toBe(true)
    for (const command of launchers) expect(launcherCall(command)).toMatch(/^agnostic-ai project -- hook memory --target [a-z]+$/)
  })

  it('leaves other commands alone, including other tools run from node_modules', () => {
    expect(launcherCall('npm test')).toBeUndefined()
    expect(launcherCall(`sh -c 'bin=eslint; [ -x node_modules/.bin/eslint ] && bin=node_modules/.bin/eslint; exec "$bin" --fix .'`)).toBeUndefined()
    expect(launcherCall('$bin = "node_modules/.bin/prettier.cmd"; & $bin --check .; exit $LASTEXITCODE')).toBeUndefined()
  })
})
