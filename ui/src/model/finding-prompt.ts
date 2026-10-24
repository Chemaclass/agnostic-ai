import type { ItemFinding } from '../contract'

export function findingFixPrompt(finding: ItemFinding, root?: string): string {
  const lines = [
    'Fix this agnostic-ai finding in the selected project or user configuration:',
    '',
    `Check: ${finding.source}${finding.code ? ` (${finding.code})` : ''}`,
    `Severity: ${finding.severity}`,
  ]
  if (root) lines.push(`Configuration root: ${root}`)
  if (finding.path) lines.push(`Source: ${finding.path}${finding.line ? `:${finding.line}` : ''}`)
  else if (finding.line) lines.push(`Line: ${finding.line}`)
  lines.push(`Finding: ${finding.message}`)
  if (finding.explanation) lines.push(`Explanation: ${finding.explanation}`)
  if (finding.fix) lines.push(`Suggested fix: ${finding.fix}`)
  lines.push(
    '',
    'Inspect the source and confirm the cause before editing. Change the canonical specs, not generated files. Preserve unrelated content and tool behavior. Keep the same project or global scope. Do not disable the check or raise its budget to hide the finding.',
    '',
    'Run the relevant agnostic-ai checks after the change. If generated files need updating, review sync --plan, then run sync and sync --check. Report what changed and any remaining findings.',
  )
  return lines.join('\n')
}
