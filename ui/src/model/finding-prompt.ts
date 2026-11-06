import type { ItemFinding } from '../contract'

/** The lines that describe one finding, shared by the single and combined fix prompts. */
export function findingFacts(finding: ItemFinding): string[] {
  const lines = [
    `Check: ${finding.source}${finding.code ? ` (${finding.code})` : ''}`,
    `Severity: ${finding.severity}`,
  ]
  if (finding.layer === 'global') lines.push('Layer: global setup, which reaches every project; edit it there, not in this project')
  // A global path is relative to the global source, so the open path names the file.
  const source = finding.layer === 'global' ? finding.openPath || finding.path : finding.path
  if (source) lines.push(`Source: ${source}${finding.line ? `:${finding.line}` : ''}`)
  else if (finding.line) lines.push(`Line: ${finding.line}`)
  lines.push(`Finding: ${finding.message}`)
  if (finding.explanation) lines.push(`Explanation: ${finding.explanation}`)
  if (finding.fix) lines.push(`Suggested fix: ${finding.fix}`)
  return lines
}

export const FIX_GUIDANCE = [
  'Inspect the source and confirm the cause before editing. Change the canonical specs, not generated files. Preserve unrelated content and tool behavior. Keep the same project or global scope. Do not disable the check or raise its budget to hide the finding.',
  '',
  'Run the relevant agnostic-ai checks after the change. If generated files need updating, review sync --plan, then run sync and sync --check. Report what changed and any remaining findings.',
]

export function findingFixPrompt(finding: ItemFinding, root?: string): string {
  const [check, severity, ...rest] = findingFacts(finding)
  const lines = ['Fix this agnostic-ai finding in the selected project or user configuration:', '', check, severity]
  if (root) lines.push(`Configuration root: ${root}`)
  lines.push(...rest, '', ...FIX_GUIDANCE)
  return lines.join('\n')
}
