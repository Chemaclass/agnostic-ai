import { createContext } from 'react'
import type { TargetSupport } from '../contract'
import { useApp } from './app-context'

export const ConfigurationRoot = createContext<string | undefined>(undefined)
export const ConfiguredTools = createContext<TargetSupport[]>([])
export const PersonalBuiltins = createContext(false)

export function useEntry(id: string): { 'data-entry': string; tabIndex?: number; linked: boolean } {
  const linked = useApp().state.entry === id
  return { 'data-entry': id, tabIndex: linked ? -1 : undefined, linked }
}
