import type { AuditRequest } from '@/models/requests/auditing-request'
import type { AuditReportResponse, AuditResponse } from '@/models/responses/auditing-response'

export interface AuditingService {
  audit: (request: AuditRequest) => Promise<AuditResponse>
  report: (request: AuditRequest) => Promise<AuditReportResponse>
}
