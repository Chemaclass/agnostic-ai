import { describe, expect, it, vi } from 'vitest'
import { createGlobalSetup, GLOBAL_SETUP_BODY, GlobalSetupError, globalSetupCreated, parseServedConfig, parseSwitchReply, requestSwitch, switchOffer } from './config-switch'

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
      command: "agnostic-ai ui --dir '/work/repo'",
    })
  })

  it('quotes the project path so the command pastes into a shell', () => {
    expect(switchOffer({ root: '/home', global: true, projectRoot: "/Users/x/My Code/it's" })?.command)
      .toBe("agnostic-ai ui --dir '/Users/x/My Code/it'\\''s'")
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

  it('reports a missing global setup with its folder, and nothing else as that', async () => {
    const missing = vi.fn(async () => new Response(JSON.stringify({ error: 'no global setup in /home/.agnostic-ai', missing: true, root: '/home/.agnostic-ai' }), { status: 409 }))
    const err = await requestSwitch(missing).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(GlobalSetupError)
    expect((err as GlobalSetupError).root).toBe('/home/.agnostic-ai')

    const broken = vi.fn(async () => new Response(JSON.stringify({ error: '/h is a symlink to /gone, which does not exist' }), { status: 409 }))
    const plain = await requestSwitch(broken).catch((e: unknown) => e)
    expect(plain).not.toBeInstanceOf(GlobalSetupError)
    expect((plain as Error).message).toContain('symlink')
  })

  it('posts the git choice to create the setup and reports a refusal', async () => {
    const ok = vi.fn(async () => new Response(JSON.stringify({ root: '/h', created: ['agnostic-ai.yaml'], git: 'initialized' }), { status: 200 }))
    await expect(createGlobalSetup(true, ok)).resolves.toEqual({ root: '/h', created: ['agnostic-ai.yaml'], git: 'initialized' })
    expect(ok).toHaveBeenCalledWith('/api/global/create', expect.objectContaining({ method: 'POST', body: '{"git":true}' }))

    const exists = vi.fn(async () => new Response(JSON.stringify({ error: 'a global setup already exists in /h' }), { status: 409 }))
    await expect(createGlobalSetup(false, exists)).rejects.toThrow('already exists')
  })

  it('keeps the onboarding copy plain', () => {
    expect(GLOBAL_SETUP_BODY).not.toMatch(/[–—]/)
    expect(globalSetupCreated('/h/.agnostic-ai')).toBe('Created /h/.agnostic-ai. Add files, then run "agnostic-ai sync --global".')
  })
})
