export interface JsonLdReference {
  '@id': string
}

export interface JsonLdTypedValue {
  '@value': string
  '@type': `xsd:${'string' | 'decimal' | 'integer' | 'boolean' | 'date' | 'dateTime'}`
}

export interface DcsTemplateMetadata {
  '@id'?: string
  '@type': 'dcs:TemplateMetadata'
  'dcs:title'?: string
  'dcs:description'?: string
  'dcs:templateType': string
  'dcs:customMetaData'?: unknown[]
}

export interface DcsContractMetadata {
  '@id'?: string
  '@type': 'dcs:ContractMetadata'
  'dcs:title'?: string
  'dcs:description'?: string
  'dcs:customMetaData'?: unknown[]
}

/** An xsd datatype a placeholder resolves to (from its SHACL sh:datatype). */
export type XsdDatatype = `xsd:${'string' | 'decimal' | 'integer' | 'boolean' | 'date' | 'dateTime'}`

/**
 * A typed, self-contained slot. The full node lives in the document's top-level
 * dcs:contractData registry, carrying its datatype straight from the SHACL
 * shape; a clause and an ODRL operand both reference it by @id. The filled value
 * rides inline on the same node (dcs:value).
 */
export interface DcsPlaceholder {
  '@id': string
  '@type': 'dcs:Placeholder'
  /** Human representation shown in prose in place of the unfilled value. */
  'dcs:label': string
  /** The input type, resolved from the shape's sh:datatype. */
  'dcs:datatype': XsdDatatype
  /** The SHACL shape the datatype and constraint were resolved from. */
  'dcs:shape'?: JsonLdReference
  'dcs:required'?: boolean
  /** The filled runtime value; absent on a template (the declaration). */
  'dcs:value'?: string | number | boolean
  /** Value constraint (options/pattern/min/max) carried inline so the slot is
   *  self-contained — render picks a select/text input without ontology lookup. */
  'dcs:valueConstraint'?: import('@/modules/template-repository/models/contract-template').SemanticValueConstraint
}

/** A clause references a placeholder by @id — a bare {"@id"} node in content. */
export type DcsPlaceholderRef = JsonLdReference

export type DcsContentSegment = string | DcsPlaceholderRef

export interface DcsSection {
  '@type': 'dcs:Section'
  '@id': string
  'dcs:title'?: string
}

export interface DcsTextBlock {
  '@type': 'dcs:TextBlock'
  '@id': string
  'dcs:text': string
}

export interface DcsSignatureField {
  '@type': 'dcs:SignatureField'
  'dcs:name': string
  'dcs:label'?: string
}

export interface DcsClause {
  '@type': 'dcs:Clause'
  '@id': string
  'dcs:content': { '@list': DcsContentSegment[] } | string
  'dcs:title'?: string
  'dcs:signatureFields'?: DcsSignatureField[]
}

export type DcsBlock = DcsSection | DcsTextBlock | DcsClause

export interface DcsLayoutNode {
  '@id': string
  '@type'?: 'dcs:LayoutNode'
  'dcs:isRoot'?: boolean
  'dcs:children': { '@list': JsonLdReference[] }
}

export interface DcsDocumentStructure {
  '@id'?: string
  '@type': 'dcs:DocumentStructure'
  'dcs:blocks': { '@list': DcsBlock[] }
  'dcs:layout': { '@list': DcsLayoutNode[] }
}

export interface DcsContractField {
  '@id': string
  '@type': 'dcs:ContractField'
  'dcs:dataType': JsonLdReference
  'dcs:sourceObject': JsonLdReference
  'dcs:path': string
  'dcs:domainField'?: JsonLdReference
}

export interface OdrlConstraint {
  '@type': 'odrl:Constraint'
  'odrl:leftOperand': JsonLdReference
  'odrl:operator': JsonLdReference
  /**
   * The boundary the left operand is checked against: a fixed literal (or list
   * for set operators), or a reference to a RequirementField whose value is
   * agreed during contract negotiation. SRS Appendix C is a template whose
   * spatial and dateTime boundaries (the permitted region, the access deadline)
   * are negotiated field references, resolved to their filled values at
   * enforcement.
   */
  'odrl:rightOperand'?: JsonLdTypedValue | JsonLdTypedValue[] | JsonLdReference
}

/**
 * An ODRL LogicalConstraint (IM §2.6): a logical operator over an ordered list
 * of constraints. and/andSequence = all hold, or = any holds, xone = exactly
 * one holds; children may themselves be logical (a tree).
 */
export interface OdrlLogicalConstraint {
  '@type': 'odrl:LogicalConstraint'
  'odrl:and'?: { '@list': OdrlConstraintNode[] }
  'odrl:or'?: { '@list': OdrlConstraintNode[] }
  'odrl:xone'?: { '@list': OdrlConstraintNode[] }
  'odrl:andSequence'?: { '@list': OdrlConstraintNode[] }
}

export type OdrlConstraintNode = OdrlConstraint | OdrlLogicalConstraint

export function isAtomicConstraint(node: OdrlConstraintNode): node is OdrlConstraint {
  return node['@type'] === 'odrl:Constraint'
}

/**
 * A Duty nested under a Permission (ODRL IM §2.5): an obligation the assignee
 * must fulfil to exercise the permission. A duty is a *fragment* — it carries
 * its own action and constraints, while the assigner/assignee/target are
 * inherited from the enclosing rule (so, unlike a top-level rule, it declares
 * none of them). A duty may carry a consequence: a further duty that becomes
 * active when the duty itself is not fulfilled.
 */
