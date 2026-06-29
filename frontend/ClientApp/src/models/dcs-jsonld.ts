export const DCS_JSONLD_CONTEXT = {
  dcs: 'https://w3id.org/facis/dcs/ontology/v1#',
  odrl: 'http://www.w3.org/ns/odrl/2/',
  xsd: 'http://www.w3.org/2001/XMLSchema#',
} as const

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

export interface DcsClause {
  '@type': 'dcs:Clause'
  '@id': string
  'dcs:content': { '@list': DcsContentSegment[] } | string
  'dcs:title'?: string
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
  'dcs:domainField': JsonLdReference
  'dcs:required': boolean
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
  'odrl:constraint'?: OdrlConstraint
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
  '@context': typeof DCS_JSONLD_CONTEXT
  '@type': 'dcs:ContractTemplate' | 'dcs:Contract'
  '@id'?: string
  'dcs:metadata': DcsTemplateMetadata | DcsContractMetadata
  'dcs:documentStructure': DcsDocumentStructure
  'dcs:contractData': DcsDataRequirement[]
  'dcs:policies': OdrlRule[]
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
  semanticConditionValues?: {
    blockId: string
    conditionId: string
    parameterName: string
    parameterValue?: string | number | boolean
  }[]
  sourceTemplate?: {
    did: string
    version?: number
    document_number?: string
  }
  derivedFromTemplate?: string
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
  return (
    (value['@type'] === 'dcs:ContractTemplate' || value['@type'] === 'dcs:Contract') &&
    typeof value['dcs:documentStructure'] === 'object' &&
    Array.isArray(value['dcs:contractData']) &&
    Array.isArray(value['dcs:policies'])
  )
}

export function isDcsTemplateData(raw: unknown): raw is DcsTemplateData {
  return isDcsDocumentData(raw) && raw['@type'] === 'dcs:ContractTemplate'
}

export function isDcsContractData(raw: unknown): raw is DcsContractData {
  return isDcsDocumentData(raw) && raw['@type'] === 'dcs:Contract'
}
