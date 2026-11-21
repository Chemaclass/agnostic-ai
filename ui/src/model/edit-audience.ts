import type { SpecLevel } from '../contract'

/** Which files an edit writes: the committed level files, or local ones Git ignores. */
export type EditAudience = 'shared' | 'personal'

/** Reads a saved choice, including `local` saved before the rename. */
export function readAudience(saved: string | null): EditAudience {
  return saved === 'personal' || saved === 'local' ? 'personal' : 'shared'
}

/** The level the page serves: a project, or the user setup in the home folder. */
/** What people read for the level a page serves. */
export function levelName(userLevel: boolean): 'project' | 'user' {
  return userLevel ? 'user' : 'project'
}

/** The level as the API names it: the user level is `global` there. */
export function specLevel(userLevel: boolean): SpecLevel {
  return userLevel ? 'global' : 'project'
}

export function audienceLabel(audience: EditAudience, userLevel: boolean): string {
  return audience === 'personal' ? 'Local' : userLevel ? 'User' : 'Project'
}

/** The selector's accessible name, which says whether Git tracks the files. */
export function audienceDescription(audience: EditAudience, userLevel: boolean): string {
  return audience === 'personal' ? 'Local (not committed)' : userLevel ? 'User (every project)' : 'Project (committed)'
}

/** The layer an item comes from, with each level's local layer shown as `local`. */
export function layerLabel(layer: string): string {
  return layer === 'local' || layer === 'user-local' ? 'local' : layer
}
