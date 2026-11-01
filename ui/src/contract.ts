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
  local?: boolean
}

export interface KindSupport {
  kind: string
  label: string
  supported: string[]
  unsupported: string[]
}

/** The code of the finding that stands in for a lint run that stopped early. */
export const LINT_INCOMPLETE = 'lint-incomplete'

/** A read-only check result, shown on what it concerns. */
export interface ItemFinding {
  source: 'lint' | 'doctor'
  code?: string
  severity: string
  message: string
  path?: string
  line?: number
  /** Absolute path of the file it names, when that file is on the server's disk. */
  openPath?: string
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
  /** For a built-in: the personal config turns it on or off against the shared list. */
  local?: boolean
  /** For a personal settings file: the dotted keys it sets itself. */
  localKeys?: string[]
  /** For an MCP server: the tools `agnostic-ai mcp tools` last listed. */
  tools?: ToolListing
  findings: ItemFinding[]
}

export interface ToolListing {
  state: 'listed' | 'stale' | 'failed' | 'not-listed'
  listedAt?: string
  error?: string
  tools: { name: string; title?: string; description?: string }[]
  /** Refreshes the listing. */
  command: string
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
  kinds: KindCount[]
  items: SectionItem[]
  support: KindSupport[]
  /** Findings that name no configured item. */
  findings: ItemFinding[]
  /** Lifecycle only: every stage, in session order, even when empty. */
  stages?: Stage[]
  /** Authority and Settings: every settings field, even when unset. */
  fields?: SettingField[]
  /** Tools only: MCP servers found in a tool's own config that no spec covers. Names and paths only, never a server's values. */
  native?: NativeServer[]
}

/** One MCP server name found in tools' own config files. `import` is absent when no import reads the file that holds it. */
export interface NativeServer {
  name: string
  holders: NativeHolder[]
  import?: string
}

/** One tool config file that defines a native server. */
export interface NativeHolder {
  target: string
  label: string
  path: string
  scope: 'project' | 'user'
  transport?: 'stdio' | 'http' | 'sse' | 'streamable-http' | 'remote'
}

export interface MCPSpecPreview {
  file: string
  spec: string
  variables: string[]
  targets: string[]
  syncCommand: string
  expected: string
}

export type PermissionKey = 'permissions.allow' | 'permissions.ask' | 'permissions.deny'

export interface PermissionEditRequest {
  action: 'preview' | 'apply'
  audience: 'shared' | 'personal'
  path?: string
  key: PermissionKey
  rules: string[] | null
  before: string[] | null
  expected?: string
}

export interface PermissionEditPreview {
  schemaVersion: 'permission-edit/v1'
  file: string
  key: PermissionKey
  audience: 'shared' | 'personal'
  before: string[] | null
  rules: string[] | null
  diff: string
  expected: string
  syncCommand: string
}

/** One field a settings spec can set, with what each spec sets it to. */
export interface SettingField {
  key: string
  label: string
  description: string
  /** Each spec that sets the field; `local` marks a personal file Git ignores. */
  set: { spec: string; path: string; values: Value[]; local?: boolean }[]
  /** `localPath` is the personal file a personal edit of `path` writes. `toolAllowed` lists the values one tool accepts, for tools with a fixed list. */
  editable?: { path?: string; paths?: string[]; sources?: { path: string; spec: string; localPath: string; disabledReason?: string }[]; kind: 'text' | 'number' | 'boolean' | 'permission-list'; allowed?: string[]; toolAllowed?: Record<string, string[]>; disabledReason?: string; syncCommand?: string; localPath?: string }
  /** For model and effort: the value sync writes for each configured tool, and why. */
  resolved?: TargetValue[]
}

/** Where a value comes from: a settings spec, or a config key when `spec` is absent. `reason` says why a losing value does not win. */
export interface ValueOrigin {
  spec?: string
  path: string
  value?: string
  tier?: string
  local?: boolean
  /** The value is the tool's own entry in the per-tool map, such as `model.codex`, not the value for every tool. */
  perTool?: boolean
  reason?: string
}

/**
 * The value sync writes for one tool. An absent `value` means sync writes none and the tool keeps a default agnostic-ai does not know.
 * `dropped` is the sync note when the tool's files have no place for the value; `checked` is false when sync did not render the tool.
 */
