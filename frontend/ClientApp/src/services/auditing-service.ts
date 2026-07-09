import http from '@/api/http'
import { contractAuditEventDisplayText } from '@/utils/contract-audit-event-display'
import type { AuditReportRequest, AuditRequest, AuditScope } from '@/models/requests/auditing-request'
import type { AuditFinding, AuditReportResponse, AuditResponse } from '@/models/responses/auditing-response'
import type { AuditingService } from '@/models/services/auditing-service'

interface RawAuditTrailEntry {
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

interface RawPACAuditResource {
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

const normalizeAuditResponse = (data: AuditResponse | string, scope: AuditScope): AuditResponse => {
  if (!Array.isArray(data)) {
    return []
  }

  return data.flatMap((item, index) => normalizeAuditItem(item as AuditFinding & RawPACAuditResource, index, scope))
}

function normalizeAuditItem(
  item: AuditFinding & RawPACAuditResource,
  index: number,
  scope: AuditScope,
): AuditFinding[] {
  const trail = item.audit_trail ?? item.auditTrail
  if (!Array.isArray(trail)) {
    if (!hasAuditPayload(item)) {
      return []
    }
    if (!isVisibleAuditEvent(item.event_type ?? item.eventType)) {
      return []
    }
    return [normalizeFinding(item, index, scope, item.did, item.created_at ?? item.createdAt)]
  }
  if (trail.length === 0) {
    return []
  }
  return trail
    .filter(hasAuditPayload)
    .filter((entry) => isVisibleAuditEvent(entry.event_type ?? entry.eventType))
    .map((entry, entryIndex) =>
      normalizeFinding(
        entry as AuditFinding & RawAuditTrailEntry,
        `${index}-${entry.id ?? entryIndex}`,
        scope,
        entry.did ?? item.did,
        entry.created_at ?? entry.createdAt ?? item.created_at ?? item.createdAt,
        true,
      ),
    )
}

function normalizeFinding(
  item: AuditFinding & RawAuditTrailEntry,
  fallbackId: number | string,
  scope: AuditScope,
  fallbackDid?: string,
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
    id: useFallbackId ? fallbackId : (item.id ?? fallbackId),
    category,
    title: item.title ?? stringValue(policyData?.title) ?? contractAuditEventDisplayText(eventType, eventData),
    description: item.description ?? descriptionFromEventData(eventData),
    component: item.component ?? auditComponentLabel(scope),
    status,
    did: item.did ?? objectDid ?? fallbackDid,
    object_name: stringValue(policyData?.objectName),
    object_type: stringValue(policyData?.objectType),
    created_at: item.created_at ?? item.createdAt ?? fallbackCreatedAt ?? new Date().toISOString(),
    details: item.details ?? item,
  }
}

type RawAuditPayload = RawAuditTrailEntry & Pick<Partial<AuditFinding>, 'description' | 'status' | 'title'>

function hasAuditPayload(item: RawAuditPayload): boolean {
  return (
    Boolean(stringValue(item.event_type ?? item.eventType)) ||
    item.event_data != null ||
    item.eventData != null ||
    Boolean(stringValue(item.title)) ||
    Boolean(stringValue(item.description)) ||
    Boolean(stringValue(item.status))
  )
}

function categoryFromEvent(eventType?: string, severity?: string): AuditFinding['category'] {
  const normalizedSeverity = severity?.trim().toLowerCase()
  if (
    normalizedSeverity === 'error' ||
    normalizedSeverity === 'critical' ||
    normalizedSeverity === 'blocking' ||
    normalizedSeverity === 'failed'
  ) {
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
  const actualValue = detailValue(eventData.actualValue)
  const expectedValue = detailValue(eventData.expectedValue)
  const expectedValues = detailValue(eventData.expectedValues)
  const operator = stringValue(eventData.operator)
  const objectName = stringValue(eventData.objectName)
  const objectDid = stringValue(eventData.objectDid)
  const state = stringValue(eventData.state)
  const templateType = stringValue(eventData.templateType)
  const documentNumber = stringValue(eventData.documentNumber)
  const version = typeof eventData.version === 'number' ? String(eventData.version) : stringValue(eventData.version)
  const parts = [
    objectName
      ? `Object: ${objectName}${objectDid ? ` (${objectDid})` : ''}`
      : objectDid
        ? `Object DID: ${objectDid}`
        : '',
    [templateType ? `Type: ${templateType}` : '', state ? `State: ${state}` : ''].filter(Boolean).join(' · '),
    [documentNumber ? `Document: ${documentNumber}` : '', version ? `Version: ${version}` : '']
      .filter(Boolean)
      .join(' · '),
    message,
    requirement ? `Requirement: ${requirement}` : '',
    actualValue ? `Actual value: ${actualValue}` : '',
    expectedValue ? `Expected value: ${expectedValue}` : '',
    expectedValues ? `Expected values: ${expectedValues}` : '',
    operator ? `Operator: ${operator}` : '',
    ruleId ? `Rule: ${ruleId}` : '',
    semanticPath ? `Semantic path: ${semanticPath}` : '',
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

function detailValue(value: unknown): string | undefined {
  if (typeof value === 'string') return value.trim() ? value : undefined
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (Array.isArray(value)) {
    const values = value.map((item) => detailValue(item)).filter(Boolean)
    return values.length ? values.join(', ') : undefined
  }
  if (isObjectRecord(value)) return JSON.stringify(value)
  return undefined
}

function auditComponentLabel(scope: AuditScope): string {
  switch (scope) {
    case 'templates':
      return 'Templates'
    case 'contracts':
      return 'Contracts'
    case 'archive':
      return 'Archive'
    case 'signatures':
      return 'Signatures'
  }
}

export const auditingService: AuditingService = {
  async audit(request: AuditRequest) {
    return http
      .post<AuditResponse | string>('/pac/audit', request)
      .then((res) => normalizeAuditResponse(res.data, request.scope))
  },

  async report(request: AuditReportRequest) {
    return http.get<AuditReportResponse>('/pac/report', { params: request }).then((res) => res.data)
  },
}
