import { describe, expect, it } from 'vitest'
import { LINT_INCOMPLETE, type ItemFinding, type OverviewView, type SectionItem } from '../contract'
import { findingEntryIds, itemEntryId } from './identity'
import { recheckAllMessage, recheckAllResult, recheckMessage, recheckResult, type RecheckTarget } from './recheck'

const read: ItemFinding = { source: 'lint', code: 'LINT038', severity: 'warning', message: 'Bare read covers every file.', path: 'permissions.yaml', line: 2 }
const edit: ItemFinding = { ...read, message: 'Bare edit covers every file.', line: 3 }
const shell: ItemFinding = { ...read, message: 'Bare shell covers every command.', line: 4 }
const item = (findings: ItemFinding[]): SectionItem => ({ kind: 'settings', name: 'permissions', label: 'permissions', description: '', path: 'permissions.yaml', layer: 'project', values: [], findings })
const view = (findings: ItemFinding[], sectionFindings: ItemFinding[] = []): OverviewView => ({
  schemaVersion: 'overview/v1',
  snapshot: {} as OverviewView['snapshot'],
  targets: [], tools: { source: 'default', items: [] },
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

  it('does not call existing findings new when their spec is renamed', () => {
    const before = view([read, edit])
    const after = view([read, edit, shell])
    after.sections[0].items[0].name = 'renamed'
    const result = recheckResult(target(read, [read, edit]), before, after)
    expect(result.state).toBe('remaining')
    expect(result.added.map((finding) => finding.finding)).toEqual([shell])
  })

  it('reports an added copy of an existing finding', () => {
    const result = recheckResult(target(read, [read]), view([read]), view([read, { ...read, line: 7 }]))
    expect(result.added).toHaveLength(1)
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

  it('does not report a stopped lint finding as fixed when lint stops for another reason', () => {
    const first: ItemFinding = { source: 'lint', code: LINT_INCOMPLETE, severity: 'error', message: 'Lint stopped before it finished: bad config', path: 'agnostic-ai.yaml' }
    const second: ItemFinding = { ...first, message: 'Lint stopped before it finished: invalid source' }
    const result = recheckResult(target(first, [first]), view([first]), view([], [second]))
    expect(result).toEqual({ state: 'incomplete', added: [] })
  })

  it('reads a stopped lint run only in the layer it ran on', () => {
    const globalStopped: ItemFinding = { source: 'lint', code: LINT_INCOMPLETE, severity: 'error', message: 'The global setup did not load: bad config', path: 'agnostic-ai.yaml', layer: 'global' }
    expect(recheckResult(target(read, [read]), view([read]), view([], [globalStopped]))).toEqual({ state: 'resolved', added: [] })
    const globalRead: ItemFinding = { ...read, layer: 'global' }
    const projectStopped: ItemFinding = { ...globalStopped, layer: undefined }
    const globalTarget: RecheckTarget = { id: findingEntryIds([globalRead])[0], finding: globalRead }
    expect(recheckResult(globalTarget, view([], [globalRead]), view([], [projectStopped]))).toEqual({ state: 'resolved', added: [] })
    expect(recheckResult(globalTarget, view([], [globalRead]), view([], [globalStopped]))).toEqual({ state: 'incomplete', added: [] })
  })

  it('names new findings on the same file only in the same layer', () => {
    const config: ItemFinding = { source: 'lint', code: 'LINT006', severity: 'warning', message: 'Unknown key.', path: 'agnostic-ai.yaml' }
    const global: ItemFinding = { ...config, message: 'Unknown global key.', layer: 'global' }
    const project: ItemFinding = { ...config, message: 'Another unknown key.' }
    const configTarget: RecheckTarget = { id: findingEntryIds([config])[0], finding: config }
    const result = recheckResult(configTarget, view([], [config]), view([], [global, project]))
    expect(result.added.map((a) => a.finding)).toEqual([project])
  })

  it('keeps a finding that moved from its item to the section', () => {
    const result = recheckResult(target(read, [read]), view([read]), view([], [{ ...read, line: 5 }]))
    expect(result).toEqual({ state: 'remaining', id: findingEntryIds([read])[0], added: [] })
  })

  it('does not confirm a doctor fix, because doctor checks skip unreadable files', () => {
    const trust: ItemFinding = { source: 'doctor', code: 'codex-hook-trust', severity: 'error', message: 'Codex hook is untrusted.', path: '.codex/hooks.json' }
    const result = recheckResult(target(trust, [trust]), view([trust]), view([]))
    expect(result.state).toBe('unconfirmed')
    expect(recheckMessage(result, trust)).toBe('This doctor finding no longer appears, but this check skips files it cannot read, so this does not confirm a fix.')
  })

  it('says a failed refresh proves nothing', () => {
    expect(recheckMessage({ state: 'failed', added: [] }, read)).toBe('The recheck did not finish, so this finding may still apply.')
  })
})

describe('recheck all', () => {
  const doctor: ItemFinding = { source: 'doctor', severity: 'error', message: 'Nested CLAUDE.md repeats a rule.', path: 'docs/CLAUDE.md' }

  it('counts what no longer appears and what remains across sections', () => {
    const result = recheckAllResult(view([read, edit], [doctor]), view([edit], [doctor]))
    expect(result).toEqual({ state: 'done', resolved: 1, unconfirmed: 0, unknown: 0, added: 0, remaining: 2 })
    expect(recheckAllMessage(result)).toBe('Rechecked every finding. 1 finding no longer appears. 2 findings remain.')
  })

  it('names new findings and doctor findings it cannot confirm', () => {
    const result = recheckAllResult(view([read], [doctor]), view([read, shell]))
    expect(result).toMatchObject({ resolved: 0, unconfirmed: 1, added: 1, remaining: 2 })
    expect(recheckAllMessage(result)).toBe('Rechecked every finding. 1 doctor finding no longer appears, but doctor skips files it cannot read, so this does not confirm a fix. 1 new finding appears. 2 findings remain.')
  })

  it('does not count lint findings as fixed when lint stopped', () => {
    const stopped: ItemFinding = { source: 'lint', code: LINT_INCOMPLETE, severity: 'error', message: 'Lint stopped before it finished: bad config', path: 'agnostic-ai.yaml' }
    const result = recheckAllResult(view([read, edit]), view([], [stopped]))
    expect(result).toMatchObject({ resolved: 0, unknown: 2, added: 0, remaining: 1 })
    expect(recheckAllMessage(result)).toBe('Rechecked every finding. Lint stopped before it finished, so 2 lint findings that no longer appear may still apply. 1 finding remains.')
  })

  it('judges a gone lint finding by the lint run of its own layer', () => {
    const stopped: ItemFinding = { source: 'lint', code: LINT_INCOMPLETE, severity: 'error', message: 'Lint stopped before it finished: bad config', path: 'agnostic-ai.yaml', layer: 'global' }
    expect(recheckAllResult(view([read]), view([], [stopped]))).toMatchObject({ resolved: 1, unknown: 0 })
    expect(recheckAllResult(view([], [{ ...read, layer: 'global' }]), view([], [stopped]))).toMatchObject({ resolved: 0, unknown: 1 })
  })

  it('matches identical findings one to one', () => {
    const twice = [read, { ...read, line: 7 }]
    expect(recheckAllResult(view(twice), view([read, shell]))).toMatchObject({ resolved: 1, added: 1, remaining: 2 })
    expect(recheckAllResult(view(twice), view([read]))).toMatchObject({ resolved: 1, added: 0, remaining: 1 })
    expect(recheckAllResult(view([read]), view(twice))).toMatchObject({ resolved: 0, added: 1, remaining: 2 })
  })

  it('says when nothing changed and when the refresh failed', () => {
    expect(recheckAllMessage(recheckAllResult(view([read]), view([read])))).toBe('Rechecked every finding. Nothing changed. 1 finding remains.')
    expect(recheckAllMessage({ state: 'failed', resolved: 0, unconfirmed: 0, unknown: 0, added: 0, remaining: 0 })).toBe('The recheck did not finish, so every finding may still apply.')
  })
})
