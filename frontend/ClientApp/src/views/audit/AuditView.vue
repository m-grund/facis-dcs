<script setup lang="ts">
import type { AuditMode, AuditScope } from '@/models/requests/auditing-request'
import type { AuditFinding } from '@/models/responses/auditing-response'
import { auditingService } from '@/services/auditing-service'
import { computed, ref, watch } from 'vue'

const findings = ref<AuditFinding[]>([])
const selectedFindingId = ref<number | string | null>(null)
const report = ref<unknown>(null)
const auditLoading = ref(false)
const reportLoading = ref(false)
const error = ref<string | null>(null)
const selectedScope = ref<AuditScope>('contracts')
const selectedAuditMode = ref<AuditMode>('repository_trail')
const hasExecutedAudit = ref(false)
type AuditResult = 'passed' | 'failed' | 'review'
type AuditTab = 'checks' | 'timeline'
type TableFilterKey = 'result' | 'category' | 'status' | 'component' | 'did'

const emptyValueLabel = '-'
const activeAuditTab = ref<AuditTab>('checks')
const tableFilters = ref<Record<TableFilterKey, Record<string, boolean>>>({
  result: {},
  category: {},
  status: {},
  component: {},
  did: {},
})
const contractDid = ref('urn:facis:dcs:contract:sla:example-001')
const contractVersion = ref('v1')
const policyVersion = ref('2026-05-18')
const contractDocumentText = ref(`{
  "@context": [],
  "@id": "facis-contract-with-demo-errors",
  "@type": [
    "sla:ServiceLevelAgreement"
  ],
  "parties": [
    {
      "@id": "urn:facis:party:supplier-001",
      "@type": "dcs:Organization",
      "role": "supplier",
      "legalName": "Example Supplier GmbH",
      "location": {
        "country": "RUS"
      }
    },
    {
      "@id": "urn:facis:party:customer-001",
      "@type": "dcs:CompanyParty",
      "role": "customer",
      "legalName": "Example Customer AG",
      "location": {
        "country": "DEU"
      }
    }
  ],
  "contract": {
    "type": "serviceAgreement",
    "governingLaw": "DE"
  },
  "service": {
    "sla": {
      "availability": 99.72,
      "responseTime": 20,
      "resolutionTime": 180,
      "supportHours": "24x7"
    }
  },
  "signature": {
    "requiredLevel": "AES"
  }
}`)

const scopeOptions: { value: AuditScope; label: string }[] = [
  { value: 'templates', label: 'Templates' },
  { value: 'contracts', label: 'Contracts' },
  { value: 'archive', label: 'Archive' },
]

const auditModeOptions: { value: AuditMode; label: string }[] = [
  { value: 'repository_trail', label: 'Repository Trail' },
  { value: 'static_contract', label: 'JSON-LD / SHACL Contract' },
]

watch(selectedAuditMode, (mode) => {
  if (mode === 'static_contract') {
    selectedScope.value = 'contracts'
  }
})

