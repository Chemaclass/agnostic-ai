import { describe, expect, it } from 'vitest'
import { DEFAULT_STATE, nextState, parseUrlState, resolveDataSource, serializeUrlState, staticOverviewUrl } from './url'

describe('URL state parsing', () => {
  it('defaults to the Context section', () => {
    expect(parseUrlState('')).toEqual({ ...DEFAULT_STATE, source: undefined })
    expect(parseUrlState('?view=nonsense').view).toBe('context')
    expect(parseUrlState('?view=feed').view).toBe('context')
    expect(parseUrlState('?view=lifecycle').view).toBe('lifecycle')
  })

  it('round-trips through serialization, leaving defaults out', () => {
    expect(serializeUrlState(parseUrlState('?view=settings&mock=1'))).toBe('?view=settings&mock=1')
    expect(serializeUrlState(DEFAULT_STATE)).toBe('')
    expect(serializeUrlState({ ...DEFAULT_STATE, source: './snapshot', mock: true })).toBe('?source=.%2Fsnapshot')
  })

  it('keeps the search and a linked entry, with reserved characters, in the URL', () => {
    const entry = 'item:mcp:git hub&co:.agnostic-ai/mcps/git#hub.yaml'
    const query = serializeUrlState({ view: 'tools', q: 'a&b=c', entry, mock: true })
    expect(query).toBe('?view=tools&q=a%26b%3Dc&entry=item%3Amcp%3Agit+hub%26co%3A.agnostic-ai%2Fmcps%2Fgit%23hub.yaml&mock=1')
    expect(parseUrlState(query)).toEqual({ view: 'tools', q: 'a&b=c', entry, mock: true, source: undefined })
  })

  it('drops the search and link when the section changes, and keeps them otherwise', () => {
    const state = parseUrlState('?view=tools&q=github&entry=item%3Amcp%3Agithub%3Ax')
    expect(nextState(state, { view: 'context' })).toEqual({ view: 'context', mock: false, q: undefined, entry: undefined, source: undefined })
    expect(nextState(state, { view: 'tools' })).toEqual(state)
    expect(nextState(state, { view: 'settings', entry: 'field:model' })).toMatchObject({ view: 'settings', q: undefined, entry: 'field:model' })
  })

  it('keeps the data source across navigation', () => {
    expect(nextState(parseUrlState('?view=tools&mock=1'), { view: 'settings' })).toEqual({ view: 'settings', mock: true, source: undefined })
  })

  it('keeps a selected tab on refresh and clears it when the page changes', () => {
    const state = parseUrlState('?view=tools&tab=native&q=github&mock=1')
    expect(state.tab).toBe('native')
    expect(serializeUrlState(state)).toBe('?view=tools&tab=native&q=github&mock=1')
    expect(nextState(state, { view: 'settings' }).tab).toBeUndefined()
    expect(nextState(state, { tab: 'servers' })).toMatchObject({ q: 'github', mock: true, tab: 'servers' })
    expect(parseUrlState('?tab=../invalid').tab).toBeUndefined()
  })

  it('keeps the Context size tool and file in the URL', () => {
    const state = parseUrlState('?view=context&tab=usage&target=claude&file=src%2Fmain.go')
    expect(state).toMatchObject({ target: 'claude', file: 'src/main.go' })
    expect(serializeUrlState(state)).toBe('?tab=usage&target=claude&file=src%2Fmain.go')
    expect(parseUrlState('?target=../bad').target).toBeUndefined()
  })

  it('drops the tool and file when the page changes', () => {
    const state = parseUrlState('?view=context&tab=usage&target=claude&file=src%2Fmain.go')
    expect(nextState(state, { view: 'tools' })).toMatchObject({ target: undefined, file: undefined })
    expect(nextState(state, { file: 'a.go' })).toMatchObject({ target: 'claude', file: 'a.go' })
  })
})

describe('data source', () => {
  it('asks the server by default', () => {
    expect(resolveDataSource(parseUrlState(''))).toEqual({ kind: 'api' })
  })

  it('loads the bundled fixture with ?mock=1', () => {
    expect(resolveDataSource(parseUrlState('?mock=1'))).toEqual({ kind: 'mock' })
  })

  it('reads a static snapshot from ?source=, relative or http(s)', () => {
    expect(resolveDataSource(parseUrlState('?source=./snapshot'))).toEqual({ kind: 'static', base: './snapshot/' })
    expect(resolveDataSource(parseUrlState('?source=https%3A%2F%2Fexample.org%2Fharness'))).toEqual({ kind: 'static', base: 'https://example.org/harness/' })
    expect(resolveDataSource(parseUrlState('?source=./snapshot&mock=1')).kind).toBe('static')
    expect(staticOverviewUrl('./s/')).toBe('./s/overview.json')
  })

  it('refuses a source with any other scheme', () => {
    expect(resolveDataSource(parseUrlState('?source=javascript%3Aalert(1)')).kind).toBe('invalid')
    expect(resolveDataSource(parseUrlState('?source=data%3Atext%2Fplain%2Cx')).kind).toBe('invalid')
  })

  it('uses a build-time default source only when the URL names none', () => {
    expect(resolveDataSource(parseUrlState(''), './snapshot')).toEqual({ kind: 'static', base: './snapshot/' })
    expect(resolveDataSource(parseUrlState('?mock=1'), './snapshot')).toEqual({ kind: 'mock' })
  })
})
