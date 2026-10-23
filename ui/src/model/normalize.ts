// Normalizes server output before the UI reads it. Go encodes a nil
// slice as `null`, and an older or newer server may omit a field the
// client expects; both would crash a renderer that maps over arrays.
// Normalizing never invents a decision: a missing effect stays missing
// and renders as its raw value, a missing list becomes an empty list.

import type {
  AuthorityView,
  Domain,
  ImpactResult,
  Operation,
  OverviewView,
  Provider,
  Resource,
  Section,
  Snapshot,
} from '../contract'

type Loose = Record<string, unknown>

function arr<T>(value: unknown): T[] {
  return Array.isArray(value) ? (value as T[]) : []
}

function obj(value: unknown): Loose {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as Loose) : {}
}

function rec(value: unknown): Record<string, string> {
  return obj(value) as Record<string, string>
}

export function normalizeSnapshot(raw: unknown): Snapshot {
  const s = obj(raw)
  const evidence = obj(s.evidence)
  return {
    ...(s as unknown as Snapshot),
    revision: String(s.revision ?? ''),
    generatedAt: String(s.generatedAt ?? ''),
    project: { root: '', name: '', ...(obj(s.project) as object) } as Snapshot['project'],
    targets: arr<string>(s.targets),
    configFiles: arr<string>(s.configFiles),
    specs: arr(s.specs),
    evidence: {
      declared: 'loaded',
      installed: 'not-checked',
      runtime: 'not-observed',
      note: '',
      ...(evidence as object),
    } as Snapshot['evidence'],
  }
}

function normalizeOperation(raw: unknown): Operation {
  const o = obj(raw)
  const coverage = obj(o.routeCoverage)
  return {
    ...(o as unknown as Operation),
    grants: arr(o.grants),
    conditions: arr(o.conditions),
    alternateRoutes: arr(o.alternateRoutes),
    routeCoverage: {
      checked: arr<Record<string, unknown>>(coverage.checked).map((r) => ({ ...r, hypothetical: r.hypothetical === true })) as Operation['routeCoverage']['checked'],
      notModeled: arr(coverage.notModeled),
    },
    enforcement: arr(o.enforcement),
    hypothetical: o.hypothetical === true,
    runtime: 'not-observed',
    explanation: String(o.explanation ?? ''),
  }
}

function normalizeResource(raw: unknown): Resource {
  const r = obj(raw)
  return {
    ...(r as unknown as Resource),
    domainIds: arr(r.domainIds),
    identity: obj(r.identity),
    operations: arr(r.operations).map(normalizeOperation),
    labelResolved: r.labelResolved !== false,
  }
}

function normalizeProvider(raw: unknown): Provider {
  const p = obj(raw)
  return {
    ...(p as unknown as Provider),
    domainIds: arr(p.domainIds),
    resourceIds: arr(p.resourceIds),
    secretIds: arr(p.secretIds),
  }
}

function normalizeDomain(raw: unknown): Domain {
  const d = obj(raw)
  return {
    ...(d as unknown as Domain),
    examples: arr(d.examples),
    providerIds: arr(d.providerIds),
    resourceIds: arr(d.resourceIds),
    summary: { allowed: 0, ask: 0, denied: 0, unavailable: 0, default: 0, unresolved: 0, ...(obj(d.summary) as object) },
  }
}

