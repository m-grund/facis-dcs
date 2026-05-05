export type AuditScope = 'templates' | 'contracts' | 'signatures' | 'archive'

export interface AuditRequest {
  scope: AuditScope
}
