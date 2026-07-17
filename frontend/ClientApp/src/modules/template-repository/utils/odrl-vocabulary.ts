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

/**
 * The full ODRL 2.2 core action vocabulary, plus the DCS-profile action for
 * data-field value obligations. An action is recorded, not evaluated, by the
 * policy checks; any further IRI (a profile action) is still reachable through
 * the clause builder's custom-IRI escape.
 */
export const ODRL_ACTIONS: OdrlTerm[] = [
  { id: 'odrl:use', label: 'use' },
  { id: 'odrl:transfer', label: 'transfer ownership' },
  { id: 'odrl:grantUse', label: 'grant use to third parties' },
  { id: 'odrl:acceptTracking', label: 'accept tracking' },
  { id: 'odrl:aggregate', label: 'aggregate' },
  { id: 'odrl:annotate', label: 'annotate' },
  { id: 'odrl:anonymize', label: 'anonymize' },
  { id: 'odrl:archive', label: 'archive' },
  { id: 'odrl:attribute', label: 'attribute' },
  { id: 'odrl:compensate', label: 'compensate' },
  { id: 'odrl:concurrentUse', label: 'concurrent use' },
  { id: 'odrl:delete', label: 'delete' },
  { id: 'odrl:derive', label: 'derive' },
  { id: 'odrl:digitize', label: 'digitize' },
  { id: 'odrl:display', label: 'display' },
  { id: 'odrl:distribute', label: 'distribute' },
  { id: 'odrl:ensureExclusivity', label: 'ensure exclusivity' },
  { id: 'odrl:execute', label: 'execute' },
  { id: 'odrl:extract', label: 'extract' },
  { id: 'odrl:give', label: 'give' },
  { id: 'odrl:include', label: 'include' },
  { id: 'odrl:index', label: 'index' },
  { id: 'odrl:inform', label: 'inform' },
  { id: 'odrl:install', label: 'install' },
  { id: 'odrl:modify', label: 'modify' },
  { id: 'odrl:move', label: 'move' },
  { id: 'odrl:nextPolicy', label: 'next policy' },
  { id: 'odrl:obtainConsent', label: 'obtain consent' },
  { id: 'odrl:play', label: 'play' },
  { id: 'odrl:present', label: 'present' },
  { id: 'odrl:print', label: 'print' },
  { id: 'odrl:read', label: 'read' },
  { id: 'odrl:reproduce', label: 'reproduce' },
  { id: 'odrl:reviewPolicy', label: 'review policy' },
  { id: 'odrl:sell', label: 'sell' },
  { id: 'odrl:stream', label: 'stream' },
  { id: 'odrl:synchronize', label: 'synchronize' },
  { id: 'odrl:textToSpeech', label: 'text to speech' },
  { id: 'odrl:transform', label: 'transform' },
  { id: 'odrl:translate', label: 'translate' },
  { id: 'odrl:uninstall', label: 'uninstall' },
  { id: 'odrl:watermark', label: 'watermark' },
  { id: 'dcs:provideCompliantValue', label: 'provide a compliant value' },
]

/**
 * The full ODRL 2.2 core Left Operand vocabulary — the access/use context an
 * enforcer evaluates at use-time against what the target reports (as distinct
 * from the document's own data fields). The contract-time audit records that
 * such a constraint applies and defers its verdict to the enforcer; SRS
 * Appendix C uses spatial and dateTime.
 */
export const ODRL_CONTEXT_OPERANDS: OdrlTerm[] = [
  { id: 'odrl:spatial', label: 'access region (spatial)' },
  { id: 'odrl:spatialCoordinates', label: 'spatial coordinates' },
  { id: 'odrl:dateTime', label: 'access time (dateTime)' },
  { id: 'odrl:timeInterval', label: 'time interval' },
  { id: 'odrl:delayPeriod', label: 'delay period' },
  { id: 'odrl:elapsedTime', label: 'elapsed time' },
  { id: 'odrl:meteredTime', label: 'metered time' },
  { id: 'odrl:absoluteTemporalPosition', label: 'absolute temporal position' },
  { id: 'odrl:relativeTemporalPosition', label: 'relative temporal position' },
  { id: 'odrl:absolutePosition', label: 'absolute position' },
  { id: 'odrl:relativePosition', label: 'relative position' },
  { id: 'odrl:absoluteSpatialPosition', label: 'absolute spatial position' },
  { id: 'odrl:relativeSpatialPosition', label: 'relative spatial position' },
  { id: 'odrl:absoluteSize', label: 'absolute size' },
  { id: 'odrl:relativeSize', label: 'relative size' },
  { id: 'odrl:resolution', label: 'resolution' },
  { id: 'odrl:count', label: 'use count' },
  { id: 'odrl:unitOfCount', label: 'unit of count' },
  { id: 'odrl:percentage', label: 'percentage' },
  { id: 'odrl:payAmount', label: 'pay amount' },
  { id: 'odrl:purpose', label: 'purpose' },
  { id: 'odrl:event', label: 'event' },
  { id: 'odrl:recipient', label: 'recipient' },
  { id: 'odrl:industry', label: 'industry' },
  { id: 'odrl:product', label: 'product' },
  { id: 'odrl:language', label: 'language' },
  { id: 'odrl:media', label: 'media' },
  { id: 'odrl:fileFormat', label: 'file format' },
  { id: 'odrl:deliveryChannel', label: 'delivery channel' },
  { id: 'odrl:systemDevice', label: 'system device' },
  { id: 'odrl:virtualLocation', label: 'virtual location' },
  { id: 'odrl:version', label: 'version' },
]

/** The full ODRL 2.2 Constraint operator vocabulary (relational + set + type). */
export const ODRL_OPERATORS: OdrlTerm[] = [
  { id: 'odrl:eq', label: 'must equal' },
  { id: 'odrl:neq', label: 'must not equal' },
  { id: 'odrl:gt', label: 'must be greater than' },
  { id: 'odrl:gteq', label: 'must be at least' },
  { id: 'odrl:lt', label: 'must be less than' },
  { id: 'odrl:lteq', label: 'must be at most' },
  { id: 'odrl:isA', label: 'must be a (class)' },
  { id: 'odrl:isPartOf', label: 'must be part of' },
  { id: 'odrl:hasPart', label: 'must contain' },
  { id: 'odrl:isAnyOf', label: 'must be one of' },
  { id: 'odrl:isNoneOf', label: 'must not be one of' },
  { id: 'odrl:isAllOf', label: 'must be all of' },
]

/** The IRIs of the context operands, so the enforcer knows an operand is
 *  reported access context rather than a document requirement field. */
export const ODRL_CONTEXT_OPERAND_IDS: ReadonlySet<string> = new Set(ODRL_CONTEXT_OPERANDS.map((o) => o.id))
