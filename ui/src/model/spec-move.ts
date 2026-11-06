import type { SectionItem, SpecLevel, SpecMovePreview, SpecMoveRequest } from '../contract'

/** The kinds both a project and the global setup read. */
const MOVABLE_KINDS = ['rule', 'agent', 'skill', 'hook', 'mcp']

export type SpecMoveMode = SpecMoveRequest['mode']

export function otherLevel(level: SpecLevel): SpecLevel {
  return level === 'global' ? 'project' : 'global'
}

/** Only a shared spec of the served level moves; built-ins, packs, and local specs stay. */
export function canMoveSpec(item: SectionItem, level: SpecLevel): boolean {
  return MOVABLE_KINDS.includes(item.kind) && item.layer === level
}

export function specMoveLabel(level: SpecLevel): string {
  return `Copy to ${otherLevel(level)}`
}

export function specMoveRequest(item: SectionItem, level: SpecLevel, action: SpecMoveRequest['action'], mode: SpecMoveMode = 'copy', overwrite = false, expected?: string): SpecMoveRequest {
  const request: SpecMoveRequest = { action, kind: item.kind, name: item.name, from: level, to: otherLevel(level), mode, overwrite }
  if (expected) request.expected = expected
  return request
}

/** What the panel says about the destination before anything is written. */
export function destinationNote(preview: SpecMovePreview): string {
  if (!preview.exists) return 'Nothing is there yet.'
  return 'A spec is already there. Tick Replace it to overwrite the whole spec.'
}

export interface SpecMoved {
  name: string
  mode: SpecMoveMode
  destination: string
  syncCommands: string[]
}

export function specMovedText(moved: SpecMoved): string {
  return `${moved.mode === 'move' ? 'Moved' : 'Copied'} ${moved.name} to ${moved.destination}.`
}

/** Reads a notice saved before the page reloaded; anything else is dropped. */
export function parseSpecMoved(value: unknown): SpecMoved | undefined {
  if (!value || typeof value !== 'object') return undefined
  const v = value as Record<string, unknown>
  if (typeof v.name !== 'string' || (v.mode !== 'copy' && v.mode !== 'move') || typeof v.destination !== 'string') return undefined
  if (!Array.isArray(v.syncCommands) || !v.syncCommands.every((command) => typeof command === 'string')) return undefined
  return { name: v.name, mode: v.mode, destination: v.destination, syncCommands: v.syncCommands }
}

export const GLOBAL_SETUP_MISSING = 'The global setup does not exist yet. Create it with Open global settings at the foot of the sidebar, then copy again.'