const filteredFindings = computed(() => {
  return checkFindings.value.filter((finding) => {
    return tableFilterEnabled('result', auditResultLabel(finding))
      && tableFilterEnabled('category', finding.category)
      && tableFilterEnabled('status', finding.status)
      && tableFilterEnabled('component', finding.component)
      && tableFilterEnabled('did', finding.did)
  })
})
const selectedFinding = computed(() => {
  return findings.value.find((finding) => String(finding.id) === String(selectedFindingId.value)) ?? null
})
const selectedFindingKind = computed(() => selectedFinding.value ? auditItemKind(selectedFinding.value) : null)
const selectedFindingEventData = computed(() => {
  if (!selectedFinding.value) {
    return null
  }
  if (isObjectRecord(selectedFinding.value.details)) {
    const eventData = selectedFinding.value.details.event_data ?? selectedFinding.value.details.eventData
    return isObjectRecord(eventData) ? eventData : null
  }
  return null
})
const selectedFindingDetailRows = computed(() => {
  const finding = selectedFinding.value
  const eventData = selectedFindingEventData.value
  if (!finding) {
    return []
  }
  if (selectedFindingKind.value === 'event') {
    return [
      { label: 'Event Type', value: stringDetail(rawEventType(finding)) },
      { label: 'Actor', value: stringDetail(actorFromEventData(eventData)) },
      { label: 'Timestamp', value: stringDetail(formatDateTime(finding.created_at)) },
      { label: 'Component', value: stringDetail(finding.component) },
      { label: 'DID', value: stringDetail(finding.did) },
    ].filter((row) => row.value !== emptyValueLabel)
  }
  return [
    { label: 'Checked', value: stringDetail(checkAssertion(finding)) },
    { label: 'Rule ID', value: stringDetail(eventData?.ruleId) },
    { label: 'Policy Set', value: stringDetail(eventData?.policySetId) },
    { label: 'Policy Version', value: stringDetail(eventData?.policyVersion) },
    { label: 'Semantic Path', value: stringDetail(eventData?.semanticPath) },
    { label: 'Path', value: stringDetail(eventData?.path) },
    { label: 'Ontology Term', value: stringDetail(eventData?.ontologyTerm) },
    { label: 'Object Type', value: stringDetail(eventData?.objectType ?? finding.object_type) },
    { label: 'Contract Version', value: stringDetail(eventData?.contractVersion) },
    { label: 'Audited By', value: stringDetail(eventData?.auditedBy) },
    { label: 'Component', value: stringDetail(finding.component) },
    { label: 'DID', value: stringDetail(finding.did) },
  ].filter((row) => row.value !== emptyValueLabel)
})

const checkFindings = computed(() => findings.value.filter((finding) => auditItemKind(finding) === 'check'))
const timelineEvents = computed(() => {
  return [...findings.value]
    .filter((finding) => auditItemKind(finding) === 'event')
    .sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at))
})
const failedCheckCount = computed(() => checkFindings.value.filter((finding) => auditResult(finding) === 'failed').length)
const passedCheckCount = computed(() => checkFindings.value.filter((finding) => auditResult(finding) === 'passed').length)
const reviewCheckCount = computed(() => checkFindings.value.filter((finding) => auditResult(finding) === 'review').length)
const auditHasPassed = computed(() => hasExecutedAudit.value && !auditLoading.value && !error.value && checkFindings.value.length === 0)
const tableFilterGroups: { key: TableFilterKey; label: string }[] = [
  { key: 'result', label: 'Result' },
  { key: 'category', label: 'Finding' },
  { key: 'status', label: 'Severity' },
  { key: 'component', label: 'Component' },
  { key: 'did', label: 'DID' },
]
const tableFilterOptions = computed<Record<TableFilterKey, string[]>>(() => ({
  result: uniqueTableValues(checkFindings.value.map((finding) => auditResultLabel(finding))),
  category: uniqueTableValues(checkFindings.value.map((finding) => finding.category)),
  status: uniqueTableValues(checkFindings.value.map((finding) => finding.status)),
  component: uniqueTableValues(checkFindings.value.map((finding) => finding.component)),
  did: uniqueTableValues(checkFindings.value.map((finding) => finding.did)),
}))

watch(tableFilterOptions, (options) => {
  for (const group of tableFilterGroups) {
    const current = tableFilters.value[group.key]
    const next: Record<string, boolean> = {}
    for (const option of options[group.key]) {
      next[option] = current[option] ?? true
    }
    tableFilters.value[group.key] = next
  }
}, { immediate: true })