export function normalizeAuthorityView(raw: unknown): AuthorityView {
  const v = obj(raw)
  const harness = obj(v.harness)
  const installed = obj(harness.installed)
  const agent = obj(v.agent)
  const toolScope = obj(agent.toolScope)
  const mcpServers = obj(agent.mcpServers)
  const explanations = obj(v.explanations)
  return {
    ...(v as unknown as AuthorityView),
    schemaVersion: String(v.schemaVersion ?? ''),
    snapshot: normalizeSnapshot(v.snapshot),
    harness: {
      ...(harness as unknown as AuthorityView['harness']),
      available: arr(harness.available),
      identity: { kind: 'local-process', description: '', ...(obj(harness.identity) as object) } as AuthorityView['harness']['identity'],
      installed: { state: 'not-checked', ...(installed as object), files: arr(installed.files) } as AuthorityView['harness']['installed'],
    },
    agent: {
      ...(agent as unknown as AuthorityView['agent']),
      toolScope: {
        mode: 'unrestricted',
        ...(toolScope as object),
        declared: arr(toolScope.declared),
        capabilities: arr(toolScope.capabilities),
      } as AuthorityView['agent']['toolScope'],
      readonly: agent.readonly === true,
      mcpServers: { mode: 'all', ...(mcpServers as object), names: arr(mcpServers.names) } as AuthorityView['agent']['mcpServers'],
    },
    agents: arr(v.agents),
    domains: arr(v.domains).map(normalizeDomain),
    providers: arr(v.providers).map(normalizeProvider),
    resources: arr(v.resources).map(normalizeResource),
    edges: arr(v.edges),
    secrets: arr(v.secrets).map((s) => {
      const secret = obj(s)
      return {
        ...(secret as unknown as AuthorityView['secrets'][number]),
        providerIds: arr(secret.providerIds),
        referencedBy: arr(secret.referencedBy),
        rights: { discover: 'unknown', readValue: 'unknown', use: 'unknown', manage: 'unknown', ...(obj(secret.rights) as object) },
      } as AuthorityView['secrets'][number]
    }),
    uncertainties: arr(v.uncertainties).map((u) => {
      const item = obj(u)
      return { ...(item as unknown as AuthorityView['uncertainties'][number]), relatedIds: arr(item.relatedIds) }
    }),
    supportedActions: arr(v.supportedActions),
    explanations: {
      effects: rec(explanations.effects),
      resolution: rec(explanations.resolution),
      lists: rec(explanations.lists),
      enforcement: rec(explanations.enforcement),
      installed: rec(explanations.installed),
      rights: rec(explanations.rights),
    },
  }
}

function normalizeSection(raw: unknown): Section {
  const s = obj(raw)
  return {
    ...(s as unknown as Section),
    kinds: arr(s.kinds),
    items: arr(s.items).map((i) => {
      const item = obj(i)
      return { ...(item as unknown as Section['items'][number]), findings: arr(item.findings) }
    }),
    support: arr(s.support).map((k) => {
      const kind = obj(k)
      return { ...(kind as unknown as Section['support'][number]), supported: arr(kind.supported), unsupported: arr(kind.unsupported) }
    }),
    findings: arr(s.findings),
  }
}

export function normalizeOverviewView(raw: unknown): OverviewView {
  const v = obj(raw)
  return {
    ...(v as unknown as OverviewView),
    schemaVersion: String(v.schemaVersion ?? ''),
    snapshot: normalizeSnapshot(v.snapshot),
    sections: arr(v.sections).map(normalizeSection),
    targets: arr(v.targets).map((t) => {
      const target = obj(t)
      const sync = obj(target.sync)
      return {
        ...(target as unknown as OverviewView['targets'][number]),
        kinds: arr(target.kinds),
        sync: { state: sync.state === 'in-sync' || sync.state === 'drifted' ? sync.state : 'not-checked', files: arr(sync.files) },
      }
    }),
  }
}

export function normalizeImpactResult(raw: unknown): ImpactResult {
  const r = obj(raw)
  return {
    ...(r as unknown as ImpactResult),
    valid: r.valid === true,
    problems: arr(r.problems),
    changes: arr(r.changes),
    summary: { changed: 0, widened: 0, narrowed: 0, moved: 0, indeterminate: 0, ...(obj(r.summary) as object) },
    affectedSubjects: arr(r.affectedSubjects),
    diff: arr(r.diff),
    ...(r.after ? { after: normalizeAuthorityView(r.after) } : {}),
  }
}
