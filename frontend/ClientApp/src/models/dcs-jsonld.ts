export const DCS_JSONLD_CONTEXT = {
  dcs: 'https://w3id.org/facis/dcs#',
  odrl: 'http://www.w3.org/ns/odrl/2/',
} as const

// ---- ODRL ----

export interface OdrlConstraint {
  '@type'?: 'odrl:Constraint'
  'odrl:leftOperand': { '@id': string }
  'odrl:operator': { '@id': string }
  'odrl:rightOperand'?: unknown
}

export interface OdrlRule {
  '@type': 'odrl:Duty' | 'odrl:Permission' | 'odrl:Prohibition'
  '@id'?: string
  'odrl:action': { '@id': string }
  'odrl:constraint'?: OdrlConstraint[]
  // Condition metadata (on duties that represent semantic conditions)
  'dcs:conditionName'?: string
  'dcs:schemaVersion'?: string
  'dcs:entityType'?: string
  'dcs:entityRole'?: string
}

export type OdrlDuty = OdrlRule & { '@type': 'odrl:Duty' }
export type OdrlPermission = OdrlRule & { '@type': 'odrl:Permission' }
export type OdrlProhibition = OdrlRule & { '@type': 'odrl:Prohibition' }

export interface OdrlSet {
  '@type': 'odrl:Set'
  '@id'?: string
  'odrl:obligation'?: OdrlDuty[]
  'odrl:permission'?: OdrlPermission[]
  'odrl:prohibition'?: OdrlProhibition[]
}

// ---- DCS Blocks ----

export interface DcsSection {
  '@type': 'dcs:Section'
  '@id': string
  'dcs:title'?: string
}

export interface DcsTextBlock {
  '@type': 'dcs:TextBlock'
  '@id': string
  'dcs:content': string
}

export interface DcsParameterRef {
  '@type': 'dcs:ParameterRef'
  'dcs:constraint': { '@id': string }
  'odrl:leftOperand': { '@id': string }
}

export type DcsContentSegment = string | DcsParameterRef

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

// ---- Layout (document hierarchy) ----

export interface DcsLayoutNode {
  '@id': string
  'dcs:isRoot'?: boolean
  'dcs:children': { '@list': { '@id': string }[] }
}

// ---- Template document ----

export interface DcsTemplateData {
  '@context': typeof DCS_JSONLD_CONTEXT
  '@type': 'dcs:ContractTemplate'
  '@id'?: string
  'dcs:title': string
  'dcs:templateType': string
  'dcs:blocks': DcsBlock[]
  'dcs:layout': DcsLayoutNode[]
  'odrl:policy'?: OdrlSet
  'dcs:customMetaData'?: unknown[]
  'dcs:subTemplateSnapshots'?: unknown[]
}

export function isDcsTemplateData(raw: unknown): raw is DcsTemplateData {
  return (
    typeof raw === 'object' &&
    raw !== null &&
    '@type' in raw &&
    (raw as Record<string, unknown>)['@type'] === 'dcs:ContractTemplate'
  )
}
