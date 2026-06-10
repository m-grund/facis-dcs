export type AuditScope = 'templates' | 'contracts' | 'signatures' | 'archive'
export type AuditMode = 'repository_trail' | 'static_contract'

export interface AuditRequest {
  scope: AuditScope
  audit_mode?: AuditMode
  contract_document?: unknown
  policy?: unknown
  contract_did?: string
  contract_version?: string
  policy_version?: string
}
