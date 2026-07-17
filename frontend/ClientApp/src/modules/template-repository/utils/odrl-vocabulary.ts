/**
 * The ODRL 2.2 core vocabulary (https://www.w3.org/TR/odrl-vocab/) the clause
 * rule builder authors against — the single source for the actions, left
 * operands, and operators offered in the UI, so what a template producer can
 * express is exactly standard ODRL. Terms live in the ODRL namespace; a rule's
 * constraints combine as conjunction (all MUST hold, ODRL Information Model
 * §2.5), and the DCS ODRL profile adds provideCompliantValue for data-field
 * obligations.
 */

export interface OdrlTerm {
  id: string
  label: string
}

/** Rule deontic types (ODRL IM: Permission enables, Prohibition disables, Duty obliges). */
export const ODRL_RULE_TYPES: { type: 'odrl:Permission' | 'odrl:Prohibition' | 'odrl:Duty'; label: string }[] = [
  { type: 'odrl:Permission', label: 'Permission — the assignee MAY' },
  { type: 'odrl:Prohibition', label: 'Prohibition — the assignee MUST NOT' },
  { type: 'odrl:Duty', label: 'Obligation — the assignee MUST' },
]

/** ODRL actions plus the DCS-profile action for data-field value obligations. */
export const ODRL_ACTIONS: OdrlTerm[] = [
  { id: 'odrl:use', label: 'use' },
  { id: 'odrl:grantUse', label: 'grant use to third parties' },
  { id: 'odrl:transfer', label: 'transfer ownership' },
  { id: 'odrl:distribute', label: 'distribute' },
  { id: 'odrl:reproduce', label: 'reproduce' },
  { id: 'odrl:display', label: 'display' },
  { id: 'odrl:play', label: 'play' },
  { id: 'odrl:read', label: 'read' },
  { id: 'odrl:execute', label: 'execute' },
  { id: 'odrl:modify', label: 'modify' },
  { id: 'odrl:derive', label: 'derive' },
  { id: 'odrl:delete', label: 'delete' },
  { id: 'dcs:provideCompliantValue', label: 'provide a compliant value' },
]

/**
 * ODRL context left operands — the access/use context an enforcer evaluates at
 * use-time against what the target reports (as distinct from the document's own
 * data fields). Enough to express SRS Appendix C (spatial, dateTime).
 */
export const ODRL_CONTEXT_OPERANDS: OdrlTerm[] = [
  { id: 'odrl:spatial', label: 'access region (spatial)' },
  { id: 'odrl:dateTime', label: 'access time (dateTime)' },
  { id: 'odrl:purpose', label: 'purpose' },
  { id: 'odrl:count', label: 'use count' },
  { id: 'odrl:recipient', label: 'recipient' },
  { id: 'odrl:industry', label: 'industry' },
  { id: 'odrl:event', label: 'event' },
]

/** ODRL relational + set operators. */
export const ODRL_OPERATORS: OdrlTerm[] = [
  { id: 'odrl:eq', label: 'must equal' },
  { id: 'odrl:neq', label: 'must not equal' },
  { id: 'odrl:gt', label: 'must be greater than' },
  { id: 'odrl:gteq', label: 'must be at least' },
  { id: 'odrl:lt', label: 'must be less than' },
  { id: 'odrl:lteq', label: 'must be at most' },
  { id: 'odrl:isPartOf', label: 'must be part of' },
  { id: 'odrl:hasPart', label: 'must contain' },
  { id: 'odrl:isAnyOf', label: 'must be one of' },
  { id: 'odrl:isNoneOf', label: 'must not be one of' },
  { id: 'odrl:isAllOf', label: 'must be all of' },
]

/** The IRIs of the context operands, so the enforcer knows an operand is
 *  reported access context rather than a document requirement field. */
export const ODRL_CONTEXT_OPERAND_IDS: ReadonlySet<string> = new Set(ODRL_CONTEXT_OPERANDS.map((o) => o.id))
