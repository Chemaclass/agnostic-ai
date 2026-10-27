/** Which files an edit writes: the committed shared files, or personal ones Git ignores. */
export type EditAudience = 'shared' | 'personal'

/** Reads a saved choice, including `local` saved before the rename. */
export function readAudience(saved: string | null): EditAudience {
  return saved === 'personal' || saved === 'local' ? 'personal' : 'shared'
}
