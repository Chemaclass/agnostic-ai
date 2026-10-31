import type { ReactNode } from 'react'
import { Icon } from './ui'
import { documentationUrl } from '../model/documentation'

const TOPICS: Record<string, { path: string; label: string }> = {
  instructions: { path: '/docs/configuration/#entry-point-files', label: 'shared instructions' },
  rule: { path: '/docs/spec-format/rules/', label: 'rules' },
  agent: { path: '/docs/spec-format/agents/', label: 'agents' },
  skill: { path: '/docs/spec-format/skills/', label: 'skills' },
  command: { path: '/docs/spec-format/commands/', label: 'commands' },
  review: { path: '/docs/spec-format/reviews/', label: 'reviews' },
  ignore: { path: '/docs/spec-format/ignore/', label: 'ignore patterns' },
  mcp: { path: '/docs/spec-format/mcps/', label: 'MCP servers' },
  native: { path: '/docs/cli-reference/start/#import', label: 'importing native configuration' },
  environment: { path: '/docs/spec-format/environments/', label: 'environments' },
  hook: { path: '/docs/spec-format/hooks/', label: 'hooks' },
  builtin: { path: '/docs/configuration/#built-ins', label: 'built-ins' },
  settings: { path: '/docs/spec-format/settings/', label: 'settings' },
  permissions: { path: '/docs/spec-format/settings/#permission-rules', label: 'permission rules' },
  config: { path: '/docs/configuration/', label: 'configuration files' },
  usage: { path: '/docs/cli-reference/inspect/#rank-context-contributions', label: 'context size' },
  support: { path: '/docs/target-behavior/', label: 'coding tool support' },
  findings: { path: '/docs/cli-reference/check/', label: 'configuration checks' },
}

export function SectionHeading({ topic, id, focusable, children }: { topic: string; id?: string; focusable?: boolean; children: ReactNode }) {
  const doc = TOPICS[topic] ?? { path: '/docs/spec-format/', label: 'specs' }
  return (
    <div className="section-head section-help-heading">
      <h2 id={id} tabIndex={focusable ? -1 : undefined}>{children}</h2>
      <a className="section-doc-link" href={documentationUrl(doc.path)} target="_blank" rel="noopener noreferrer" aria-label={`Documentation for ${doc.label} (opens in a new tab)`} title={`Read about ${doc.label} (opens in a new tab)`}>
        <span>Docs</span><Icon name="arrow" />
      </a>
    </div>
  )
}
