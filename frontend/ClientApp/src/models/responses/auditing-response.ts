export type AuditFindingCategory = 'violation' | 'inconsistency' | 'compliance_check'

export interface AuditFinding {
  id: number | string
  category: AuditFindingCategory | (string & {})
  title?: string
  description?: string
  component?: string
  status?: string
  did?: string
  object_name?: string
  object_type?: string
  created_at: string
  details?: unknown
}

export type AuditResponse = AuditFinding[]
export type AuditReportResponse = unknown
