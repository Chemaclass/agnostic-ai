// Fixture loading for the model tests. The tests read the handwritten
// mock by default; set AUTHORITY_FIXTURE or OVERVIEW_FIXTURE (a path
// relative to ui/) to run the same tests against real server output,
// for example src/fixtures/authority.demo.json. Assertions that only
// hold for the mock's exact names are guarded by `usingMockAuthority`.

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import type { AuthorityView, OverviewView } from '../contract'
import { normalizeAuthorityView, normalizeOverviewView } from './normalize'

export const AUTHORITY_FIXTURE = process.env.AUTHORITY_FIXTURE ?? 'src/fixtures/authority.mock.json'
export const OVERVIEW_FIXTURE = process.env.OVERVIEW_FIXTURE ?? 'src/fixtures/overview.mock.json'
export const usingMockAuthority = !process.env.AUTHORITY_FIXTURE
export const usingMockOverview = !process.env.OVERVIEW_FIXTURE

export function readJson(path: string): unknown {
  return JSON.parse(readFileSync(resolve(process.cwd(), path), 'utf8'))
}

export function loadAuthorityFixture(path = AUTHORITY_FIXTURE): AuthorityView {
  return normalizeAuthorityView(readJson(path))
}

export function loadOverviewFixture(path = OVERVIEW_FIXTURE): OverviewView {
  return normalizeOverviewView(readJson(path))
}
