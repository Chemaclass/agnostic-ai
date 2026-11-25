import { describe, expect, it } from 'vitest'
import { normalizeConversationsView, normalizeOverviewView } from './normalize'
import { conversationPlaceLabel, conversationToolLabel, conversationsVerdict, groupConversations, needsYouReason } from './conversations'

describe('normalizeOverviewView repository', () => {
  it('keeps a GitHub repository page', () => {
    const view = normalizeOverviewView({ repository: { url: 'https://github.com/octo/hello', name: 'octo/hello' } })
    expect(view.repository).toEqual({ url: 'https://github.com/octo/hello', name: 'octo/hello' })
  })

  it('drops a link to another site or a malformed value', () => {
    for (const repository of [
      { url: 'https://evil.example', name: 'octo/hello' },
      { url: 'https://github.com.evil.example/octo/hello', name: 'octo/hello' },
      { url: 'javascript:alert(1)', name: 'octo/hello' },
      { url: 'https://github.com/octo/hello', name: {} },
      'https://github.com/octo/hello',
      null,
    ]) {
      expect(normalizeOverviewView({ repository }).repository).toBeUndefined()
    }
  })
})

describe('normalizeConversationsView', () => {
  const now = new Date('2026-11-07T16:30:00Z')
  const row = { tool: 'claude', app: 'cli', id: 'holder', title: 'A chat', updatedAt: now.toISOString(), open: { kind: 'terminal', label: 'Open' } }

  it('keeps the server-wide Needs you count through normalization and summary, including zero', () => {
    const raw = { needsYou: 7, conversations: [{ ...row, state: 'unread' }] }
    const view = normalizeConversationsView(raw)
    expect(view.conversations).toHaveLength(1)
    expect(view.needsYou).toBe(7)
    expect(conversationsVerdict(view.conversations, now, false, view.needsYou)).toBe('7 chats need you.')
    const zero = normalizeConversationsView({ ...raw, needsYou: 0 })
    expect(zero.needsYou).toBe(0)
    expect(conversationsVerdict(zero.conversations, now, false, zero.needsYou)).toBe('No chats need you.')
  })

  it('leaves absent or invalid counts absent for the older-server summary', () => {
    for (const needsYou of [undefined, null, '3', -1, 0.5, Infinity, NaN]) {
      const view = normalizeConversationsView({ needsYou, conversations: [{ ...row, live: true }] })
      expect(view.needsYou).toBeUndefined()
      expect(conversationsVerdict(view.conversations, now, false, view.needsYou)).toBe('One chat is open now.')
    }
  })

  it('carries wait and conflict fields through the real input boundary into grouping and reasons', () => {
    const conflict = { id: 'claim', worktree: '/repo', path: 'app.ts', requester: { tool: 'codex', id: 'requester' } }
    const view = normalizeConversationsView({ conversations: [
      { ...row, state: 'blocked', waiting: { for: 'permission', tool: 'Edit', detail: 'app.ts' }, claimConflicts: [conflict] },
      { ...row, id: 'conflict', state: 'working', claimConflicts: [conflict] },
      { ...row, id: 'quiet', state: 'blocked', quietSince: '2026-11-07T16:23:00Z' },
    ] })
    expect(groupConversations(view.conversations).map((group) => group.id)).toEqual(['needs-you'])
    expect(view.conversations.map((conversation) => needsYouReason(conversation, now))).toEqual(['Approve edit app.ts', 'Codex session requeste wants app.ts, which this session claims', 'No activity for 7m'])
  })

  it('preserves the product and place labels and cross-tool nesting the server supplied', () => {
    const view = normalizeConversationsView({ conversations: [
      { ...row, app: 'chat', state: 'active' },
      { ...row, id: 'cowork', app: 'cowork', state: 'active' },
      { ...row, id: 'cloud', app: 'cloud', state: 'active' },
      { ...row, id: 'shared', cloud: { id: 'remote', link: 'https://claude.ai/code/remote' }, state: 'active' },
      { ...row, id: 'chatgpt-parent', tool: 'chatgpt', app: 'chat', state: 'active' },
      { ...row, id: 'child', state: 'active', parent: { tool: 'chatgpt', id: 'chatgpt-parent' } },
    ] })
    expect(view.conversations.map((conversation) => [conversationToolLabel(conversation), conversationPlaceLabel(conversation)])).toEqual([
      ['Claude', 'Chat'], ['Claude', 'Cowork'], ['Claude Code', 'Cloud'], ['Claude Code', 'Shared'], ['ChatGPT', 'Chat'], ['Claude Code', undefined],
    ])
    expect(groupConversations(view.conversations)[0].under.get('claude:child')?.id).toBe('chatgpt-parent')
  })
})
