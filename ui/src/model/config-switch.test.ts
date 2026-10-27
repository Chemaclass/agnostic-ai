import { describe, expect, it, vi } from 'vitest'
import { parseServedConfig, parseSwitchReply, requestSwitch, switchOffer } from './config-switch'

describe('switch between project and global', () => {
  it('reads the configuration a server reads, with the project a global server started in', () => {
    expect(parseServedConfig({ ok: true, root: '/repo', global: false, ui: true })).toEqual({ root: '/repo', global: false })
    expect(parseServedConfig({ ok: true, root: '/home/.agnostic-ai', global: true, projectRoot: '/repo' }))
      .toEqual({ root: '/home/.agnostic-ai', global: true, projectRoot: '/repo' })
    expect(parseServedConfig({ ok: true, root: '/home', global: true, projectRoot: '' })).toEqual({ root: '/home', global: true })
    expect(parseServedConfig({ ok: true })).toBeUndefined()
    expect(parseServedConfig('nope')).toBeUndefined()
  })

  it('offers global settings on a project page', () => {
    expect(switchOffer({ root: '/repo', global: false })).toEqual({
      label: 'Open global settings',
      title: 'Open the user configuration in a new tab, starting its server when none runs',
      command: 'agnostic-ai ui --global',
    })
  })

  it('offers the way back only on a global page started in a project', () => {
    expect(switchOffer({ root: '/home/.agnostic-ai', global: true })).toBeUndefined()
    expect(switchOffer({ root: '/home/.agnostic-ai', global: true, projectRoot: '/work/repo' })).toEqual({
      label: 'Open project repo',
      title: 'Open /work/repo in a new tab, starting its server when none runs',
      command: 'agnostic-ai ui --dir /work/repo',
    })
  })

  it('accepts only a loopback http address for the new tab', () => {
    for (const url of ['http://127.0.0.1:4311/', 'http://[::1]:4311/', 'http://localhost:4311/']) {
      expect(parseSwitchReply({ url, root: '/home', global: true, started: true }).url).toBe(url)
    }
    for (const url of ['https://example.com/', 'http://example.com:4311/', 'javascript:alert(1)', 'http://127.0.0.1:4311/api/health', 4311]) {
      expect(() => parseSwitchReply({ url, root: '/home', global: true, started: false })).toThrow('invalid switch reply')
    }
  })

  it('posts an empty JSON body and reports the server error with its hint', async () => {
    const ok = vi.fn(async () => new Response(JSON.stringify({ url: 'http://127.0.0.1:4311/', root: '/home', global: true, started: false }), { status: 200 }))
    await expect(requestSwitch(ok)).resolves.toMatchObject({ url: 'http://127.0.0.1:4311/' })
    expect(ok).toHaveBeenCalledWith('/api/switch', expect.objectContaining({ method: 'POST', body: '{}', headers: expect.objectContaining({ 'Content-Type': 'application/json' }) }))

    const refused = vi.fn(async () => new Response(JSON.stringify({ error: 'no free port; run agnostic-ai ui --global' }), { status: 503 }))
    await expect(requestSwitch(refused)).rejects.toThrow('no free port; run agnostic-ai ui --global')

    const down = vi.fn(async () => { throw new TypeError('offline') })
    await expect(requestSwitch(down)).rejects.toThrow('Could not reach the server')
  })
})