const executeAudit = async () => {
  auditLoading.value = true
  error.value = null
  report.value = null
  hasExecutedAudit.value = true
  try {
    const request = selectedAuditMode.value === 'static_contract'
      ? {
          scope: selectedScope.value,
          audit_mode: selectedAuditMode.value,
          contract_document: parseJSONInput(contractDocumentText.value, 'Contract document'),
          contract_did: contractDid.value.trim() || undefined,
          contract_version: contractVersion.value.trim() || undefined,
          policy_version: policyVersion.value.trim() || undefined,
        }
      : { scope: selectedScope.value, audit_mode: selectedAuditMode.value }
    findings.value = await auditingService.audit(request)
    selectedFindingId.value = null
    activeAuditTab.value = checkFindings.value.length > 0 ? 'checks' : 'timeline'
  } catch (err) {
    console.error('Audit Error:', err)
    error.value = err instanceof Error ? err.message : 'Audit could not be executed.'
  } finally {
    auditLoading.value = false
  }
}

const generateReport = async () => {
  reportLoading.value = true
  error.value = null
  try {
    report.value = await auditingService.report({ scope: selectedScope.value })
  } catch (err) {
    console.error('Audit Report Error:', err)
    error.value = 'Audit report could not be generated.'
  } finally {
    reportLoading.value = false
  }
}

const formatLabel = (value: string) => value.split('_').join(' ')

const parseJSONInput = (value: string, label: string) => {
  try {
    return JSON.parse(value)
  } catch {
    throw new Error(`${label} is not valid JSON.`)
  }
}

const reportText = computed(() => {
  if (!report.value) {
    return ''
  }
  return typeof report.value === 'string' ? report.value : JSON.stringify(report.value, null, 2)
})
const selectedFindingRawDetails = computed(() => {
  if (!selectedFinding.value?.details) {
    return ''
  }
  return JSON.stringify(selectedFinding.value.details, null, 2)
})

function tableValue(value?: string) {
  const trimmed = value?.trim()
  return trimmed || emptyValueLabel
}

function uniqueTableValues(values: Array<string | undefined>) {
  return Array.from(new Set(values.map(tableValue))).sort((a, b) => a.localeCompare(b))
}

function tableFilterEnabled(key: TableFilterKey, value?: string) {
  const normalized = tableValue(value)
  return tableFilters.value[key][normalized] ?? true
}

function checkedTableFilterCount(key: TableFilterKey) {
  return tableFilterOptions.value[key].filter((option) => tableFilters.value[key][option]).length
}

function setAllTableFilters(key: TableFilterKey, enabled: boolean) {
  for (const option of tableFilterOptions.value[key]) {
    tableFilters.value[key][option] = enabled
  }
}

function selectFinding(finding: AuditFinding) {
  selectedFindingId.value = finding.id
}

function selectTab(tab: AuditTab) {
  activeAuditTab.value = tab
  selectedFindingId.value = null
}

function stringDetail(value: unknown) {
  if (typeof value === 'string' && value.trim()) {
    return value
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value)
  }
  return emptyValueLabel
}

function isObjectRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function findingBadgeClass(finding: AuditFinding) {
  const result = auditResult(finding)
  if (result === 'passed') {
    return 'badge-success'
  }
  if (result === 'failed') {
    return 'badge-error'
  }
  return 'badge-warning'
}

function auditItemKind(finding: AuditFinding): 'check' | 'event' {
  const eventType = rawEventType(finding)?.toUpperCase()
  const eventData = eventDataFromFinding(finding)
  if (eventType?.includes('POLICY_AUDIT_FINDING') || eventType?.includes('COMPLIANCE_FINDING')) {
    return 'check'
  }
  if (eventData?.ruleId || eventData?.policySetId || eventData?.semanticPath || eventData?.severity) {
    return 'check'
  }
  return 'event'
}

function auditResult(finding: AuditFinding): AuditResult {
  const value = finding.status?.trim().toLowerCase()
  if (value === 'passed' || value === 'pass' || value === 'success' || value === 'successful' || value === 'ok' || value === 'compliant' || value === 'info') {
    return 'passed'
  }
  if (value === 'failed' || value === 'fail' || value === 'error' || value === 'critical' || value === 'blocking' || value === 'violation' || value === 'non_compliant') {
    return 'failed'
  }
  if (value === 'warning' || value === 'warn') {
    return 'review'
  }
  if (finding.category === 'violation') {
    return 'failed'
  }
  if (finding.category === 'inconsistency') {
    return 'review'
  }
  return 'review'
}

