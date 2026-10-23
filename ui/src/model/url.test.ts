import { describe, expect, it } from 'vitest'
import {
  DEFAULT_STATE,
  nextState,
  parseUrlState,
  resolveDataSource,
  serializeUrlState,
  staticAuthorityCandidates,
  staticOverviewUrl,
} from './url'

describe('URL state parsing', () => {
  it('defaults to the Context section', () => {
    expect(parseUrlState('')).toEqual({ ...DEFAULT_STATE, target: undefined, agentId: undefined, domain: undefined, resource: undefined, proposal: undefined, source: undefined })
    expect(parseUrlState('?view=nonsense').view).toBe('context')
    expect(parseUrlState('?view=feed').view).toBe('context')
    expect(parseUrlState('?view=lifecycle').view).toBe('lifecycle')
  })

  it('reads every navigable choice', () => {
    const state = parseUrlState('?view=authority&target=codex&agentId=coding-agent&domain=cloud-deployment&resource=mcp-railway-scope&tab=circuit&mock=1')
    expect(state).toMatchObject({
      view: 'authority',
      target: 'codex',
      agentId: 'coding-agent',
      domain: 'cloud-deployment',
      resource: 'mcp-railway-scope',
      tab: 'circuit',
      mock: true,
    })
  })

  it('drops malformed ids instead of passing them on', () => {
    const state = parseUrlState('?view=authority&agentId=%3Cscript%3E&domain=' + 'x'.repeat(300))
    expect(state.agentId).toBeUndefined()
    expect(state.domain).toBeUndefined()
  })

  it('accepts only a well-formed proposal', () => {
    const good = { agentId: 'coding-agent', target: 'claude', changes: [{ op: 'add', list: 'ask', rule: 'mcp:railway/deploy' }] }
    expect(parseUrlState(`?proposal=${encodeURIComponent(JSON.stringify(good))}`).proposal).toEqual(good)
    const badList = { ...good, changes: [{ op: 'add', list: 'protected', rule: 'x' }] }
    expect(parseUrlState(`?proposal=${encodeURIComponent(JSON.stringify(badList))}`).proposal).toBeUndefined()
    expect(parseUrlState('?proposal=%7Bnot-json').proposal).toBeUndefined()
  })

  it('round-trips through serialization in a stable order, leaving defaults out', () => {
    const search = '?view=authority&target=claude&agentId=reviewer&domain=source-control&resource=mcp-github-scope&tab=circuit&mock=1'
    expect(serializeUrlState(parseUrlState(search))).toBe(search)
    expect(serializeUrlState(DEFAULT_STATE)).toBe('')
    expect(serializeUrlState({ ...DEFAULT_STATE, source: './snapshot', mock: true })).toBe('?source=.%2Fsnapshot')
  })

  it('clears what a navigation invalidates and keeps the data source', () => {
    const at = parseUrlState('?view=authority&domain=cloud-deployment&resource=mcp-railway-scope&mock=1')
    expect(nextState(at, { domain: 'data-storage' })).toMatchObject({ domain: 'data-storage', resource: undefined, mock: true })
    expect(nextState(at, { view: 'tools' })).toMatchObject({ view: 'tools', domain: undefined, resource: undefined, mock: true })
    const proposal = { agentId: 'coding-agent', changes: [] }
    expect(nextState({ ...at, proposal }, { target: 'codex' }).proposal).toBeUndefined()
    expect(nextState({ ...at, proposal }, { resource: 'x' }).proposal).toBe(proposal)
  })
})

describe('data source', () => {
  it('asks the server by default', () => {
    expect(resolveDataSource(parseUrlState(''))).toEqual({ kind: 'api' })
  })

  it('loads the bundled fixtures with ?mock=1', () => {
    expect(resolveDataSource(parseUrlState('?mock=1'))).toEqual({ kind: 'mock' })
  })

  it('reads a static snapshot from ?source=, relative or http(s)', () => {
    expect(resolveDataSource(parseUrlState('?source=./snapshot'))).toEqual({ kind: 'static', base: './snapshot/' })
    expect(resolveDataSource(parseUrlState('?source=snapshots/demo/'))).toEqual({ kind: 'static', base: 'snapshots/demo/' })
    expect(resolveDataSource(parseUrlState('?source=https%3A%2F%2Fexample.org%2Fharness'))).toEqual({ kind: 'static', base: 'https://example.org/harness/' })
    expect(resolveDataSource(parseUrlState('?source=./snapshot&mock=1')).kind).toBe('static')
  })

  it('refuses a source with any other scheme', () => {
    expect(resolveDataSource(parseUrlState('?source=javascript%3Aalert(1)')).kind).toBe('invalid')
    expect(resolveDataSource(parseUrlState('?source=data%3Atext%2Fplain%2Cx')).kind).toBe('invalid')
  })

  it('uses a build-time default source only when the URL names none', () => {
    expect(resolveDataSource(parseUrlState(''), './snapshot')).toEqual({ kind: 'static', base: './snapshot/' })
    expect(resolveDataSource(parseUrlState('?mock=1'), './snapshot')).toEqual({ kind: 'mock' })
    expect(resolveDataSource(parseUrlState('?source=other'), './snapshot')).toEqual({ kind: 'static', base: 'other/' })
  })

  it('names the static files, most specific first', () => {
    expect(staticAuthorityCandidates('./s/', 'reviewer', 'codex')).toEqual(['./s/authority.reviewer.codex.json', './s/authority.json'])
    expect(staticAuthorityCandidates('./s/', 'reviewer')).toEqual(['./s/authority.json'])
    expect(staticAuthorityCandidates('./s/', '../etc', 'codex')).toEqual(['./s/authority.json'])
    expect(staticOverviewUrl('./s/')).toBe('./s/overview.json')
  })
})
