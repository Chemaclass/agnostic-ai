import { describe, expect, it } from 'vitest'
import {
  DEFAULT_STATE,
  nextState,
  parseInAppHref,
  parseUrlState,
  resolveDataSource,
  serializeUrlState,
  staticAuthorityCandidates,
  staticOverviewUrl,
} from './url'

describe('URL state parsing', () => {
  it('defaults to the feed', () => {
    expect(parseUrlState('')).toEqual({ ...DEFAULT_STATE, target: undefined, agentId: undefined, domain: undefined, resource: undefined, card: undefined, proposal: undefined, source: undefined })
    expect(parseUrlState('?view=nonsense').view).toBe('feed')
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
    expect(nextState(at, { view: 'feed' })).toMatchObject({ view: 'feed', domain: undefined, resource: undefined, mock: true })
    const proposal = { agentId: 'coding-agent', changes: [] }
    expect(nextState({ ...at, proposal }, { target: 'codex' }).proposal).toBeUndefined()
    expect(nextState({ ...at, proposal }, { resource: 'x' }).proposal).toBe(proposal)
  })

  it('follows only in-app links from data', () => {
    const current = parseUrlState('?source=./snap')
    expect(parseInAppHref('?view=authority&domain=data-storage', current)).toMatchObject({ view: 'authority', domain: 'data-storage', source: './snap' })
    expect(parseInAppHref('/?view=context', current)?.view).toBe('context')
    expect(parseInAppHref('https://example.com/?view=feed', current)).toBeUndefined()
    expect(parseInAppHref('javascript:alert(1)', current)).toBeUndefined()
    expect(parseInAppHref(undefined, current)).toBeUndefined()
  })

  it('follows a link to this same page, never one to another origin or path', () => {
    const current = parseUrlState('')
    const page = 'http://127.0.0.1:4310/ui/?view=feed'
    expect(parseInAppHref('/ui/?view=authority&domain=data-storage', current, page)).toMatchObject({ view: 'authority', domain: 'data-storage' })
    expect(parseInAppHref('http://127.0.0.1:4310/ui/?view=tools', current, page)?.view).toBe('tools')
    expect(parseInAppHref('/elsewhere/?view=tools', current, page)).toBeUndefined()
    expect(parseInAppHref('http://example.com/ui/?view=tools', current, page)).toBeUndefined()
    expect(parseInAppHref('javascript:alert(1)', current, page)).toBeUndefined()
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