function auditResultLabel(finding: AuditFinding) {
  const result = auditResult(finding)
  if (result === 'passed') {
    return 'Passed'
  }
  if (result === 'failed') {
    return 'Failed'
  }
  return 'Needs review'
}

function auditResultSummary(finding: AuditFinding) {
  const assertion = checkAssertion(finding)
  const result = auditResult(finding)
  if (result === 'passed') {
    return assertion ? `Passed: ${assertion}` : 'Check passed'
  }
  if (result === 'failed') {
    return assertion ? `Failed: ${assertion}` : 'Check failed'
  }
  return assertion ? `Review: ${assertion}` : 'Review required'
}

function checkAssertion(finding: AuditFinding) {
  const eventData = eventDataFromFinding(finding)
  const message = stringDetail(eventData?.message)
  if (message !== emptyValueLabel) {
    return message
  }
  const descriptionLine = finding.description
    ?.split('\n')
    .map((line) => line.trim())
    .find((line) => line && !line.startsWith('Object DID:') && !line.startsWith('Rule:') && !line.startsWith('Semantic path:'))
  return descriptionLine || ''
}

function severityLabel(finding: AuditFinding) {
  return finding.status?.trim() || 'not set'
}

function severityBadgeClass(finding: AuditFinding) {
  const severity = finding.status?.trim().toLowerCase()
  if (severity === 'error' || severity === 'critical' || severity === 'blocking' || severity === 'failed' || severity === 'violation') {
    return 'badge-error'
  }
  if (severity === 'warning' || severity === 'warn') {
    return 'badge-warning'
  }
  if (severity === 'passed' || severity === 'pass' || severity === 'success' || severity === 'ok' || severity === 'compliant' || severity === 'info') {
    return 'badge-success'
  }
  return 'badge-ghost'
}

function eventDataFromFinding(finding: AuditFinding) {
  if (!isObjectRecord(finding.details)) {
    return null
  }
  const eventData = finding.details.event_data ?? finding.details.eventData
  return isObjectRecord(eventData) ? eventData : null
}

function rawEventType(finding: AuditFinding) {
  if (!isObjectRecord(finding.details)) {
    return undefined
  }
  const eventType = finding.details.event_type ?? finding.details.eventType
  return typeof eventType === 'string' ? eventType : undefined
}

function actorFromEventData(eventData: Record<string, unknown> | null) {
  if (!eventData) {
    return undefined
  }
  const explicitActor = eventData.actor ?? eventData.user ?? eventData.username ?? eventData.auditedBy
  if (typeof explicitActor === 'string') {
    return explicitActor
  }
  for (const [key, value] of Object.entries(eventData)) {
    if (key.endsWith('_by') && typeof value === 'string') {
      return value
    }
  }
  return undefined
}

