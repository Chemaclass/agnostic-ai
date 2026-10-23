// Authority view contract, schema `authority/v1`, and the overview and
// overview contract, schema `overview/v1`.
//
// This file mirrors internal/authority/types.go and
// internal/overview/types.go field for field. The Go side is canonical:
// the server builds the views, the client renders them. Change both
// together, and docs/internal/authority-view.md with them.
//
// Four states are kept apart and never collapsed into one badge:
// declared (what the specs say), effective (what the selected coding
// tool does with it), installed (whether the generated file matches
// disk), and runtime (what a running tool enforced, never observed here).

export const SCHEMA_VERSION = 'authority/v1'
export const OVERVIEW_SCHEMA_VERSION = 'overview/v1'

export type SubjectKind = 'agent' | 'session'
export const SESSION_SUBJECT_ID = 'session'

export type Effect =
  | 'allowed'
  | 'ask'
  | 'denied'
  | 'unavailable'
  | 'default'
  | 'unresolved'

export type Resolution =
  | 'translated'
  | 'translated-unverified'
  | 'hook'
  | 'not-expressed'
  | 'agent-scope'
  | 'no-rule'
  | 'disabled'

export type InstalledStateValue =
  | 'in-sync'
  | 'stale'
  | 'not-generated'
  | 'not-checked'
  | 'not-applicable'

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
export type ResourceCoverage = 'declared' | 'complete' | 'undeclared'
export type ResourceKind =
  | 'undeclared-scope'
  | 'declared-root'
  | 'file-pattern'
  | 'command-pattern'
  | 'web'
export type ResourceBasis = 'rule-pattern' | 'declared-root' | 'undeclared'
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
  | 'provider-scope-undeclared'
  | 'credential-reach-not-inspected'
  | 'domain-defaulted'
  | 'target-not-expressed'
  | 'tool-list-unknown'
  | 'pattern-containment-approximate'
  | 'secret-read-unknown'
  | 'alternate-route-not-modeled'
  | 'installed-not-checked'
  | 'root-not-filesystem'
  | 'target-matching-unverified'
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

export interface Evidence {
  declared: 'loaded'
  installed: 'checked' | 'not-checked'
  runtime: 'not-observed'
  note: string
}

export interface Snapshot {
  revision: string
  generatedAt: string
  project: Project
  targets: string[]
  configFiles: string[]
  specs: SpecRef[]
  evidence: Evidence
}

export interface HarnessOption {
  target: string
  label: string
}

export interface ExecutionIdentity {
  kind: 'local-process'
  description: string
}

export interface InstalledFile {
  path: string
  state: InstalledStateValue
}

export interface InstalledState {
  state: InstalledStateValue
  files: InstalledFile[]
  checkedAt?: string
}

export interface Harness {
  target: string
  label: string
  available: HarnessOption[]
  identity: ExecutionIdentity
  installed: InstalledState
  permissionSupport: 'native-rules' | 'hooks-only' | 'none'
  note?: string
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
  effect: Effect
  native?: string[]
  note?: string
}

export interface RouteStatus {
  route: string
  effect: Effect
  /** The route's own flag, from the operation it stands for; never borrow the inspected operation's. */
  hypothetical: boolean
  installed?: InstalledStateValue
  note?: string
}

/** Qualifies alternateRoutes: an empty list means no checked route is open, never that no route exists. */
export interface RouteCoverage {
  checked: RouteStatus[]
  notModeled: string[]
}

