import type { ContextSize } from '../contract'

export interface StartupVerdict {
  /** 'over' only when a budget exists and startup words exceed it. */
  state: 'within' | 'over' | 'unknown'
  text: string
}

const words = (n: number) => `${n.toLocaleString('en-US')} ${n === 1 ? 'word' : 'words'}`

/** Compares the startup words with the lint budget the server reports; says nothing when there is none. */
export function startupVerdict(startup: ContextSize, budget: number | undefined): StartupVerdict {
  const size = `Startup is ${words(startup.words)}`
  if (!budget || budget <= 0) return { state: 'unknown', text: `${size}.` }
  const limit = `the ${budget.toLocaleString('en-US')}-word lint budget (lint.instructions-words)`
  if (startup.words <= budget) return { state: 'within', text: `${size}, within ${limit}.` }
  return { state: 'over', text: `${size}, ${words(startup.words - budget)} over ${limit}; agnostic-ai lint reports LINT011.` }
}
