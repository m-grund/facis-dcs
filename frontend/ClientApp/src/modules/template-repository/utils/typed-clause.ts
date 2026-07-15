import type { DcsTypedClauseInstance } from '@/models/dcs-jsonld'

/**
 * Typed-clause helpers (ADR-10): a typed clause instance lives nested in a
 * dcs:Clause block (dcs:typedClause) and its dcs:content carries a
 * human-readable summary derived from the typed values, so previews/PDF
 * rendering that only understand clause text still show something faithful.
 */

/** "amount: 100 · currency: EUR" from an instance's dcs:-prefixed values. */
export function typedClauseValuesSummary(instance: DcsTypedClauseInstance): string {
  return Object.entries(instance)
    .filter(([key]) => key !== '@type')
    .map(([key, value]) => `${key.replace(/^dcs:/, '')}: ${String(value)}`)
    .join(' · ')
}

/** The value entries of an instance, for read-only display. */
export function typedClauseEntries(instance: DcsTypedClauseInstance): { key: string; value: string }[] {
  return Object.entries(instance)
    .filter(([key]) => key !== '@type')
    .map(([key, value]) => ({ key: key.replace(/^dcs:/, ''), value: String(value) }))
}
