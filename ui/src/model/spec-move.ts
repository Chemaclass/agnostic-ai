import type { SectionItem, SpecLevel, SpecMovePreview, SpecMoveRequest } from '../contract'

/** The kinds both a project and the user setup read. */
const MOVABLE_KINDS = ['rule', 'agent', 'skill', 'hook', 'mcp']

export type SpecMoveMode = SpecMoveRequest['mode']

export function otherLevel(level: SpecLevel): SpecLevel {
  return level === 'global' ? 'project' : 'global'
}

/** The spec layer a level's own shared specs load in. The level keeps its name in the API; the layer is what `list` prints. */
export function layerOfLevel(level: SpecLevel): string {
  return level === 'global' ? 'user' : 'project'
}

/** Only a shared spec of the served level moves; built-ins, packs, and local specs stay. */
export function canMoveSpec(item: SectionItem, level: SpecLevel): boolean {
  return MOVABLE_KINDS.includes(item.kind) && item.layer === layerOfLevel(level)
}

export function specMoveLabel(level: SpecLevel): string {
  // The API calls the user level `global`; people read "user".
  return `Copy to ${otherLevel(level) === 'global' ? 'user' : 'project'}`
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
  warning?: string
}

export function specMovedText(moved: SpecMoved): string {
  const text = `${moved.mode === 'move' ? 'Moved' : 'Copied'} ${moved.name} to ${moved.destination}.`
  return moved.warning ? `${text} ${moved.warning}.` : text
}

/** Reads a notice saved before the page reloaded; anything else is dropped. */
export function parseSpecMoved(value: unknown): SpecMoved | undefined {
  if (!value || typeof value !== 'object') return undefined
  const v = value as Record<string, unknown>
  if (typeof v.name !== 'string' || (v.mode !== 'copy' && v.mode !== 'move') || typeof v.destination !== 'string') return undefined
  if (!Array.isArray(v.syncCommands) || !v.syncCommands.every((command) => typeof command === 'string')) return undefined
  const moved: SpecMoved = { name: v.name, mode: v.mode, destination: v.destination, syncCommands: v.syncCommands }
  if (typeof v.warning === 'string' && v.warning) moved.warning = v.warning
  return moved
}

export const USER_SETUP_MISSING = 'The user setup does not exist yet. Create it with Open user-level settings at the foot of the sidebar, then copy again.'
