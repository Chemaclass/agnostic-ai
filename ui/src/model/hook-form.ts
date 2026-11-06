import type { HookSpecRequest, Stage } from '../contract'
import type { EditAudience } from './edit-audience'

const PORTABLE_STAGES = ['session-start', 'prompt-submit', 'before-tool', 'after-tool', 'after-edit', 'stop', 'session-end']

export const HOOK_MATCHES: { value: string; label: string }[] = [
  { value: '', label: 'Every tool' },
  { value: 'shell', label: 'Shell commands' },
  { value: 'edit', label: 'File edits' },
  { value: 'read', label: 'File reads' },
  { value: 'web', label: 'Web fetch and search' },
]

export interface HookDraft {
  name: string
  on: string
  match: string
  command: string
  timeout: string
  description: string
}

export function emptyHookDraft(on: string): HookDraft {
  return { name: '', on, match: '', command: '', timeout: '', description: '' }
}

/** The stages a new hook can use: the portable events, not one tool's own event names. */
export function hookStages(stages: Stage[]): Stage[] {
  return stages.filter((stage) => PORTABLE_STAGES.includes(stage.id))
}

export function takesMatch(on: string): boolean {
  return on === 'before-tool' || on === 'after-tool'
}

export function hookDraftProblem(draft: HookDraft): string | undefined {
  if (!draft.name) return 'Enter a name.'
  if (!/^[a-z0-9][a-z0-9-]*$/.test(draft.name)) return 'Use lowercase letters, digits, and hyphens, such as format-on-save.'
  if (!draft.command.trim()) return 'Enter the command to run.'
  if (draft.timeout.trim() && !/^[1-9]\d*$/.test(draft.timeout.trim())) return 'Timeout must be a whole number of seconds above zero.'
  return undefined
}

export function hookRequest(draft: HookDraft, audience: EditAudience, action: 'preview' | 'apply', expected?: string): HookSpecRequest {
  const request: HookSpecRequest = { action, audience, name: draft.name, on: draft.on, command: draft.command }
  if (takesMatch(draft.on) && draft.match) request.match = draft.match
  if (draft.timeout.trim()) request.timeout = Number(draft.timeout.trim())
  if (draft.description.trim()) request.description = draft.description.trim()
  if (expected) request.expected = expected
  return request
}
