<script setup lang="ts">
import type { AuditScope } from '@/models/requests/auditing-request'
import type { AuditFinding } from '@/models/responses/auditing-response'
import { auditingService } from '@/services/auditing-service'
import { computed, ref } from 'vue'

const findings = ref<AuditFinding[]>([])
const report = ref<unknown>(null)
const auditLoading = ref(false)
const reportLoading = ref(false)
const error = ref<string | null>(null)
const selectedScope = ref<AuditScope>('templates')
const selectedCategory = ref('All')
const hasExecutedAudit = ref(false)

const scopeOptions: { value: AuditScope; label: string }[] = [
  { value: 'templates', label: 'Templates' },
  { value: 'contracts', label: 'Contracts' },
  { value: 'signatures', label: 'Signatures' },
  { value: 'archive', label: 'Archive' },
]

const categoryOptions = ['violation', 'inconsistency', 'compliance_check']

const filteredFindings = computed(() => {
  return findings.value.filter((finding) => {
    return selectedCategory.value === 'All' || finding.category === selectedCategory.value
  })
})

const violationCount = computed(() => findings.value.filter((finding) => finding.category === 'violation').length)
const inconsistencyCount = computed(() => findings.value.filter((finding) => finding.category === 'inconsistency').length)
const complianceCheckCount = computed(() => findings.value.filter((finding) => finding.category === 'compliance_check').length)

const executeAudit = async () => {
  auditLoading.value = true
  error.value = null
  report.value = null
  hasExecutedAudit.value = true
  try {
    findings.value = await auditingService.audit({ scope: selectedScope.value })
    selectedCategory.value = 'All'
  } catch (err) {
    console.error('Audit Error:', err)
    error.value = 'Audit could not be executed.'
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

const formatTimestamp = (timestamp: string) => {
  const date = new Date(timestamp)
  if (Number.isNaN(date.getTime())) {
    return timestamp
  }
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date)
}

const formatLabel = (value: string) => value.replaceAll('_', ' ')

const reportText = computed(() => {
  if (!report.value) {
    return ''
  }
  return typeof report.value === 'string' ? report.value : JSON.stringify(report.value, null, 2)
})
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
          <select v-model="selectedScope" class="select select-bordered rounded-box" :disabled="auditLoading || reportLoading">
            <option v-for="scope in scopeOptions" :key="scope.value" :value="scope.value">
              {{ scope.label }}
            </option>
          </select>
        </label>

        <label class="form-control w-full sm:w-48">
          <span class="label-text mb-1">Finding</span>
          <select v-model="selectedCategory" class="select select-bordered rounded-box" :disabled="auditLoading || reportLoading">
            <option>All</option>
            <option v-for="category in categoryOptions" :key="category" :value="category">
              {{ formatLabel(category) }}
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

    <div v-if="auditLoading" class="p-4">Executing audit...</div>
    <div v-else-if="error" class="alert alert-error rounded-box">{{ error }}</div>

    <div v-else class="overflow-x-auto border border-base-content/10 rounded-box">
      <table class="table table-zebra">
        <thead>
          <tr>
            <th>Timestamp</th>
            <th>Finding</th>
            <th>Status</th>
            <th>DID</th>
            <th>Details</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="finding in filteredFindings" :key="finding.id">
            <td class="whitespace-nowrap">{{ formatTimestamp(finding.created_at) }}</td>
            <td>
              <div class="font-medium capitalize">{{ formatLabel(finding.category) }}</div>
              <div v-if="finding.component" class="text-xs opacity-60">{{ finding.component }}</div>
            </td>
            <td>{{ finding.status ?? '-' }}</td>
            <td>{{ finding.did ?? '-' }}</td>
            <td class="min-w-72 max-w-xl">
              <div class="font-medium">{{ finding.title ?? 'Audit finding' }}</div>
              <div class="text-sm opacity-80 whitespace-pre-wrap break-words">{{ finding.description }}</div>
            </td>
          </tr>
          <tr v-if="filteredFindings.length === 0">
            <td colspan="5" class="text-center py-8 opacity-70">
              {{ hasExecutedAudit ? 'No findings match the selected filters.' : 'Select a scope and execute an audit.' }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="report" class="border border-base-content/10 rounded-box p-4 bg-base-200">
      <h3 class="font-bold mb-3">Structured Audit Report</h3>
      <pre class="text-xs whitespace-pre-wrap break-words">{{ reportText }}</pre>
    </div>
  </section>
</template>
