import { describe, expect, it } from 'vitest'
import type { TargetSupport } from '../contract'
import { syncSummary } from './sync-status'

const tool = (target: string, label: string, state: TargetSupport['sync']['state'], files: string[] = []): TargetSupport => ({
  target, label, kinds: [], sync: { state, files: files.map((path) => ({ path, state: 'stale' })) },
})

describe('generated-file status', () => {
  it('reports a match only when every configured tool was compared and matches', () => {
    const summary = syncSummary([tool('claude', 'Claude Code', 'in-sync'), tool('codex', 'Codex', 'in-sync')])
    expect(summary.tone).toBe('match')
    expect(summary.headline).toBe('Generated files match the source for 2 tools.')
  })

  it('names the tools that need sync before any that were not checked', () => {
    const summary = syncSummary([tool('claude', 'Claude Code', 'drifted', ['CLAUDE.md']), tool('codex', 'Codex', 'not-checked'), tool('cursor', 'Cursor', 'in-sync')])
    expect(summary.tone).toBe('needs-sync')
    expect(summary.headline).toBe('Claude Code needs sync.')
    expect(summary.detail).toBe('Codex was not checked.')
  })

  it('never reports a match when a tool was not checked', () => {
    const summary = syncSummary([tool('claude', 'Claude Code', 'in-sync'), tool('ext', 'External', 'not-checked')])
    expect(summary.tone).toBe('unknown')
    expect(summary.headline).toBe('External was not checked.')
  })

  it('never reports a match for an empty tool list', () => {
    const summary = syncSummary([])
    expect(summary.tone).toBe('unknown')
    expect(summary.headline).toBe('No coding tools are configured, so no generated files were checked.')
  })
})
