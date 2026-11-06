/** Which files an edit writes: the committed level files, or local ones Git ignores. */
export type EditAudience = 'shared' | 'personal'

/** Reads a saved choice, including `local` saved before the rename. */
export function readAudience(saved: string | null): EditAudience {
  return saved === 'personal' || saved === 'local' ? 'personal' : 'shared'
}

/** The level the page serves: a project, or the global setup in the home folder. */
export function levelName(global: boolean): 'project' | 'global' {
  return global ? 'global' : 'project'
}

export function audienceLabel(audience: EditAudience, global: boolean): string {
  return audience === 'personal' ? 'Local' : global ? 'Global' : 'Project'
}

/** The selector's accessible name, which says whether Git tracks the files. */
export function audienceDescription(audience: EditAudience, global: boolean): string {
  return audience === 'personal' ? 'Local (not committed)' : global ? 'Global (every project)' : 'Project (committed)'
}

/** The layer an item comes from, with each level's local layer shown as `local`. */
export function layerLabel(layer: string): string {
  return layer === 'project-user' || layer === 'global-local' ? 'local' : layer
}