export interface OdrlDuty {
  '@id'?: string
  '@type': 'odrl:Duty'
  'odrl:action': JsonLdReference | JsonLdReference[]
  'odrl:constraint'?: OdrlConstraintNode[]
  'odrl:consequence'?: OdrlDuty[]
}

export interface OdrlRule {
  '@id': string
  '@type': 'odrl:Duty' | 'odrl:Permission' | 'odrl:Prohibition'
  /**
   * The action(s) the rule governs. A single action is one reference; several
   * actions are an array (ODRL Policy Rule Composition §2.7 — normatively the
   * atomic equivalent is one rule per action).
   */
  'odrl:action': JsonLdReference | JsonLdReference[]
  /** Bound party DIDs for a contract instance (ODRL Agreement); open/placeholder party references for a template (ODRL Offer). */
  'odrl:assigner': JsonLdReference
  'odrl:assignee': JsonLdReference
  /** The contract/data-asset IRI this rule applies to. */
  'odrl:target': JsonLdReference
  /** The human-readable clause node this rule is backed by (required — machine rules operationalize audited prose). */
  'dcs:prose': JsonLdReference
  /** The rule's constraints. A plain list is a conjunction (all hold, ODRL IM
   *  §2.5); a single LogicalConstraint expresses or/xone/andSequence. Nodes may
   *  nest (a constraint tree). */
  'odrl:constraint'?: OdrlConstraintNode[]
  /** Duties the assignee must fulfil to exercise this rule (ODRL IM §2.5 —
   *  meaningful on a Permission). Each is a fragment with its own action and
   *  constraints. */
  'odrl:duty'?: OdrlDuty[]
}

/** The single enclosing ODRL 2.2 policy for a template (Offer) or contract (Agreement). */
export interface OdrlSet {
  '@id': string
  '@type': 'odrl:Offer' | 'odrl:Agreement'
  'odrl:profile': JsonLdReference
  /** Policy-level Duty rules (ODRL 2.2: a Policy carries obligation, never duty — duty nests under a Permission). */
  'odrl:obligation'?: OdrlRule[]
  'odrl:permission'?: OdrlRule[]
  'odrl:prohibition'?: OdrlRule[]
}

export interface DcsDocumentData {
  /** Anchored server-side to the Semantic Hub's versioned context URL; the client never emits it. */
  '@context'?: unknown
  '@type': 'dcs:ContractTemplate' | 'dcs:Contract'
  '@id'?: string
  'dcs:metadata': DcsTemplateMetadata | DcsContractMetadata
  'dcs:documentStructure': DcsDocumentStructure
  /** Flat, self-contained registry of the document's typed placeholder nodes. */
  'dcs:contractData': DcsPlaceholder[]
  'dcs:policies': OdrlSet
}

export interface DcsTemplateData extends DcsDocumentData {
  '@type': 'dcs:ContractTemplate'
  'dcs:metadata': DcsTemplateMetadata
}

export interface DcsContractData extends DcsDocumentData {
  '@type': 'dcs:Contract'
  'dcs:metadata': DcsContractMetadata | DcsTemplateMetadata
  'dcs:contractFields'?: DcsContractField[]
  'dcs:parentContract'?: JsonLdReference
  derivedFromTemplate?: DcsTemplateProvenance
}

/** The source-template node: a prov:wasDerivedFrom edge plus version assertion. */
export interface DcsTemplateProvenance {
  '@id': string
  version?: number
}

export function isDcsSection(block: DcsBlock): block is DcsSection {
  return block['@type'] === 'dcs:Section'
}

export function isDcsTextBlock(block: DcsBlock): block is DcsTextBlock {
  return block['@type'] === 'dcs:TextBlock'
}

export function isDcsClause(block: DcsBlock): block is DcsClause {
  return block['@type'] === 'dcs:Clause'
}

export function isDcsPlaceholder(seg: DcsContentSegment): seg is DcsPlaceholderRef {
  return typeof seg !== 'string'
}

export function isDcsDocumentData(raw: unknown): raw is DcsDocumentData {
  if (typeof raw !== 'object' || raw === null) return false
  const value = raw as Record<string, unknown>
  const policies = value['dcs:policies']
  return (
    (value['@type'] === 'dcs:ContractTemplate' || value['@type'] === 'dcs:Contract') &&
    typeof value['dcs:documentStructure'] === 'object' &&
    Array.isArray(value['dcs:contractData']) &&
    // Canonical shape: a single enclosing odrl:Set object.
    // An empty array is still accepted as "no policies yet" (brand-new
    // documents); a non-empty bare-rule array is not.
    (isOdrlSet(policies) || (Array.isArray(policies) && policies.length === 0))
  )
}

function isOdrlSet(value: unknown): value is OdrlSet {
  if (typeof value !== 'object' || value === null) return false
  const type = (value as Record<string, unknown>)['@type']
  return type === 'odrl:Offer' || type === 'odrl:Agreement'
}

export function isDcsTemplateData(raw: unknown): raw is DcsTemplateData {
  return isDcsDocumentData(raw) && raw['@type'] === 'dcs:ContractTemplate'
}

export function isDcsContractData(raw: unknown): raw is DcsContractData {
  return isDcsDocumentData(raw) && raw['@type'] === 'dcs:Contract'
}
