import http from '@/api/http'
import type { AuditRequest } from '@/models/requests/auditing-request'
import type { AuditFinding, AuditReportResponse, AuditResponse } from '@/models/responses/auditing-response'
import type { AuditingService } from '@/models/services/auditing-service'
import { contractAuditEventDisplayText } from '@/utils/contract-audit-event-display'

type RawAuditTrailEntry = {
  id?: number | string
  component?: string
  event_type?: string
  eventType?: string
  event_data?: unknown
  eventData?: unknown
  did?: string
  created_at?: string
  createdAt?: string
}

type RawPACAuditResource = {
  id?: number | string
  component?: string
  event_type?: string
  eventType?: string
  did?: string
  created_at?: string
  createdAt?: string
  audit_trail?: RawAuditTrailEntry[]
  auditTrail?: RawAuditTrailEntry[]
}

const normalizeAuditResponse = (data: AuditResponse | string): AuditResponse => {
  if (!Array.isArray(data)) {
    return []
  }

  return data.flatMap((item, index) => normalizeAuditItem(item as AuditFinding & RawPACAuditResource, index))
}

function normalizeAuditItem(item: AuditFinding & RawPACAuditResource, index: number): AuditFinding[] {
  const trail = item.audit_trail ?? item.auditTrail
  if (!Array.isArray(trail)) {
    if (!isVisibleAuditEvent(item.event_type ?? item.eventType)) {
      return []
    }
    return [normalizeFinding(item, index, item.did, item.component, item.created_at ?? item.createdAt)]
  }
  if (trail.length === 0) {
    return []
  }
  return trail
    .filter((entry) => isVisibleAuditEvent(entry.event_type ?? entry.eventType))
    .map((entry, entryIndex) =>
      normalizeFinding(
        entry as AuditFinding & RawAuditTrailEntry,
        `${index}-${entry.id ?? entryIndex}`,
        entry.did ?? item.did,
        entry.component ?? item.component,
        entry.created_at ?? entry.createdAt ?? item.created_at ?? item.createdAt,
        true,
      ),
    )
}

function normalizeFinding(
  item: AuditFinding & RawAuditTrailEntry,
  fallbackId: number | string,
  fallbackDid?: string,
  fallbackComponent?: string,
  fallbackCreatedAt?: string,
  useFallbackId = false,
): AuditFinding {
  const eventType = item.event_type ?? item.eventType
  const eventData = item.event_data ?? item.eventData
  const policyData = isObjectRecord(eventData) ? eventData : null
  const severity = stringValue(policyData?.severity)
  const status = item.status ?? severity
  const category = item.category ?? categoryFromEvent(eventType, status)
  const objectDid = stringValue(policyData?.objectDid)
  return {
    id: useFallbackId ? fallbackId : item.id ?? fallbackId,
    category,
    title: item.title ?? stringValue(policyData?.title) ?? contractAuditEventDisplayText(eventType, eventData),
    description: item.description ?? descriptionFromEventData(eventData),
    component: item.component ?? fallbackComponent,
    status,
    did: item.did ?? objectDid ?? fallbackDid,
    object_name: stringValue(policyData?.objectName),
    object_type: stringValue(policyData?.objectType),
    created_at: item.created_at ?? item.createdAt ?? fallbackCreatedAt ?? new Date().toISOString(),
    details: item.details ?? item,
  }
}

function categoryFromEvent(eventType?: string, severity?: string): AuditFinding['category'] {
  const normalizedSeverity = severity?.trim().toLowerCase()
  if (normalizedSeverity === 'error' || normalizedSeverity === 'critical' || normalizedSeverity === 'failed') {
    return 'violation'
  }
  if (normalizedSeverity === 'warning' || normalizedSeverity === 'warn') {
    return 'inconsistency'
  }
  if (eventType === 'TEMPLATE_POLICY_AUDIT_FINDING') {
    return 'compliance_check'
  }
  return 'compliance_check'
}

function isVisibleAuditEvent(eventType?: string): boolean {
  const normalized = eventType?.trim().toUpperCase()
  if (!normalized) return true
  return !normalized.startsWith('RETRIEVE_') && !normalized.startsWith('SEARCH_')
}

function descriptionFromEventData(eventData: unknown): string {
  if (typeof eventData === 'string') return eventData
  if (!isObjectRecord(eventData)) return ''
  const message = stringValue(eventData.message)
  const ruleId = stringValue(eventData.ruleId)
  const semanticPath = stringValue(eventData.semanticPath)
  const requirement = stringValue(eventData.requirement)
  const objectName = stringValue(eventData.objectName)
  const objectDid = stringValue(eventData.objectDid)
  const state = stringValue(eventData.state)
  const templateType = stringValue(eventData.templateType)
  const documentNumber = stringValue(eventData.documentNumber)
  const version = typeof eventData.version === 'number' ? String(eventData.version) : stringValue(eventData.version)
  const parts = [
    objectName ? `Object: ${objectName}${objectDid ? ` (${objectDid})` : ''}` : objectDid ? `Object DID: ${objectDid}` : '',
    [templateType ? `Type: ${templateType}` : '', state ? `State: ${state}` : ''].filter(Boolean).join(' · '),
    [documentNumber ? `Document: ${documentNumber}` : '', version ? `Version: ${version}` : ''].filter(Boolean).join(' · '),
    message,
    ruleId ? `Rule: ${ruleId}` : '',
    semanticPath ? `Semantic path: ${semanticPath}` : '',
    requirement ? `Requirement: ${requirement}` : '',
  ].filter(Boolean)
  if (parts.length) return parts.join('\n')
  return JSON.stringify(eventData, null, 2)
}

function isObjectRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function stringValue(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value : undefined
}

export const auditingService: AuditingService = {
  async audit(request: AuditRequest) {
    return http.post<AuditResponse | string>('/pac/audit', request).then((res) => normalizeAuditResponse(res.data))
  },

  async report(request: AuditRequest) {
    return http.get<AuditReportResponse>('/pac/report', { params: request }).then((res) => res.data)
  },
}
