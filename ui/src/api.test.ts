import { afterEach, describe, expect, it, vi } from 'vitest'
import { loadOverview, parseScopes, postEdit } from './api'
import { builtinChange } from './builtin-change'
import { API } from './contract'
import fixture from './fixtures/overview.mock.json'

const idle = { schemaVersion: 'builtin-change/v1', name: 'caveman', status: 'idle', enabled: false, supported: true, undoAvailable: false, targets: [], changes: [], findings: [] }

function recordFetch(body: unknown): string[] {
  const urls: string[] = []
  vi.stubGlobal('fetch', async (url: string) => {
    urls.push(url)
    return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
  })
  return urls
}

afterEach(() => vi.unstubAllGlobals())

describe('scoped requests', () => {
  it('asks for the chosen scope on reads, edits, and Caveman changes', async () => {
    const urls = recordFetch(fixture)
    const loaded = await loadOverview({ kind: 'api' }, undefined, 'global')
    expect(loaded.endpoint).toBe('GET /api/views/overview?scope=global')
    await postEdit(API.editBuiltin, { name: 'memory', enabled: true }, 'global')
    const changes = recordFetch(idle)
    await builtinChange(undefined, undefined, 'global')
    await builtinChange('undo', { id: 'x' }, 'project')
    expect(urls).toEqual(['/api/views/overview?scope=global', '/api/edit/builtin?scope=global'])
    expect(changes).toEqual(['/api/changes/builtin?scope=global', '/api/changes/builtin/undo?scope=project'])
  })

  it('leaves the scope out for the server default', async () => {
    const urls = recordFetch(fixture)
    await loadOverview({ kind: 'api' })
    await postEdit(API.editValue, { path: 'a', key: 'b', value: true })
    expect(urls).toEqual(['/api/views/overview', '/api/edit/value'])
  })
})

describe('scopes contract', () => {
  const both = { default: 'project', scopes: [{ id: 'project', name: 'demo', root: '/work/demo' }, { id: 'global', root: '/home/me/.agnostic-ai' }] }

  it('keeps the served scopes and the default', () => {
    expect(parseScopes(both)).toEqual(both)
  })

  it.each([
    null, { ...both, default: 'team' }, { ...both, scopes: [{ id: 'team', root: '/' }] },
    { ...both, scopes: [{ id: 'global' }] }, { default: 'project', scopes: [{ id: 'global', root: '/g' }] },
  ])('refuses %j, so no switch shows', (value) => {
    expect(parseScopes(value)).toBeUndefined()
  })
})
