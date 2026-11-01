const WEBSITE_URL = 'https://agnostic-ai.org/'

export function documentationUrl(path: string): string {
  return new URL(path, WEBSITE_URL).href
}

export interface DocTopic { path: string; label: string }

/** Docs page (and optional anchor) behind each section heading's Docs link. Paths are site paths; the test checks them against docs/site/content. */
export const DOC_TOPICS: Record<string, DocTopic> = {
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
  hook: { path: '/docs/spec-format/hooks/#a-typical-session', label: 'the session timeline and hooks' },
  builtin: { path: '/docs/configuration/#built-ins', label: 'built-ins' },
  settings: { path: '/docs/spec-format/settings/', label: 'settings' },
  permissions: { path: '/docs/spec-format/settings/#permission-rules', label: 'permission rules' },
  config: { path: '/docs/configuration/', label: 'configuration files' },
  usage: { path: '/docs/cli-reference/inspect/#rank-context-contributions', label: 'context size' },
  support: { path: '/docs/target-behavior/', label: 'coding tool support' },
  findings: { path: '/docs/cli-reference/check/', label: 'configuration checks' },
}

export const DOC_FALLBACK: DocTopic = { path: '/docs/spec-format/', label: 'specs' }
