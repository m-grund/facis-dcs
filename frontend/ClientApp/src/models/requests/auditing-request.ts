export type AuditScope = 'templates' | 'contracts' | 'signatures' | 'archive'
export type AuditReportFormat = 'json' | 'csv' | 'pdf'

export interface AuditRequest {
  scope: AuditScope
}

export interface AuditReportRequest extends AuditRequest {
  format?: AuditReportFormat
  did?: string
}
