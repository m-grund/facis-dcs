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
const selectedScope = ref<AuditScope>('templates')
const selectedAuditMode = ref<AuditMode>('repository_trail')
const hasExecutedAudit = ref(false)
type TableFilterKey = 'category' | 'status' | 'component' | 'did'

const emptyValueLabel = '-'
const tableFilters = ref<Record<TableFilterKey, Record<string, boolean>>>({
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
const policyText = ref(`{
  "policySetId": "facis.dcs.contract.structure-semantics",
  "version": "2026-05-18",
  "shaclShapes": [
    {
      "id": "FACIS-CONTRACT-SHACL-SLA",
      "title": "Contract JSON-LD must satisfy the SLA SHACL shape",
      "targetClass": "dcs:Contract",
      "severity": "error",
      "requirement": "DCS-FR-PACM-03",
      "properties": [
        {
          "path": "@id",
          "name": "Contract identifier",
          "minCount": 1,
          "maxCount": 1,
          "datatype": "xsd:anyURI"
        },
        {
          "path": "@type",
          "name": "Contract type",
          "minCount": 1,
          "in": ["dcs:Contract", "Contract"]
        },
        {
          "path": "parties",
          "name": "Contract parties",
          "minCount": 2,
          "class": "dcs:CompanyParty"
        },
        {
          "path": "contract.jurisdiction",
          "name": "Jurisdiction",
          "minCount": 1,
          "datatype": "xsd:string"
        },
        {
          "path": "service.sla.availability",
          "name": "SLA availability",
          "minCount": 1,
          "datatype": "xsd:decimal"
        }
      ]
    }
  ],
  "rules": [
    {
      "id": "FACIS-CONTRACT-STATIC-002",
      "title": "Contract jurisdiction must be allowed",
      "builtin": "value_in",
      "semanticPath": "contract.jurisdiction",
      "values": ["DEU", "AUT", "CHE", "FRA", "NLD", "BEL", "LUX", "POL", "CZE", "ESP", "ITA", "GBR", "USA"],
      "ontologyTerm": "dcs:Contract",
      "requirement": "DCS-FR-PACM-03"
    },
    {
      "id": "FACIS-CONTRACT-STATIC-003",
      "title": "Service availability must satisfy policy minimum",
      "builtin": "min_number",
      "semanticPath": "service.sla.availability",
      "min": 99.9,
      "ontologyTerm": "sla:AvailabilityMetric",
      "requirement": "DCS-FR-CWE-09"
    },
    {
      "id": "FACIS-CONTRACT-STATIC-004",
      "title": "Service response time must satisfy policy maximum",
      "builtin": "max_number",
      "semanticPath": "service.sla.responseTime",
      "max": 15,
      "ontologyTerm": "sla:ResponseTimeMetric",
      "requirement": "DCS-FR-CWE-09"
    },
    {
      "id": "FACIS-CONTRACT-STATIC-005",
      "title": "Service resolution time must satisfy policy maximum",
      "builtin": "max_number",
      "semanticPath": "service.sla.resolutionTime",
      "max": 240,
      "ontologyTerm": "sla:ResolutionTimeMetric",
      "requirement": "DCS-FR-CWE-09"
    },
    {
      "id": "FACIS-CONTRACT-STATIC-006",
      "title": "Signature level must satisfy policy",
      "builtin": "signature_level_at_least",
      "semanticPath": "signature.requiredLevel",
      "required": "AES",
      "ontologyTerm": "dcs:SignatureLevelCode",
      "requirement": "DCS-FR-PACM-03"
    }
  ]
}`)

const scopeOptions: { value: AuditScope; label: string }[] = [
  { value: 'templates', label: 'Templates' },
  { value: 'contracts', label: 'Contracts' },
  { value: 'signatures', label: 'Signatures' },
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
  return findings.value.filter((finding) => {
    return tableFilterEnabled('category', finding.category)
      && tableFilterEnabled('status', finding.status)
      && tableFilterEnabled('component', finding.component)
      && tableFilterEnabled('did', finding.did)
  })
})
const selectedFinding = computed(() => {
  return findings.value.find((finding) => String(finding.id) === String(selectedFindingId.value)) ?? null
})
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
  return [
    { label: 'Rule ID', value: stringDetail(eventData?.ruleId) },
    { label: 'Policy Set', value: stringDetail(eventData?.policySetId) },
    { label: 'Policy Version', value: stringDetail(eventData?.policyVersion) },
    { label: 'Requirement', value: stringDetail(eventData?.requirement) },
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

const violationCount = computed(() => findings.value.filter((finding) => finding.category === 'violation').length)
const inconsistencyCount = computed(() => findings.value.filter((finding) => finding.category === 'inconsistency').length)
const complianceCheckCount = computed(() => findings.value.filter((finding) => finding.category === 'compliance_check').length)
const tableFilterGroups: { key: TableFilterKey; label: string }[] = [
  { key: 'category', label: 'Finding' },
  { key: 'status', label: 'Status' },
  { key: 'component', label: 'Component' },
  { key: 'did', label: 'DID' },
]
const tableFilterOptions = computed<Record<TableFilterKey, string[]>>(() => ({
  category: uniqueTableValues(findings.value.map((finding) => finding.category)),
  status: uniqueTableValues(findings.value.map((finding) => finding.status)),
  component: uniqueTableValues(findings.value.map((finding) => finding.component)),
  did: uniqueTableValues(findings.value.map((finding) => finding.did)),
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
          policy: parseJSONInput(policyText.value, 'Policy'),
          contract_did: contractDid.value.trim() || undefined,
          contract_version: contractVersion.value.trim() || undefined,
          policy_version: policyVersion.value.trim() || undefined,
        }
      : { scope: selectedScope.value, audit_mode: selectedAuditMode.value }
    findings.value = await auditingService.audit(request)
    selectedFindingId.value = null
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
  if (finding.category === 'violation') {
    return 'badge-error'
  }
  if (finding.category === 'inconsistency') {
    return 'badge-warning'
  }
  return 'badge-info'
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
          <div class="stat-title">Violations</div>
          <div class="stat-value text-2xl">{{ violationCount }}</div>
        </div>
        <div class="stat">
          <div class="stat-title">Inconsistencies</div>
          <div class="stat-value text-2xl">{{ inconsistencyCount }}</div>
        </div>
        <div class="stat">
          <div class="stat-title">Compliance Checks</div>
          <div class="stat-value text-2xl">{{ complianceCheckCount }}</div>
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

    <div v-if="selectedAuditMode === 'static_contract'" class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
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
      <label class="form-control">
        <span class="label-text mb-1">Policy and SHACL JSON</span>
        <textarea
          v-model="policyText"
          class="textarea textarea-bordered rounded-box min-h-96 font-mono text-xs leading-5"
          spellcheck="false"
          :disabled="auditLoading || reportLoading"
        ></textarea>
      </label>
    </div>

    <div v-if="auditLoading" class="p-4">Executing audit...</div>
    <div v-else-if="error" class="alert alert-error rounded-box">{{ error }}</div>

    <div v-else class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_24rem]">
      <div class="overflow-x-auto border border-base-content/10 rounded-box">
        <div class="flex flex-wrap items-center gap-2 border-b border-base-content/10 px-4 py-3">
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
        <table class="table table-zebra">
          <thead>
            <tr>
              <th>Finding</th>
              <th>Status</th>
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
                <div class="font-medium capitalize">{{ formatLabel(finding.category) }}</div>
              </td>
              <td>
                <span v-if="finding.status" class="badge" :class="findingBadgeClass(finding)">
                  {{ finding.status }}
                </span>
                <span v-else>-</span>
              </td>
              <td>{{ finding.did ?? '-' }}</td>
              <td class="min-w-72 max-w-xl">
                <div class="font-medium">{{ finding.title ?? 'Audit finding' }}</div>
              </td>
            </tr>
            <tr v-if="filteredFindings.length === 0">
              <td colspan="4" class="text-center py-8 opacity-70">
                {{ hasExecutedAudit ? 'No findings match the selected filters.' : 'Select a scope and execute an audit.' }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <aside class="border border-base-content/10 rounded-box bg-base-100 min-h-80 xl:sticky xl:top-4 xl:self-start">
        <div class="flex items-center justify-between border-b border-base-content/10 px-4 py-3">
          <h3 class="font-bold">Finding Details</h3>
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
          Select a finding row to inspect policy, requirement, semantic path, and raw audit evidence.
        </div>
        <div v-else class="p-4 space-y-4">
          <div>
            <div class="text-xs uppercase opacity-60">{{ formatLabel(selectedFinding.category) }}</div>
            <h4 class="font-bold leading-snug">{{ selectedFinding.title ?? 'Audit finding' }}</h4>
            <div class="mt-2 badge" :class="findingBadgeClass(selectedFinding)">
              {{ selectedFinding.status ?? selectedFinding.category }}
            </div>
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