export interface TargetValue {
  target: string
  label: string
  value?: string
  alias?: string
  tier?: string
  from?: ValueOrigin
  dropped?: string
  checked: boolean
  others: ValueOrigin[]
  /** Specs that also set `x-<target>.model`, a native key sync writes as is. */
  native?: ValueOrigin[]
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

export interface ToolChoice { target: string; label: string; description: string; enabled: boolean; local: boolean; detected: boolean }
export type ToolCatalogSource = 'default' | 'shared' | 'personal'
export interface ToolCatalog { source: ToolCatalogSource; items: ToolChoice[] }

export interface OverviewView {
  schemaVersion: string
  snapshot: Snapshot
  sections: Section[]
  targets: TargetSupport[]
  tools: ToolCatalog
  /** True for user configuration, which has no project to size. */
  global?: boolean
}

// ---------------------------------------------------------------------
// Context size, schema `1`: the `explain --context` report for one target.
// Words and bytes measure planned instruction text, not model tokens.
// ---------------------------------------------------------------------

export const CONTEXT_SCHEMA_VERSION = '1'

export type ContextLoad = 'startup' | 'file-scope' | 'on-demand'

export interface ContextSize {
  words: number
  bytes: number
}

export interface ContextContribution extends ContextSize {
  /** Project-relative path with forward slashes. */
  source: string
  category: string
  load: ContextLoad
}

export interface ContextOpportunity {
  kind: string
  target: string
  load: ContextLoad
  passage: string
  copies: number
  /** One copy. */
  size: ContextSize
  /** (copies - 1) * size. */
  reduction: ContextSize
  sources: { source: string; category: string }[]
  suggestion: string
}

export interface ContextReport {
  version: string
  command: string
  target: string
  /** The project-relative file the report counted file-scoped rules for, when one was given. */
  file?: string
  note: string
  startup: ContextSize
  file_scope: ContextSize
  on_demand: ContextSize
  /** Startup first, then file-scope, then on-demand; largest first within each. */
  contributions: ContextContribution[]
  /** Largest reduction first. */
  opportunities: ContextOpportunity[]
}

export interface ContextView {
  schemaVersion: string
  /** False for user configuration (`serve --global`), with the reason. */
  supported: boolean
  reason: string
  root?: string
  targets: string[]
  /** Targets that accept a file, so scoped rules can count as file-scoped. */
  fileTargets: string[]
  target: string
  /** lint.instructions-words, the startup words LINT011 allows; absent for user configuration or an older server. */
  startupWordBudget?: number
  report?: ContextReport
}

// HTTP API served by `agnostic-ai serve`.
//
//   GET  /api/views/authority?agentId=<id>&target=<tool>  → AuthorityView
//   POST /api/views/authority/impact                      → ImpactResult (body: Proposal)
//   GET  /api/views/authority/schema                      → { schemaVersion, endpoints }
//   GET  /api/views/overview                              → OverviewView
//   GET  /api/views/context?target=<tool>&file=<path>     → ContextView
//   GET  /api/health                                      → { ok, root, global, ui, projectRoot? }
//   POST /api/switch                                      → SwitchReply (body: {}); 409 { missing, root } when there is no global setup
//   POST /api/global/create                               → GlobalCreateReply (body: { git })
export const API = {
  view: '/api/views/authority',
  impact: '/api/views/authority/impact',
  schema: '/api/views/authority/schema',
  overview: '/api/views/overview',
  context: '/api/views/context',
  editBuiltin: '/api/edit/builtin',
  editTargets: '/api/edit/targets',
  editValue: '/api/edit/value',
  editField: '/api/edit/field',
  editPermissions: '/api/edit/permissions',
  editMCP: '/api/edit/mcp',
  health: '/api/health',
  switch: '/api/switch',
  globalCreate: '/api/global/create',
} as const

/** The global setup folder the page just created. */
export interface GlobalCreateReply {
  root: string
  created: string[]
  git: string
}

/** The server for the other configuration, found running or just started. */
export interface SwitchReply {
  url: string
  root: string
  global: boolean
  started: boolean
}
