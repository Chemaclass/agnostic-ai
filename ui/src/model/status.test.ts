import { describe, expect, it } from 'vitest'
import type { Operation, Resource } from '../contract'
import { findResource } from './domains'
import { actionWeight, isReadLike, resourceStatus } from './status'
import { loadAuthorityFixture, usingMockAuthority } from './testing'

function resource(ops: Array<Partial<Operation> & Pick<Operation, 'action' | 'label' | 'effect'>>): Resource {
  return {
    id: 'r',
    label: 'R',
    kind: 'declared-root',
    providerId: 'mcp-x',
    domainIds: [],
    identity: {},
    basis: 'declared-root',
    labelResolved: true,
    operations: ops.map((o, i) => ({
      id: `r:${i}`,
      declared: o.effect,
      resolution: 'translated',
      installed: 'in-sync',
      hypothetical: false,
      runtime: 'not-observed',
      grants: [],
      conditions: [],
      alternateRoutes: [],
      routeCoverage: { checked: [], notModeled: [] },
      enforcement: [],
      explanation: '',
      ...o,
    })),
  }
}

describe('action weight', () => {
  it('ranks destructive over state-changing over reading actions', () => {
    expect(actionWeight('delete')).toBe(5)
    expect(actionWeight('merge_pull_request')).toBe(5)
    expect(actionWeight('deploy')).toBe(4)
    expect(actionWeight('create_pull_request')).toBe(4)
    expect(actionWeight('logs')).toBe(1)
    expect(actionWeight('list_pull_request_files')).toBe(1)
    expect(actionWeight('frobnicate')).toBe(3)
    expect(isReadLike('get_pull_request')).toBe(true)
  })
})

describe('resource status pill', () => {
  it('names the most consequential permitted operation', () => {
    const status = resourceStatus(
      resource([
        { action: 'logs', label: 'Read logs', effect: 'allowed' },
        { action: 'deploy', label: 'Deploy', effect: 'ask' },
        { action: 'delete', label: 'Delete', effect: 'denied' },
      ]),
    )
    expect(status).toMatchObject({ text: 'Deploy permitted, approval required', tone: 'approval', operationId: 'r:1' })
  })

  it('prefers the operation that needs no human when weights tie', () => {
    const status = resourceStatus(
      resource([
        { action: 'variables', label: 'Manage variables', effect: 'ask' },
        { action: 'deploy', label: 'Deploy', effect: 'allowed' },
      ]),
    )
    expect(status.text).toBe('Deploy permitted')
    expect(status.tone).toBe('permitted')
  })

  it('says read only when every permitted operation reads and nothing else is open', () => {
    expect(resourceStatus(resource([{ action: 'read', label: 'Read', effect: 'allowed' }, { action: 'edit', label: 'Edit', effect: 'denied' }]))).toMatchObject({
      text: 'Read only',
      tone: 'info',
    })
  })

  it('does not call a resource read only when a write falls to the tool default', () => {
    const status = resourceStatus(resource([{ action: 'read', label: 'Read files', effect: 'allowed' }, { action: 'edit', label: 'Edit files', effect: 'default' }]))
    expect(status.text).toBe("Edit files: no rule; the tool's default applies")
    expect(status.tone).toBe('neutral')
  })

  it('words a hypothetical status conditionally', () => {
    const status = resourceStatus(resource([{ action: 'deploy', label: 'Deploy', effect: 'allowed', hypothetical: true, installed: 'not-generated' }]))
    expect(status.text).toBe('Deploy would be permitted once synced')
  })

  it('says only "unresolved" when nothing on the resource is decided for this tool', () => {
    const status = resourceStatus(
      resource([
        { action: 'deploy', label: 'Deploy', effect: 'unresolved' },
        { action: 'delete', label: 'Delete', effect: 'unresolved' },
      ]),
    )
    expect(status).toMatchObject({ text: 'Unresolved on this tool', tone: 'neutral' })
    expect(resourceStatus(resource([{ action: 'fetch', label: 'Fetch', effect: 'default' }, { action: 'search', label: 'Search', effect: 'default' }])).text).toBe(
      "No rule; the tool's default applies",
    )
  })

  it('summarises a closed resource by its shared effect or its heaviest operation', () => {
    expect(resourceStatus(resource([{ action: 'run', label: 'Run', effect: 'unavailable' }])).text).toBe('Unavailable through this route')
    expect(
      resourceStatus(resource([{ action: 'query', label: 'Query', effect: 'unavailable' }, { action: 'execute', label: 'Execute', effect: 'denied' }])).text,
    ).toBe('Execute blocked')
  })

  it.runIf(usingMockAuthority)('derives the mock pills', () => {
    const view = loadAuthorityFixture()
    const pill = (id: string) => resourceStatus(findResource(view, id)!).text
    expect(pill('mcp-railway-scope')).toBe('Deploy would be permitted once synced')
    expect(pill('mcp-github-scope')).toBe('Merge pull request would require approval once synced')
    expect(pill('files-env')).toBe('Would be read only once synced')
    expect(pill('files-all')).toBe("Edit files: no rule; the tool's default applies")
    expect(pill('mcp-analytics-db-scope')).toBe('Would be unavailable through this route once synced')
    expect(pill('web-all')).toBe("No rule; the tool's default applies")
  })

  it('derives a pill for every resource of any fixture', () => {
    const view = loadAuthorityFixture()
    for (const r of view.resources) {
      const status = resourceStatus(r)
      expect(status.text.length, r.id).toBeGreaterThan(0)
      if (status.operationId) expect(r.operations.map((o) => o.id)).toContain(status.operationId)
      // Never an unconditional "Permitted" for a hypothetical operation.
      if (status.hypothetical) expect(status.text, r.id).not.toMatch(/(^|\s)permitted(,|$)/i)
    }
  })
})
