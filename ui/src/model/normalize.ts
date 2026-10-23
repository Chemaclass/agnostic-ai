// Normalizes server output before the UI reads it. Go encodes a nil
// slice as `null`, and an older or newer server may omit a field the
// client expects; both would crash a renderer that maps over arrays.
// Normalizing never invents a decision: a missing effect stays missing
// and renders as its raw value, a missing list becomes an empty list.

import type { OverviewView, Section, Snapshot } from '../contract'

type Loose = Record<string, unknown>

function arr<T>(value: unknown): T[] {
  return Array.isArray(value) ? (value as T[]) : []
}

function obj(value: unknown): Loose {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as Loose) : {}
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

function normalizeSection(raw: unknown): Section {
  const s = obj(raw)
  return {
    ...(s as unknown as Section),
    kinds: arr(s.kinds),
    items: arr(s.items).map((i) => {
      const item = obj(i)
      return {
        ...(item as unknown as Section['items'][number]),
        findings: arr(item.findings),
        ...(item.tools ? { tools: { ...(obj(item.tools) as unknown as NonNullable<Section['items'][number]['tools']>), tools: arr(obj(item.tools).tools) } } : {}),
        values: arr<Loose>(item.values).map((v) => ({ ...(v as unknown as Section['items'][number]['values'][number]), ...(v.list ? { list: arr(v.list) } : {}) })),
      }
    }),
    support: arr(s.support).map((k) => {
      const kind = obj(k)
      return { ...(kind as unknown as Section['support'][number]), supported: arr(kind.supported), unsupported: arr(kind.unsupported) }
    }),
    findings: arr(s.findings),
    ...(s.fields
      ? {
          fields: arr<Loose>(s.fields).map((f) => ({
            ...(f as unknown as NonNullable<Section['fields']>[number]),
            set: arr<Loose>(f.set).map((src) => ({ ...(src as unknown as NonNullable<Section['fields']>[number]['set'][number]), values: arr(src.values) })),
          })),
        }
      : {}),
    ...(s.stages ? { stages: arr<Loose>(s.stages).map((st) => ({ ...(st as unknown as NonNullable<Section['stages']>[number]), targets: arr(st.targets) })) } : {}),
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