function formatDateTime(value?: string) {
  if (!value) {
    return emptyValueLabel
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  return date.toLocaleString()
}
</script>

<template>
  <div class="flex justify-between p-4 mb-4">
    <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
      {{ $route.meta.name }}
    </h2>
  </div>

  <section class="px-4 space-y-4">
    <div class="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
      <div class="stats stats-vertical sm:stats-horizontal bg-base-200 border border-base-content/10">
        <div class="stat">
          <div class="stat-title">Failed Checks</div>
          <div class="stat-value text-2xl text-error">{{ failedCheckCount }}</div>
        </div>
        <div class="stat">
          <div class="stat-title">Passed Checks</div>
          <div class="stat-value text-2xl text-success">{{ passedCheckCount }}</div>
        </div>
        <div class="stat">
          <div class="stat-title">Needs Review</div>
          <div class="stat-value text-2xl text-warning">{{ reviewCheckCount }}</div>
        </div>
      </div>

      <div class="flex flex-col gap-3 sm:flex-row">
        <label class="form-control w-full sm:w-48">
          <span class="label-text mb-1">Scope</span>
          <select v-model="selectedScope" class="select select-bordered rounded-box" :disabled="auditLoading || reportLoading || selectedAuditMode === 'static_contract'">
            <option v-for="scope in scopeOptions" :key="scope.value" :value="scope.value">
              {{ scope.label }}
            </option>
          </select>
        </label>

        <label class="form-control w-full sm:w-48">
          <span class="label-text mb-1">Mode</span>
          <select
            v-model="selectedAuditMode"
            class="select select-bordered rounded-box"
            :disabled="auditLoading || reportLoading"
          >
            <option v-for="mode in auditModeOptions" :key="mode.value" :value="mode.value">
              {{ mode.label }}
            </option>
          </select>
        </label>

        <button class="btn btn-primary rounded-box sm:self-end" :disabled="auditLoading || reportLoading" @click="executeAudit">
          <span v-if="auditLoading" class="loading loading-spinner loading-sm"></span>
          <span v-else>Execute Audit</span>
        </button>

        <button
          class="btn btn-secondary rounded-box sm:self-end"
          :disabled="reportLoading || auditLoading || !hasExecutedAudit"
          @click="generateReport"
        >
          <span v-if="reportLoading" class="loading loading-spinner loading-sm"></span>
          <span v-else>Generate Report</span>
        </button>
      </div>
    </div>

    <div v-if="selectedAuditMode === 'static_contract'" class="space-y-3">
      <div class="space-y-3">
        <div class="grid gap-3 sm:grid-cols-3">
          <label class="form-control">
            <span class="label-text mb-1">Contract DID</span>
            <input v-model="contractDid" class="input input-bordered rounded-box" :disabled="auditLoading || reportLoading" />
          </label>
          <label class="form-control">
            <span class="label-text mb-1">Contract Version</span>
            <input v-model="contractVersion" class="input input-bordered rounded-box" :disabled="auditLoading || reportLoading" />
          </label>
          <label class="form-control">
            <span class="label-text mb-1">Policy Version</span>
            <input v-model="policyVersion" class="input input-bordered rounded-box" :disabled="auditLoading || reportLoading" />
          </label>
        </div>
        <label class="form-control">
          <span class="label-text mb-1">Contract JSON-LD</span>
          <textarea
            v-model="contractDocumentText"
            class="textarea textarea-bordered rounded-box min-h-96 font-mono text-xs leading-5"
            spellcheck="false"
            :disabled="auditLoading || reportLoading"
          ></textarea>
        </label>
      </div>
    </div>

    <div v-if="auditLoading" class="p-4">Executing audit...</div>
    <div v-else-if="error" class="alert alert-error rounded-box">{{ error }}</div>
    <div v-if="auditHasPassed" class="alert alert-success rounded-box">
      Audit passed. No failed checks or review findings were returned.
    </div>

    <div v-if="!auditLoading && !error" class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_24rem]">
      <div class="overflow-x-auto border border-base-content/10 rounded-box">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-base-content/10 px-4 py-3">
          <div role="tablist" class="tabs tabs-box">
            <button
              type="button"
              role="tab"
              class="tab"
              :class="activeAuditTab === 'checks' ? 'tab-active' : ''"
              @click="selectTab('checks')"
            >
              Checks
              <span class="badge badge-sm ml-2">{{ checkFindings.length }}</span>
            </button>
            <button
              type="button"
              role="tab"
              class="tab"
              :class="activeAuditTab === 'timeline' ? 'tab-active' : ''"
              @click="selectTab('timeline')"
            >
              Timeline
              <span class="badge badge-sm ml-2">{{ timelineEvents.length }}</span>
            </button>
          </div>

          <div v-if="activeAuditTab === 'checks'" class="flex flex-wrap items-center gap-2">
            <span class="text-sm font-medium opacity-70">Table filters</span>
            <details v-for="group in tableFilterGroups" :key="group.key" class="dropdown">
              <summary class="btn btn-sm btn-outline rounded-box">
                {{ group.label }}
                <span class="badge badge-sm">{{ checkedTableFilterCount(group.key) }}/{{ tableFilterOptions[group.key].length }}</span>
              </summary>
              <div class="dropdown-content z-10 mt-2 w-72 rounded-box border border-base-content/10 bg-base-100 p-3 shadow">
                <div class="mb-2 flex justify-between gap-2">
                  <button type="button" class="btn btn-xs btn-ghost" @click="setAllTableFilters(group.key, true)">All</button>
                  <button type="button" class="btn btn-xs btn-ghost" @click="setAllTableFilters(group.key, false)">None</button>
                </div>
                <div class="max-h-64 overflow-auto space-y-1">
                  <label
                    v-for="option in tableFilterOptions[group.key]"
                    :key="option"
                    class="flex min-h-8 items-center gap-2 rounded px-2 hover:bg-base-200"
                  >
                    <input
                      v-model="tableFilters[group.key][option]"
                      type="checkbox"
                      class="checkbox checkbox-sm checkbox-primary"
                    />
                    <span class="text-sm break-all">{{ group.key === 'category' ? formatLabel(option) : option }}</span>
                  </label>
                </div>
              </div>
            </details>
          </div>
        </div>
        <table v-if="activeAuditTab === 'checks'" class="table table-zebra">
          <thead>
            <tr>
              <th>Result</th>
              <th>Severity</th>
              <th>DID</th>
              <th>Details</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="finding in filteredFindings"
              :key="finding.id"
              class="cursor-pointer"
              :class="String(selectedFindingId) === String(finding.id) ? 'bg-primary/10' : ''"
              tabindex="0"
              @click="selectFinding(finding)"
              @keydown.enter.prevent="selectFinding(finding)"
              @keydown.space.prevent="selectFinding(finding)"
            >
              <td>
                <span class="badge" :class="findingBadgeClass(finding)">
                  {{ auditResultLabel(finding) }}
                </span>
                <div class="mt-1 text-xs opacity-70">{{ formatLabel(finding.category) }}</div>
              </td>
              <td>
                <span class="badge badge-outline" :class="severityBadgeClass(finding)">
                  {{ severityLabel(finding) }}
                </span>
              </td>
              <td>{{ finding.did ?? '-' }}</td>
              <td class="min-w-72 max-w-xl">
                <div class="font-medium">{{ checkAssertion(finding) || finding.title || 'Audit finding' }}</div>
                <div v-if="checkAssertion(finding) && finding.title" class="text-xs opacity-70">{{ finding.title }}</div>
                <div class="text-xs opacity-70">{{ auditResultSummary(finding) }}</div>
              </td>
            </tr>
            <tr v-if="filteredFindings.length === 0">
              <td colspan="4" class="text-center py-8 opacity-70">
                {{ hasExecutedAudit ? 'No checks match the selected filters.' : 'Select a scope and execute an audit.' }}
              </td>
            </tr>
          </tbody>
        </table>

        <table v-else class="table table-zebra">
          <thead>
            <tr>
              <th>Time</th>
              <th>Event</th>
              <th>Actor</th>
              <th>DID</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="event in timelineEvents"
              :key="event.id"
              class="cursor-pointer"
              :class="String(selectedFindingId) === String(event.id) ? 'bg-primary/10' : ''"
              tabindex="0"
              @click="selectFinding(event)"
              @keydown.enter.prevent="selectFinding(event)"
              @keydown.space.prevent="selectFinding(event)"
            >
              <td class="whitespace-nowrap">{{ formatDateTime(event.created_at) }}</td>
              <td class="min-w-72 max-w-xl">
                <div class="font-medium">{{ event.title ?? formatLabel(rawEventType(event) ?? 'Audit event') }}</div>
                <div class="text-xs opacity-70">{{ event.component ?? '-' }}</div>
              </td>
              <td>{{ actorFromEventData(eventDataFromFinding(event)) ?? '-' }}</td>
              <td>{{ event.did ?? '-' }}</td>
            </tr>
            <tr v-if="timelineEvents.length === 0">
              <td colspan="4" class="text-center py-8 opacity-70">
                {{ hasExecutedAudit ? 'No timeline events were returned.' : 'Select a scope and execute an audit.' }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <aside class="border border-base-content/10 rounded-box bg-base-100 min-h-80 xl:sticky xl:top-4 xl:self-start">
        <div class="flex items-center justify-between border-b border-base-content/10 px-4 py-3">
          <h3 class="font-bold">{{ selectedFindingKind === 'event' ? 'Event Details' : 'Check Details' }}</h3>
          <button
            v-if="selectedFinding"
            type="button"
            class="btn btn-xs btn-ghost"
            @click="selectedFindingId = null"
          >
            Close
          </button>
        </div>
        <div v-if="!selectedFinding" class="p-4 text-sm opacity-70">
          Select a row to inspect the corresponding audit evidence.
        </div>
        <div v-else class="p-4 space-y-4">
          <div v-if="selectedFindingKind === 'check'">
            <div class="mb-2 badge" :class="findingBadgeClass(selectedFinding)">
              {{ auditResultLabel(selectedFinding) }}
            </div>
            <h4 class="font-bold leading-snug">{{ checkAssertion(selectedFinding) || selectedFinding.title || 'Audit finding' }}</h4>
            <div v-if="checkAssertion(selectedFinding) && selectedFinding.title" class="mt-1 text-sm opacity-70">
              {{ selectedFinding.title }}
            </div>
            <div class="mt-2 flex flex-wrap items-center gap-2 text-xs opacity-80">
              <span>{{ formatLabel(selectedFinding.category) }}</span>
              <span class="badge badge-outline badge-sm" :class="severityBadgeClass(selectedFinding)">
                Severity: {{ severityLabel(selectedFinding) }}
              </span>
            </div>
          </div>

          <div v-else>
            <div class="mb-2 badge badge-outline">Timeline event</div>
            <h4 class="font-bold leading-snug">{{ selectedFinding.title ?? formatLabel(rawEventType(selectedFinding) ?? 'Audit event') }}</h4>
            <div class="mt-2 text-xs opacity-80">{{ formatDateTime(selectedFinding.created_at) }}</div>
          </div>

          <p class="text-sm whitespace-pre-wrap break-words opacity-80">{{ selectedFinding.description }}</p>

          <dl class="divide-y divide-base-content/10 text-sm">
            <div
              v-for="row in selectedFindingDetailRows"
              :key="row.label"
              class="grid grid-cols-[8rem_minmax(0,1fr)] gap-3 py-2"
            >
              <dt class="font-medium opacity-70">{{ row.label }}</dt>
              <dd class="break-words">{{ row.value }}</dd>
            </div>
          </dl>

          <details v-if="selectedFindingRawDetails" class="collapse collapse-arrow border border-base-content/10 rounded-box">
            <summary class="collapse-title text-sm font-medium">Raw Details</summary>
            <div class="collapse-content">
              <pre class="text-xs whitespace-pre-wrap break-words">{{ selectedFindingRawDetails }}</pre>
            </div>
          </details>
        </div>
      </aside>
    </div>

    <div v-if="report" class="border border-base-content/10 rounded-box p-4 bg-base-200">
      <h3 class="font-bold mb-3">Structured Audit Report</h3>
      <pre class="text-xs whitespace-pre-wrap break-words">{{ reportText }}</pre>
    </div>
  </section>
</template>
