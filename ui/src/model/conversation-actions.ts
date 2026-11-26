import type { Conversation, ConversationDeleteReply, ConversationMarkRequest } from '../contract'
import { conversationGroupOf } from './conversations'

export const isArchived = (row: Conversation): boolean => conversationGroupOf(row) === 'archived'
export const canRead = (row: Conversation): boolean => row.state === 'blocked' || row.state === 'unread' || (!isArchived(row) && row.unread === true)
export const canComplete = (row: Conversation): boolean => !row.completedAt && row.state !== 'working' && ['needs-you', 'active'].includes(conversationGroupOf(row))
export const canRename = (row: Conversation): boolean => !(row.tool === 'claude' && (row.app === 'chat' || row.app === 'cowork')) && !(row.tool === 'codex' && row.app === 'cloud')
export const cloudResumeOf = (row: Conversation): string | undefined => row.tool === 'claude' && row.app === 'cloud' && row.cloudResumeCommand?.trim() ? row.cloudResumeCommand : undefined

export type ConversationMenuItem = 'read' | 'rename' | 'complete' | 'reopen' | 'hide' | 'web' | 'copy-session-id' | 'copy-resume' | 'delete' | 'stop-shell'

export const MENU_LABELS: Readonly<Record<ConversationMenuItem, string>> = {
  read: 'Mark as read',
  rename: 'Rename…',
  complete: 'Mark complete',
  reopen: 'Undo complete',
  hide: 'Hide from agnostic-ai',
  web: 'Open on claude.ai',
  'copy-session-id': 'Copy session ID',
  'copy-resume': 'Copy resume command',
  delete: 'Delete permanently…',
  'stop-shell': 'Stop shell…',
}

export const MENU_TOOLTIPS: Readonly<Record<Exclude<ConversationMenuItem, 'copy-resume'>, string>> = {
  read: 'Takes this chat out of Needs you without opening it, until something new is written.',
  rename: 'Changes this chat’s name in the list.',
  complete: 'Moves this chat to Archived until it has new activity.',
  reopen: 'Clears the completion mark you set for this chat.',
  hide: 'Hides this chat from the list and search until it has new activity.',
  web: 'Opens this chat on claude.ai.',
  'copy-session-id': 'Copies the ID your tool uses to identify this chat.',
  delete: 'Permanently removes this chat from this computer.',
  'stop-shell': 'Asks which shell command to stop when the chat has one still running.',
}

export const CLOUD_RESUME_TOOLTIP = 'Copies the command to resume this cloud session in Claude Code.'
export const MARK_ALL_READ_TOOLTIP = 'Mark each chat shown here as read without opening it. A chat that waits on something else, such as a permission or a path conflict, stays here.'
export const MARK_ALL_COMPLETE_TOOLTIP = 'Mark every active conversation complete, until something new is written in it or it starts working'
export const DANGER_ITEMS: ReadonlySet<ConversationMenuItem> = new Set(['delete', 'stop-shell'])
export const NEEDS_SERVER_MESSAGE = 'Changing conversations needs a running agnostic-ai server.'
export const BLANK_NAME_MESSAGE = 'Type a name, or press Escape to keep the old one.'

export function conversationMenuOf(row: Conversation, { localResume, server }: { localResume?: string; server: boolean }): ConversationMenuItem[] {
  const archived = isArchived(row)
  const offered: ConversationMenuItem[] = []
  if (canRead(row)) offered.push('read')
  if (canRename(row)) offered.push('rename')
  if (!archived && canComplete(row)) offered.push('complete')
  if (row.completedAt) offered.push('reopen')
  if (archived) offered.push('hide')
  if (row.cloud) offered.push('web')
  offered.push('copy-session-id')
  if (cloudResumeOf(row) || localResume) offered.push('copy-resume')
  if (archived && !row.live) offered.push('delete')
  if (server && row.stuckShells?.length) offered.push('stop-shell')
  return offered
}

export function nameOf(typed: string): string | undefined {
  const name = typed.split(/\s+/).filter(Boolean).join(' ')
  return name === '' ? undefined : name
}

export function renameOf(draft: string, shown: string): { keep: true } | { empty: true } | { title: string } {
  const title = nameOf(draft)
  if (title === undefined) return { empty: true }
  if (title === shown) return { keep: true }
  return { title }
}

export function deleteReason(row: Conversation): string | undefined {
  if (!isArchived(row)) return 'Only archived conversations can be deleted.'
  if (row.live) return 'This conversation is still running.'
  return row.deleteUnsupported
}

const DELETED_DATA: Readonly<Partial<Record<Conversation['tool'], string>>> = {
  claude: "This removes the session from this computer: its transcript, its subagent transcripts, saved tool output, file backups and pasted images, and its name and marks in agnostic-ai. It cannot be undone. The prompts you typed stay in Claude Code's own prompt history.",
}

export function deleteConfirmOf(row: Conversation): { title: string; body: string } {
  return {
    title: `Delete "${row.title || 'Untitled conversation'}" permanently?`,
    body: DELETED_DATA[row.tool] ?? 'This removes the conversation from this computer. It cannot be undone.',
  }
}

const SIZE_UNITS = ['KB', 'MB', 'GB']

export function freedOf(bytes: number | undefined): string {
  if (bytes === undefined || !Number.isFinite(bytes) || bytes <= 0) return ''
  if (bytes < 1000) return `Freed ${bytes} bytes`
  let size = bytes
  let unit = -1
  while (size >= 1000 && unit < SIZE_UNITS.length - 1) {
    size /= 1000
    unit += 1
  }
  const shown = size < 10 ? Math.round(size * 10) / 10 : Math.round(size)
  return `Freed ${shown} ${SIZE_UNITS[unit]}`
}

export function deletedOf(reply: ConversationDeleteReply): string {
  const freed = freedOf(reply.freedBytes)
  return [freed === '' ? 'Deleted.' : `Deleted. ${freed}.`, reply.note].filter(Boolean).join(' ')
}

export function unchangedForMark(shown: Conversation, current: Conversation | undefined, as: ConversationMarkRequest['as']): boolean {
  if (!current || current.tool !== shown.tool || current.id !== shown.id) return false
  const version = (row: Conversation) => JSON.stringify([row.updatedAt, row.state, row.completedAt, row.seenAt, row.unread, row.live, row.activity, row.waiting, row.claimConflicts])
  if (version(current) !== version(shown)) return false
  return as === 'seen' ? canRead(current) : as === 'completed' && canComplete(current)
}
