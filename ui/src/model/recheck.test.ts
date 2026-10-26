import { describe, expect, it } from 'vitest'
import { LINT_INCOMPLETE, type ItemFinding, type OverviewView, type SectionItem } from '../contract'
import { findingEntryIds, itemEntryId } from './identity'
import { recheckMessage, recheckResult, type RecheckTarget } from './recheck'

const read: ItemFinding = { source: 'lint', code: 'LINT038', severity: 'warning', message: 'Bare read covers every file.', path: 'permissions.yaml', line: 2 }
const edit: ItemFinding = { ...read, message: 'Bare edit covers every file.', line: 3 }
const shell: ItemFinding = { ...read, message: 'Bare shell covers every command.', line: 4 }
const item = (findings: ItemFinding[]): SectionItem => ({ kind: 'settings', name: 'permissions', label: 'permissions', description: '', path: 'permissions.yaml', layer: 'project', values: [], findings })
const view = (findings: ItemFinding[], sectionFindings: ItemFinding[] = []): OverviewView => ({
  schemaVersion: 'overview/v1',
  snapshot: {} as OverviewView['snapshot'],
  targets: [],
  sections: [{ id: 'authority', label: 'Authority', question: '', order: 1, kinds: [], items: [item(findings)], support: [], findings: sectionFindings }],
})
const owner = itemEntryId(item([]))
const target = (finding: ItemFinding, findings: ItemFinding[]): RecheckTarget => ({ id: findingEntryIds(findings, owner)[findings.indexOf(finding)], finding })

describe('recheck', () => {
  it('reports a finding that no longer appears as resolved', () => {
    const before = view([read, edit])
    expect(recheckResult(target(read, [read, edit]), before, view([edit]))).toEqual({ state: 'resolved', added: [] })
  })

  it('keeps a finding whose line moved', () => {
    const t = target(edit, [read, edit])
    const result = recheckResult(t, view([read, edit]), view([{ ...edit, line: 9 }]))
    expect(result).toEqual({ state: 'remaining', id: t.id, added: [] })
  })

  it('names new findings on the same file from the same check', () => {
    const result = recheckResult(target(read, [read, edit]), view([read, edit]), view([edit, shell], [{ ...shell, source: 'doctor' }]))
    expect(result.state).toBe('resolved')
    expect(result.added.map((a) => a.finding)).toEqual([shell])
    expect(recheckMessage(result, read)).toBe('This lint finding no longer appears. 1 new lint finding appears in permissions.yaml.')
  })

  it('keeps one of two identical findings as remaining when one copy is fixed', () => {
    const twice = [read, { ...read, line: 7 }]
    const second = target(twice[1], twice)
    const result = recheckResult(second, view(twice), view([read]))
    expect(result.state).toBe('remaining')
    expect(recheckMessage(result, read)).toBe('This finding still appears after the recheck. 1 of 2 identical findings still appears.')
  })

  it('does not treat a lint run that stopped as a fix', () => {
    const stopped: ItemFinding = { source: 'lint', code: LINT_INCOMPLETE, severity: 'error', message: 'Lint stopped before it finished: bad config', path: 'agnostic-ai.yaml' }
    const result = recheckResult(target(read, [read]), view([read]), view([], [stopped]))
    expect(result).toEqual({ state: 'incomplete', added: [] })
    expect(recheckMessage(result, read)).toBe('Lint stopped before it finished, so this recheck cannot tell whether the finding is fixed. The last known finding is shown.')
  })

  it('says a failed refresh proves nothing', () => {
    expect(recheckMessage({ state: 'failed', added: [] }, read)).toBe('The recheck did not finish, so this finding may still apply.')
  })
})
