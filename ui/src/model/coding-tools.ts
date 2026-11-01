import type { ToolCatalog, ToolChoice } from '../contract'
import type { EditAudience } from './edit-audience'

/** The state the chosen view shows: the personal list for Personal, the shared list for Shared. */
export function toolOn(tool: ToolChoice, audience: EditAudience): boolean {
  return audience === 'personal' || !tool.local ? tool.enabled : !tool.enabled
}

export function filterTools(items: ToolChoice[], query: string, onlyOn: boolean, audience: EditAudience): ToolChoice[] {
  const q = query.trim().toLowerCase()
  return items.filter((tool) => (!onlyOn || toolOn(tool, audience)) && (!q || [tool.target, tool.label, tool.description].some((part) => part.toLowerCase().includes(q))))
}

export function toolCount(items: ToolChoice[], audience: EditAudience): { on: number; total: number } {
  return { on: items.filter((tool) => toolOn(tool, audience)).length, total: items.length }
}

/** What the next sync does to a tool that is turned off, in the words the panel shows. */
export function offConsequence(label: string, global: boolean): string {
  if (global) return `The next sync --global stops writing files for ${label}. Files it wrote before stay until you remove them by hand.`
  return `The next sync removes the files it wrote for ${label}. A file you edited is kept as a .bak copy, and a shared settings file keeps its other keys.`
}

/** Where the switch saves, and the sync command that applies it. */
export function saveHint(catalog: ToolCatalog, audience: EditAudience, global: boolean): string {
  const command = global ? 'agnostic-ai sync --global' : 'agnostic-ai sync'
  const personal = audience === 'personal'
  const file = global ? (personal ? 'local/agnostic-ai.yaml' : 'agnostic-ai.yaml') : personal ? 'agnostic-ai.local.yaml' : 'agnostic-ai.yaml'
  const start = catalog.source === 'default' && !personal ? `No config file lists coding tools yet, so the default list applies. The first change writes the whole list to ${file}.` : `Changes save to ${file}.`
  return `${start} Run ${command} to apply them to your tools.${global ? '' : ' A running sync --watch applies them on its own, unless it was started with --target, --only or --except.'}`
}
