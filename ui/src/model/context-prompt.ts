import type { ContextLoad, ContextOpportunity, ContextSize } from '../contract'

export const LOAD_LABELS: Record<ContextLoad, string> = {
  startup: 'Startup',
  'file-scope': 'File-scoped',
  'on-demand': 'On demand',
}

/** When content with this load reaches the tool. */
export const LOAD_CONDITIONS: Record<ContextLoad, string> = {
  startup: 'at the start of every session',
  'file-scope': 'when the tool works on a matching file',
  'on-demand': 'only when the tool opens it (a skill or agent) or works on a matching file (a scoped rule)',
}

export function sizeText(size: ContextSize): string {
  return `${size.words} ${size.words === 1 ? 'word' : 'words'}, ${size.bytes} ${size.bytes === 1 ? 'byte' : 'bytes'}`
}

export function contextReviewPrompt(opportunity: ContextOpportunity, root?: string): string {
  const { target, load } = opportunity
  const lines = [
    'Review this duplicated instruction passage that agnostic-ai explain --context found:',
    '',
    `Target: ${target}`,
    `Loads: ${LOAD_CONDITIONS[load]} (${load})`,
    `Copies: ${opportunity.copies}`,
    `Size of one copy: ${sizeText(opportunity.size)}`,
    `Possible reduction: ${sizeText(opportunity.reduction)} from ${load} content, if only one copy stays`,
  ]
  if (root) lines.push(`Configuration root: ${root}`)
  lines.push('Sources:', ...opportunity.sources.map((s) => `- ${s.source}${s.category ? ` (${s.category})` : ''}`))
  lines.push('Passage:', '"""', opportunity.passage, '"""')
  lines.push(
    '',
    'Compare the copies in their sources and check that they mean the same thing where they appear. Propose which copy to keep, or how to merge them, and state the tradeoffs, such as a rule or skill that would stop carrying the instruction. Edit the canonical specs under .agnostic-ai/, not generated files. Do not apply any change until I agree.',
    '',
    `After the change, run agnostic-ai sync, then agnostic-ai explain --context --target ${target} to check the new counts. Words and bytes measure text size, not tokens; do not claim lower latency or cost.`,
  )
  return lines.join('\n')
}
