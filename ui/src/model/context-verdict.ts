import type { ContextSize } from '../contract'

export interface StartupVerdict {
  /** 'over' only when a budget exists and startup words exceed it. */
  state: 'within' | 'over' | 'unknown'
  /** Empty when there is no budget: the startup figure already says the size. */
  text: string
}

const words = (n: number) => `${n.toLocaleString('en-US')} ${n === 1 ? 'word' : 'words'}`

/** Compares the startup words with the budget the server reports (lint.instructions-words). */
export function startupVerdict(startup: ContextSize, budget: number | undefined): StartupVerdict {
  if (!budget || budget <= 0) return { state: 'unknown', text: '' }
  const limit = `the ${budget.toLocaleString('en-US')}-word startup budget`
  if (startup.words <= budget) return { state: 'within', text: `Within ${limit}.` }
  return { state: 'over', text: `${words(startup.words - budget)} over ${limit}.` }
}

/** The verdict for a prompt: a coding agent can act on the size, the config key and the lint code. */
export function startupBudgetLine(startup: ContextSize, budget: number | undefined): string {
  const size = `Startup is ${words(startup.words)}`
  if (!budget || budget <= 0) return `${size}.`
  const limit = `the ${budget.toLocaleString('en-US')}-word lint budget (lint.instructions-words)`
  if (startup.words <= budget) return `${size}, within ${limit}.`
  return `${size}, ${words(startup.words - budget)} over ${limit}; agnostic-ai lint reports LINT011.`
}

/** The budget as a fact beside the page verdict; undefined without a budget. */
export function budgetFact(startup: ContextSize, budget: number | undefined): { over: boolean; text: string } | undefined {
  if (!budget || budget <= 0) return undefined
  const limit = `the ${budget.toLocaleString('en-US')}-word budget`
  if (startup.words <= budget) return { over: false, text: `Within ${limit}` }
  return { over: true, text: `${words(startup.words - budget)} over ${limit}` }
}
