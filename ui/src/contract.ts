// Authority view contract, schema `authority/v1`.
//
// This file mirrors internal/authority/types.go field for field. The Go
// side is canonical: the server builds the view, the client renders it.
// Change both together, and docs/internal/authority-view.md with them.

export const SCHEMA_VERSION = 'authority/v1'

export type SubjectKind = 'agent' | 'session'
export const SESSION_SUBJECT_ID = 'session'

export type Effect =
  | 'allowed'
  | 'ask'
  | 'denied'
  | 'unavailable'
  | 'default'
  | 'unresolved'

export type DomainId =
  | 'source-control'
  | 'cloud-deployment'
  | 'data-storage'
  | 'filesystem-execution'
  | 'web-browser'
  | 'communication'
  | 'secrets-identity'
  | 'ai-agent-ops'
  | (string & {})

export type DomainResolution = 'curated' | 'builtin' | 'default'
export type ProviderKind = 'mcp' | 'builtin'
export type ProviderStatus = 'configured' | 'disabled' | 'unavailable-to-agent'
export type ResourceCoverage = 'declared' | 'complete' | 'unresolved'
export type ResourceKind =
  | 'mcp-server'
  | 'declared-root'
  | 'file-pattern'
  | 'command-pattern'
  | 'web'
export type GrantList =
  | 'allow'
  | 'ask'
  | 'deny'
  | 'protected'
  | 'can'
  | 'tools'
  | 'readonly'
  | 'mcpServers'
export type GrantScope = 'project' | 'agent'
export type EnforcementStatus = 'native-rule' | 'hook' | 'not-expressed' | 'not-applicable'
export type SecretRight = 'allowed' | 'unavailable' | 'not-readable' | 'unknown'
export type Severity = 'info' | 'warning'
export type UncertaintyCode =
  | 'runtime-not-observed'
  | 'provider-resources-unresolved'
  | 'domain-defaulted'
  | 'target-not-expressed'
  | 'tool-list-unknown'
  | 'pattern-containment-approximate'
  | 'secret-read-unknown'
  | (string & {})

export interface Source {
  path: string
  layer: string
  line?: number
}

export interface SpecRef {
  kind: string
  name: string
  path: string
  layer: string
}

export interface Project {
  root: string
  name: string
}

export interface Snapshot {
  revision: string
  generatedAt: string
  project: Project
  targets: string[]
  specs: SpecRef[]
}

export interface SubjectSummary {
  id: string
  label: string
  description: string
  kind: SubjectKind
}

export interface ToolScope {
  mode: 'unrestricted' | 'listed'
  field?: 'can' | 'tools' | ''
  declared: string[]
  capabilities: string[]
}

export interface MCPScope {
  mode: 'all' | 'listed'
  names: string[]
}

export interface Subject extends SubjectSummary {
  source?: Source
  toolScope: ToolScope
  readonly: boolean
  mcpServers: MCPScope
}

export interface EffectCounts {
  allowed: number
  ask: number
  denied: number
  unavailable: number
  default: number
  unresolved: number
}

export interface Domain {
  id: DomainId
  label: string
  description: string
  examples: string[]
  order: number
  providerIds: string[]
  resourceIds: string[]
  summary: EffectCounts
}

export interface Transport {
  type: string
  endpoint?: string
}

export interface Provider {
  id: string
  label: string
  description: string
  kind: ProviderKind
  domainIds: DomainId[]
  domainResolution: DomainResolution
  status: ProviderStatus
  transport?: Transport
  source?: Source
  resourceIds: string[]
  secretIds: string[]
  resourceCoverage: ResourceCoverage
  coverageNote?: string
}

export interface Identity {
  uri?: string
  pattern?: string
  server?: string
}

export interface Matches {
  count: number
  samples: string[]
  truncated: boolean
}

export interface Grant {
  rule: string
  list: GrantList
  scope: GrantScope
  source: Source
  wins: boolean
  note?: string
}

export interface Enforcement {
  target: string
  label: string
  status: EnforcementStatus
  native?: string[]
  note?: string
}

export interface Operation {
  id: string
  action: string
  label: string
  effect: Effect
  decided?: Grant
  grants: Grant[]
  conditions: string[]
  enforcement: Enforcement[]
  explanation: string
}

export interface Resource {
  id: string
  label: string
  description?: string
  kind: ResourceKind
  providerId: string
  domainIds: DomainId[]
  identity: Identity
  operations: Operation[]
  matches?: Matches
  labelResolved: boolean
  unresolvedReason?: string
}

export interface Edge {
  id: string
  subjectId: string
  resourceId: string
  operationId: string
  effect: Effect
  label: string
}

export interface SecretRef {
  source: Source
  field: string
}

export interface SecretRights {
  discover: SecretRight
  readValue: SecretRight
  use: SecretRight
  manage: SecretRight
}

export interface Secret {
  id: string
  name: string
  label: string
  providerIds: string[]
  referencedBy: SecretRef[]
  rights: SecretRights
  explanation: string
}

export interface Uncertainty {
  id: string
  code: UncertaintyCode
  severity: Severity
  message: string
  relatedIds: string[]
}

export interface SupportedAction {
  id: string
  method?: string
  path?: string
  description: string
  status: 'available' | 'unsupported'
}

export interface RuntimeStatus {
  enforcement: 'not-observed'
  note: string
}

export interface Explanations {
  effects: Record<string, string>
  lists: Record<string, string>
  enforcement: Record<string, string>
  rights: Record<string, string>
}

export interface AuthorityView {
  schemaVersion: string
  snapshot: Snapshot
  agent: Subject
  agents: SubjectSummary[]
  domains: Domain[]
  providers: Provider[]
  resources: Resource[]
  edges: Edge[]
  secrets: Secret[]
  uncertainties: Uncertainty[]
  supportedActions: SupportedAction[]
  runtime: RuntimeStatus
  explanations: Explanations
}

export interface Change {
  op: 'add' | 'remove'
  list: 'allow' | 'ask' | 'deny'
  rule: string
  source?: string
}

export interface Proposal {
  agentId: string
  revision?: string
  changes: Change[]
}

export interface EffectChange {
  subjectId: string
  resourceId: string
  resourceLabel: string
  operationId: string
  operationLabel: string
  before: Effect
  after: Effect
  direction: 'widened' | 'narrowed' | 'moved'
}

export interface ImpactSummary {
  changed: number
  widened: number
  narrowed: number
  moved: number
}

export interface FileDiff {
  path: string
  before: string
  after: string
}

export interface ImpactResult {
  schemaVersion: string
  baseRevision: string
  proposedRevision: string
  valid: boolean
  problems: string[]
  changes: EffectChange[]
  summary: ImpactSummary
  affectedSubjects: string[]
  diff: FileDiff[]
  after?: AuthorityView
}

// HTTP API served by `agnostic-ai serve`.
//
//   GET  /api/views/authority?agentId=<id>   → AuthorityView
//   POST /api/views/authority/impact         → ImpactResult   (body: Proposal)
//   GET  /api/views/authority/schema         → { schemaVersion, endpoints }
//   GET  /api/health                         → { ok, revision }
export const API = {
  view: '/api/views/authority',
  impact: '/api/views/authority/impact',
  schema: '/api/views/authority/schema',
  health: '/api/health',
} as const
