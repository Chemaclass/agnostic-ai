// Fixture loading for the model tests. The tests read the bundled mock
// by default; set OVERVIEW_FIXTURE (a path relative to ui/) to run the
// same tests against real server output.

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import type { OverviewView } from '../contract'
import { normalizeOverviewView } from './normalize'

export const OVERVIEW_FIXTURE = process.env.OVERVIEW_FIXTURE ?? 'src/fixtures/overview.mock.json'

export function readJson(path: string): unknown {
  return JSON.parse(readFileSync(resolve(process.cwd(), path), 'utf8'))
}

export function loadOverviewFixture(path = OVERVIEW_FIXTURE): OverviewView {
  return normalizeOverviewView(readJson(path))
}
