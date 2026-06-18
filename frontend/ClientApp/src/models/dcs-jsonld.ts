export const DCS_JSONLD_CONTEXT = {
  dcs: 'https://w3id.org/facis/dcs/ontology/v1#',
  odrl: 'http://www.w3.org/ns/odrl/2/',
} as const

export interface JsonLdReference {
  '@id': string
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

export interface DcsPlaceholder {
  '@type': 'dcs:Placeholder'
  'dcs:token': string
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
  'dcs:isRoot'?: boolean
  'dcs:children': { '@list': JsonLdReference[] }
}

export interface DcsDocumentStructure {
  '@id'?: string
  '@type': 'dcs:DocumentStructure'
  'dcs:blocks': DcsBlock[]
  'dcs:layout': DcsLayoutNode[]
}

export interface DcsRequirementField {
  '@id': string
  '@type': 'dcs:RequirementField'
  'dcs:parameterName': string
  'dcs:domainField': JsonLdReference
  'dcs:semanticPath': string
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

export interface OdrlConstraint {
  '@type': 'odrl:Constraint'
  'odrl:leftOperand': JsonLdReference
  'odrl:operator': JsonLdReference
  'odrl:rightOperand'?: unknown
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

export interface DcsTemplateData {
  '@context': typeof DCS_JSONLD_CONTEXT
  '@type': 'dcs:ContractTemplate'
  '@id'?: string
  'dcs:metadata': DcsTemplateMetadata
  'dcs:documentStructure': DcsDocumentStructure
  'dcs:contractData': DcsDataRequirement[]
  'dcs:policies': OdrlRule[]
}

export function isDcsTemplateData(raw: unknown): raw is DcsTemplateData {
  if (typeof raw !== 'object' || raw === null) return false
  const value = raw as Record<string, unknown>
  return (
    value['@type'] === 'dcs:ContractTemplate' &&
    typeof value['dcs:documentStructure'] === 'object' &&
    Array.isArray(value['dcs:contractData']) &&
    Array.isArray(value['dcs:policies'])
  )
}
