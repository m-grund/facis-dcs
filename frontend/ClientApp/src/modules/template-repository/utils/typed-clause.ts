import type { DcsTypedClauseInstance } from '@/models/dcs-jsonld'
import type { ClauseCatalogType } from '@/services/semantic-hub-service'

/**
 * Typed-clause helpers: a typed clause instance lives nested in a
 * dcs:Clause block (dcs:typedClause) and its dcs:content carries a
 * human-readable summary derived from the typed values, so previews/PDF
 * rendering that only understand clause text still show something faithful.
 */

/** Local name of an IRI or prefixed term ("…#legalName" / "gx:legalName" → "legalName"). */
function termLocalName(term: string): string {
  const cut = Math.max(term.lastIndexOf('#'), term.lastIndexOf('/'), term.lastIndexOf(':'))
  return cut >= 0 && cut < term.length - 1 ? term.slice(cut + 1) : term
}

function summarizeValue(value: unknown): string {
  if (Array.isArray(value)) return value.map(summarizeValue).join(', ')
  if (typeof value === 'object' && value !== null) {
    const nested = value as Record<string, unknown>
    if (typeof nested['@value'] === 'string' || typeof nested['@value'] === 'number' || typeof nested['@value'] === 'boolean') {
      return String(nested['@value'])
    }
    if (typeof nested['@id'] === 'string') return nested['@id']
    return valueEntries(nested)
      .map(({ key, value: v }) => `${key}: ${v}`)
      .join(', ')
  }
  return String(value)
}

function valueEntries(node: Record<string, unknown>): { key: string; value: string }[] {
  return Object.entries(node)
    .filter(([key]) => key !== '@type' && key !== '@id')
    .map(([key, value]) => ({ key: termLocalName(key), value: summarizeValue(value) }))
}

/** "amount: 100 · currency: EUR" from an instance's values, any namespace. */
export function typedClauseValuesSummary(instance: DcsTypedClauseInstance): string {
  return valueEntries(instance)
    .map(({ key, value }) => `${key}: ${value}`)
    .join(' · ')
}

/** The value entries of an instance, for read-only display. */
export function typedClauseEntries(instance: DcsTypedClauseInstance): { key: string; value: string }[] {
  return valueEntries(instance)
}

/** Expands "dcs:PaymentClause" to its full IRI via the hub context's prefixes; full IRIs pass through. */
export function expandPrefixedTerm(term: string, prefixes: Record<string, string>): string {
  const colon = term.indexOf(':')
  if (colon <= 0) return term
  const namespace = prefixes[term.slice(0, colon)]
  return namespace ? namespace + term.slice(colon + 1) : term
}

/** All rdf:type IRIs an instance carries (its @type may be a string or an array). */
export function instanceTypeIris(instance: DcsTypedClauseInstance): string[] {
  const raw = instance['@type']
  return (Array.isArray(raw) ? raw : [raw]).filter((t): t is string => typeof t === 'string')
}

/**
 * Resolves a stored typed clause instance back to its hub clause-catalog
 * entry: catalog types are hub-context-compacted terms, instance types are
 * the absolute IRIs shacl-form serialized — expansion bridges the two.
 */
export function findCatalogClause(
  instance: DcsTypedClauseInstance,
  clauses: readonly ClauseCatalogType[],
  prefixes: Record<string, string>,
): ClauseCatalogType | undefined {
  const typeIris = new Set(instanceTypeIris(instance).map((iri) => expandPrefixedTerm(iri, prefixes)))
  return clauses.find((clause) => typeIris.has(expandPrefixedTerm(clause.type, prefixes)))
}
