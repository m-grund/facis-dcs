export interface JsonLdReference {
  '@id': string
}

export interface JsonLdTypedValue {
  '@value': string
  '@type': `xsd:${'string' | 'decimal' | 'integer' | 'boolean' | 'date'}`
}

export interface DcsTemplateMetadata {
  '@id'?: string
  '@type': 'dcs:TemplateMetadata'
  'dcs:title'?: string
  'dcs:description'?: string
  'dcs:templateType': string
  'dcs:customMetaData'?: unknown[]
  'dcs:subTemplates'?: DcsSubTemplateSnapshot[]
}

export interface DcsContractMetadata {
  '@id'?: string
  '@type': 'dcs:ContractMetadata'
  'dcs:title'?: string
  'dcs:description'?: string
  'dcs:customMetaData'?: unknown[]
  'dcs:subTemplates'?: DcsSubTemplateSnapshot[]
}

export interface DcsPlaceholder {
  '@type': 'dcs:Placeholder'
  'dcs:bindsTo': JsonLdReference
}

export type DcsContentSegment = string | DcsPlaceholder

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
  /**
   * An optional typed clause instance nested inside a free-text dcs:Clause
   * block. The nested object carries its own @type (e.g. "dcs:PaymentClause",
   * from the Semantic Hub clause catalog, GET /semantic/clauses) and becomes
   * its own JSON-LD/RDF node — server-side SHACL validation targets it by
   * that @type regardless of nesting, so the palette-generated form and
   * enforcement share one source of truth.
   */
  'dcs:typedClause'?: DcsTypedClauseInstance
}

export interface DcsTypedClauseInstance {
  '@type': string
  [property: string]: unknown
}

export interface DcsApprovedTemplate {
  '@type': 'dcs:ApprovedTemplate'
  '@id': string
  'dcs:templateDid': string
  'dcs:version': number
  'dcs:documentNumber'?: string
}

export type DcsBlock = DcsSection | DcsTextBlock | DcsClause | DcsApprovedTemplate

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
  'dcs:layout': DcsLayoutNode[]
}

export interface DcsRequirementField {
  '@id': string
  '@type': 'dcs:RequirementField'
  'dcs:parameterName': string
  /** Optional: the served RequirementField shape requires only dcs:parameterName. */
  'dcs:domainField'?: JsonLdReference
  'dcs:valueType'?: string
  'dcs:required': boolean
  /**
   * The submitted runtime value, carried inline on the field an ODRL
   * constraint names as its odrl:leftOperand. Absent on a template (the
   * declaration), filled at contract time.
   */
  'dcs:parameterValue'?: string | number | boolean
  /** The document block a placeholder bound to this field renders into. */
  'dcs:blockId'?: string
}

export interface DcsDataRequirement {
  '@id': string
  '@type': 'dcs:DataRequirement'
  'dcs:conditionId': string
  'dcs:name': string
  'dcs:schemaVersion': 'v1'
  'dcs:entityType'?: string
  'dcs:entityRole'?: string
  'dcs:fields': DcsRequirementField[]
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
  'odrl:rightOperand'?: JsonLdTypedValue | JsonLdTypedValue[]
}

export interface OdrlRule {
  '@id': string
  '@type': 'odrl:Duty' | 'odrl:Permission' | 'odrl:Prohibition'
  /** Every rule declares exactly one action (DCS ODRL profile). */
  'odrl:action': JsonLdReference
  /** Bound party DIDs for a contract instance (ODRL Agreement); open/placeholder party references for a template (ODRL Offer). */
  'odrl:assigner': JsonLdReference
  'odrl:assignee': JsonLdReference
  /** The contract/data-asset IRI this rule applies to. */
  'odrl:target': JsonLdReference
  /** The human-readable clause node this rule is backed by (required — machine rules operationalize audited prose). */
  'dcs:prose': JsonLdReference
  'odrl:constraint'?: OdrlConstraint
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

export interface DcsSubTemplateSnapshot {
  '@id': string
  'dcs:version': number
  'dcs:documentNumber'?: string
  'dcs:name'?: string
  'dcs:description'?: string
  'dcs:template': DcsTemplateData
}

export interface DcsDocumentData {
  /** Anchored server-side to the Semantic Hub's versioned context URL; the client never emits it. */
  '@context'?: unknown
  '@type': 'dcs:ContractTemplate' | 'dcs:Contract'
  '@id'?: string
  'dcs:metadata': DcsTemplateMetadata | DcsContractMetadata
  'dcs:documentStructure': DcsDocumentStructure
  'dcs:contractData': DcsDataRequirement[]
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

export function isDcsApprovedTemplate(block: DcsBlock): block is DcsApprovedTemplate {
  return block['@type'] === 'dcs:ApprovedTemplate'
}

export function isDcsPlaceholder(seg: DcsContentSegment): seg is DcsPlaceholder {
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