export interface Operation {
  id: string
  action: string
  label: string
  declared: Effect
  effect: Effect
  resolution: Resolution
  native?: string[]
  installed: InstalledStateValue
  /** True when the effect describes what the tool would do once synced (installed is stale, not-generated, or not-checked). Word it conditionally. */
  hypothetical: boolean
  runtime: 'not-observed'
  decided?: Grant
  grants: Grant[]
  conditions: string[]
  alternateRoutes: string[]
  routeCoverage: RouteCoverage
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
  basis: ResourceBasis
  caveat?: string
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
  declared: Effect
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

export interface Explanations {
  effects: Record<string, string>
  resolution: Record<string, string>
  lists: Record<string, string>
  enforcement: Record<string, string>
  installed: Record<string, string>
  rights: Record<string, string>
}

export interface AuthorityView {
  schemaVersion: string
  snapshot: Snapshot
  harness: Harness
  agent: Subject
  agents: SubjectSummary[]
  domains: Domain[]
  providers: Provider[]
  resources: Resource[]
  edges: Edge[]
  secrets: Secret[]
  uncertainties: Uncertainty[]
  supportedActions: SupportedAction[]
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
  target?: string
  revision?: string
  changes: Change[]
}

export interface EffectChange {
  subjectId: string
  resourceId: string
  resourceLabel: string
  operationId: string
  operationLabel: string
  declaredBefore: Effect
  declaredAfter: Effect
  effectiveBefore: Effect
  effectiveAfter: Effect
  /** Only known orders are ranked: allowed over ask over denied (unavailable counts as denied). Any default or unresolved side is indeterminate. */
  direction: 'widened' | 'narrowed' | 'moved' | 'indeterminate'
  reason?: string
}

export interface ImpactSummary {
  changed: number
  widened: number
  narrowed: number
  moved: number
  indeterminate: number
}

export interface FileDiff {
  path: string
  before: string
  after: string
}

export interface ImpactResult {
  schemaVersion: string
  status: 'proposed'
  target: string
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

// ---------------------------------------------------------------------
// Overview, schema `overview/v1` (internal/overview/types.go).
// ---------------------------------------------------------------------

export type SectionId = 'context' | 'authority' | 'tools' | 'lifecycle' | 'settings'

export interface KindCount {
  kind: string
  label: string
  count: number
}

export interface KindSupport {
  kind: string
  label: string
  supported: string[]
  unsupported: string[]
}

/** A read-only check result, shown on what it concerns. */
export interface ItemFinding {
  source: 'lint' | 'doctor'
  code?: string
  severity: string
  message: string
  path?: string
  line?: number
  fix?: string
  explanation?: string
}

export interface SectionItem {
  kind: string
  name: string
  label: string
  description: string
  path: string
  /** Absolute path of a Markdown file, for opening it in an editor. */
  openPath?: string
  layer: string
  scope?: string
  /** For a hook, the id of the lifecycle stage it runs at. */
  group?: string
  /** What the item sets, key by key. */
  values: Value[]
  /** For a built-in: whether the config turns it on. */
  enabled?: boolean
  findings: ItemFinding[]
}

/** One setting: a list renders as chips, a bool as a switch, text as text. */
export interface Value {
  key: string
  type: 'text' | 'bool' | 'list'
  value?: string
  list?: string[]
}

/** One point in a session a hook can run at. */
export interface Stage {
  id: string
  label: string
  description: string
  /** The configured coding tools that run a hook here, with their own event name. */
  targets: { target: string; label: string; event: string }[]
}

export interface Section {
  id: SectionId
  label: string
  question: string
  order: number
  deepView?: string
  kinds: KindCount[]
  items: SectionItem[]
  support: KindSupport[]
  /** Findings that name no configured item. */
  findings: ItemFinding[]
  /** Lifecycle only: every stage, in session order, even when empty. */
  stages?: Stage[]
  /** Authority and Settings: every settings field, even when unset. */
  fields?: SettingField[]
}

/** One field a settings spec can set, with what each spec sets it to. */
export interface SettingField {
  key: string
  label: string
  description: string
  set: { spec: string; path: string; values: Value[] }[]
}

export type SyncState = 'in-sync' | 'drifted' | 'not-checked'

export interface TargetSync {
  state: SyncState
  files: { path: string; state: string }[]
}

export interface TargetSupport {
  target: string
  label: string
  kinds: string[]
  sync: TargetSync
}

export interface OverviewView {
  schemaVersion: string
  snapshot: Snapshot
  sections: Section[]
  targets: TargetSupport[]
}

// HTTP API served by `agnostic-ai serve`.
//
//   GET  /api/views/authority?agentId=<id>&target=<tool>  → AuthorityView
//   POST /api/views/authority/impact                      → ImpactResult (body: Proposal)
//   GET  /api/views/authority/schema                      → { schemaVersion, endpoints }
//   GET  /api/views/overview                              → OverviewView
//   GET  /api/health                                      → { ok, revision }
export const API = {
  view: '/api/views/authority',
  impact: '/api/views/authority/impact',
  schema: '/api/views/authority/schema',
  overview: '/api/views/overview',
  health: '/api/health',
} as const
