import http from '@/api/http'
import type { AuditRequest } from '@/models/requests/auditing-request'
import type { AuditFinding, AuditReportResponse, AuditResponse } from '@/models/responses/auditing-response'
import type { AuditingService } from '@/models/services/auditing-service'

const normalizeAuditResponse = (data: AuditResponse | string): AuditResponse => {
  if (!Array.isArray(data)) {
    return []
  }

  return data.map((item: AuditFinding & { event_type?: string; event_data?: string }, index) => ({
    id: item.id ?? index,
    category: item.category ?? item.event_type ?? 'compliance_check',
    title: item.title ?? item.event_type ?? 'Audit finding',
    description: item.description ?? item.event_data ?? '',
    component: item.component,
    status: item.status,
    did: item.did,
    created_at: item.created_at,
    details: item.details ?? item,
  }))
}

export const auditingService: AuditingService = {
  async audit(request: AuditRequest) {
    return http.post<AuditResponse | string>('/pac/audit', request).then((res) => normalizeAuditResponse(res.data))
  },

  async report(request: AuditRequest) {
    return http.get<AuditReportResponse>('/pac/report', { params: request }).then((res) => res.data)
  },
}
