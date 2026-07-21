<script setup lang="ts">
import { computed, onMounted, ref, useId } from 'vue'
import {
  type SignatureAuditEntry,
  type SignatureComplianceResult,
  type SignatureContract,
  signatureManagementService,
  type SignatureValidateResult,
  type SignatureViewItem,
  type SignatureViewResult,
} from '@/services/signature-management-service'
import { useAuthStore } from '@/stores/auth-store'

// The Signature Compliance Viewer (DCS-FR-SM-05/-07/-08, DCS-FR-SM-18/-21/-26):
// a tabbed dashboard over the signed contracts an Auditor / Compliance Officer /
// Contract Manager may inspect — Validation, Revocation, Compliance Checks, and
// Audit Reports — with pass/fail indicators and PDF+JSON report export.

type Tab = 'validation' | 'revocation' | 'compliance' | 'audit'

const authStore = useAuthStore()
// The validate / revoke / compliance endpoints require a Contract Manager scope;
// the audit endpoint requires Auditor or Compliance Officer. Gate the controls
// so the UI only offers actions the caller can actually perform.
const canManage = computed(() => authStore.user?.roles?.includes('CONTRACT_MANAGER') ?? false)
const canAudit = computed(
  () => authStore.user?.roles?.some((r) => r === 'AUDITOR' || r === 'COMPLIANCE_OFFICER') ?? false,
)

const contracts = ref<SignatureContract[]>([])
const loadingContracts = ref(false)
const error = ref<string | null>(null)

const search = ref('')
const statusFilter = ref('')

const searchId = useId()
const statusFilterId = useId()

const selected = ref<SignatureContract | null>(null)
const activeTab = ref<Tab>('validation')

const view = ref<SignatureViewResult | null>(null)
const loadingView = ref(false)
const validateResult = ref<SignatureValidateResult | null>(null)
const complianceResult = ref<SignatureComplianceResult | null>(null)
const auditEntries = ref<SignatureAuditEntry[] | null>(null)
const busy = ref(false)

const statuses = computed(() => {
  const set = new Set<string>()
  for (const c of contracts.value) {
    if (c.state) set.add(c.state)
  }
  return Array.from(set).sort()
})

const filteredContracts = computed(() => {
  const q = search.value.trim().toLowerCase()
  return contracts.value.filter((c) => {
    if (statusFilter.value && c.state !== statusFilter.value) return false
    if (!q) return true
    return c.did.toLowerCase().includes(q) || (c.name ?? '').toLowerCase().includes(q)
  })
})

onMounted(async () => {
  loadingContracts.value = true
  try {
    contracts.value = await signatureManagementService.retrieveContracts()
  } catch {
    error.value = 'Failed to load contracts.'
  } finally {
    loadingContracts.value = false
  }
})

async function selectContract(contract: SignatureContract) {
  selected.value = contract
  activeTab.value = 'validation'
  view.value = null
  validateResult.value = null
  complianceResult.value = null
  auditEntries.value = null
  loadingView.value = true
  try {
    view.value = await signatureManagementService.getSignatureView(contract.did)
  } catch (e: unknown) {
    error.value = `Failed to load signature data: ${e instanceof Error ? e.message : String(e)}`
  } finally {
    loadingView.value = false
  }
}

async function runValidate() {
  if (!selected.value) return
  busy.value = true
  error.value = null
  try {
    validateResult.value = await signatureManagementService.validateSignature(selected.value.did)
  } catch (e: unknown) {
    error.value = `Validation failed: ${e instanceof Error ? e.message : String(e)}`
  } finally {
    busy.value = false
  }
}

async function runCompliance() {
  if (!selected.value) return
  busy.value = true
  error.value = null
  try {
    complianceResult.value = await signatureManagementService.complianceCheck(selected.value.did)
  } catch (e: unknown) {
    error.value = `Compliance check failed: ${e instanceof Error ? e.message : String(e)}`
  } finally {
    busy.value = false
  }
}

async function revoke(sig: SignatureViewItem) {
  if (!selected.value) return
  busy.value = true
  error.value = null
  try {
    await signatureManagementService.revokeSignature(selected.value.did, sig.signer_did)
    view.value = await signatureManagementService.getSignatureView(selected.value.did)
  } catch (e: unknown) {
    error.value = `Revocation failed: ${e instanceof Error ? e.message : String(e)}`
  } finally {
    busy.value = false
  }
}

async function loadAudit() {
  if (!selected.value) return
  busy.value = true
  error.value = null
  try {
    auditEntries.value = await signatureManagementService.getAudit(selected.value.did)
  } catch (e: unknown) {
    error.value = `Failed to load audit report: ${e instanceof Error ? e.message : String(e)}`
  } finally {
    busy.value = false
  }
}

// --- Pass/fail derivation -------------------------------------------------

interface Indicator {
  label: string
  cls: string
}

function dssIndicator(indication: string | undefined): Indicator {
  switch ((indication ?? '').toUpperCase()) {
    case 'TOTAL-PASSED':
      return { label: 'PASSED', cls: 'badge-success' }
    case 'INDETERMINATE':
      return { label: 'INDETERMINATE', cls: 'badge-warning' }
    case 'TOTAL-FAILED':
      return { label: 'FAILED', cls: 'badge-error' }
    default:
      return { label: indication ?? 'Unknown', cls: 'badge-ghost' }
  }
}

const FAILURE_KEYWORDS =
  /(mismatch|drift detected|does not match|failed|could not|missing|no longer|power of attorney)/i

function isFailureFinding(finding: string): boolean {
  return FAILURE_KEYWORDS.test(finding)
}

function findingIndicator(finding: string): Indicator {
  return isFailureFinding(finding) ? { label: 'FAIL', cls: 'badge-error' } : { label: 'PASS', cls: 'badge-success' }
}

// Prefer the freshest structured DSS report: the one just returned by the
// Validate action, else the one loaded with the signature view.
const activeDss = computed(() => validateResult.value?.dss ?? view.value?.dss ?? null)

// The integrity findings shown are the freshest available: the Validate
// action's result once it has run, else those loaded with the view.
const integrityFindings = computed(() => validateResult.value?.findings ?? view.value?.integrity_findings ?? [])

const integrityIntact = computed(() => {
  const findings = integrityFindings.value
  return findings.length > 0 && !findings.some(isFailureFinding)
})

function statusIndicator(status: string): Indicator {
  return status.toUpperCase() === 'REVOKED'
    ? { label: 'REVOKED', cls: 'badge-error' }
    : { label: 'ACTIVE', cls: 'badge-success' }
}

// DCS-FR-SM-21: the signature level (SES/AES/QES). A qualified PAdES level from
// the DSS report (…-BASELINE-LTA on a qualified chain) is QES; otherwise the
// credential class the signature was applied under stands.
function signatureLevel(sig: SignatureViewItem): string {
  return (sig.credential_type || 'AES').toUpperCase()
}

// --- Report export --------------------------------------------------------

function buildReport() {
  return {
    contract_did: selected.value?.did,
    contract_name: selected.value?.name,
    contract_state: view.value?.contract_state,
    generated_at: new Date().toISOString(),
    dss_report: activeDss.value,
    integrity_findings: integrityFindings.value,
    validation_findings: validateResult.value?.findings ?? [],
    compliance_findings: complianceResult.value?.findings ?? [],
    signatures: view.value?.signatures ?? [],
    audit_entries: auditEntries.value ?? [],
  }
}

function reportFilename(ext: string): string {
  const base = (selected.value?.name ?? selected.value?.did ?? 'contract').replace(/[^\w.-]+/g, '_')
  return `compliance-report-${base}.${ext}`
}

function exportJson() {
  const blob = new Blob([JSON.stringify(buildReport(), null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = reportFilename('json')
  anchor.click()
  URL.revokeObjectURL(url)
}

function escapeHtml(value: string | number | boolean | null | undefined): string {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

// PDF export without a bundled PDF library: render the assembled report into a
// print window and let the browser's "Save as PDF" produce the document. This
// reuses the platform print pipeline rather than adding a client-side PDF
// dependency.
function exportPdf() {
  const report = buildReport()
  const rows = (label: string, items: string[]) =>
    items.length
      ? `<h3>${escapeHtml(label)}</h3><ul>${items.map((i) => `<li>${escapeHtml(i)}</li>`).join('')}</ul>`
      : ''
  const sigRows = report.signatures
    .map(
      (s) => `<tr>
        <td>${escapeHtml(s.signer_did)}</td>
        <td>${escapeHtml(s.field_name ?? '—')}</td>
        <td>${escapeHtml(signatureLevel(s))}</td>
        <td>${escapeHtml(s.status)}</td>
        <td>${escapeHtml(s.signed_at ?? '—')}</td>
        <td style="font-family:monospace;font-size:10px">${escapeHtml(s.content_hash ?? '—')}</td>
      </tr>`,
    )
    .join('')
  const dss = report.dss_report
  const dssHtml = dss
    ? `<h3>EU DSS Validation Report</h3><ul>
        <li>Indication: ${escapeHtml(dss.indication)}</li>
        <li>Sub-indication: ${escapeHtml(dss.sub_indication ?? '—')}</li>
        <li>Signed by: ${escapeHtml(dss.signed_by ?? '—')}</li>
        <li>Signature format: ${escapeHtml(dss.signature_format ?? '—')}</li>
        <li>Signing time: ${escapeHtml(dss.signing_time ?? '—')}</li>
      </ul>`
    : ''
  const html = `<!doctype html><html><head><meta charset="utf-8"><title>${escapeHtml(reportFilename('pdf'))}</title>
    <style>
      body{font-family:system-ui,sans-serif;margin:2rem;color:#111}
      h1{font-size:1.4rem} h2{font-size:1.1rem;margin-top:1.5rem} h3{font-size:1rem;margin-top:1rem}
      table{border-collapse:collapse;width:100%;font-size:12px} th,td{border:1px solid #ccc;padding:4px 6px;text-align:left}
      ul{font-size:13px} .meta{color:#555;font-size:12px}
    </style></head><body>
      <h1>Signature Compliance Report</h1>
      <p class="meta">
        Contract: ${escapeHtml(report.contract_name ?? report.contract_did)}<br>
        DID: ${escapeHtml(report.contract_did)}<br>
        State: ${escapeHtml(report.contract_state)}<br>
        Generated: ${escapeHtml(report.generated_at)}
      </p>
      <h2>Signatures</h2>
      <table><thead><tr><th>Signer</th><th>Field</th><th>Level</th><th>Status</th><th>Signed at</th><th>Content hash</th></tr></thead>
      <tbody>${sigRows || '<tr><td colspan="6">No signatures</td></tr>'}</tbody></table>
      ${dssHtml}
      ${rows('Integrity Findings', report.integrity_findings)}
      ${rows('Validation Findings', report.validation_findings)}
      ${rows('Compliance Findings', report.compliance_findings)}
    </body></html>`
  const win = window.open('', '_blank')
  if (!win) {
    error.value = 'Could not open the print window — allow pop-ups to export the PDF report.'
    return
  }
  win.document.write(html)
  win.document.close()
  win.focus()
  win.print()
}
</script>

<template>
  <div class="mb-4 flex items-center justify-between border-b border-base-content/10 bg-base-100 p-4">
    <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">Signature Compliance Viewer</h2>
  </div>

  <div class="p-4">
    <div v-if="error" class="mb-4 alert alert-error">{{ error }}</div>

    <div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
      <!-- Contract list: filter/search signed contracts by compliance status -->
      <div class="lg:col-span-1">
        <div class="mb-2 flex flex-col gap-2">
          <input
            :id="searchId"
            v-model="search"
            type="text"
            placeholder="Search DID or name…"
            aria-label="Search contract DID or name"
            class="input-bordered input input-sm w-full"
          />
          <select
            :id="statusFilterId"
            v-model="statusFilter"
            aria-label="Select contract state"
            class="select-bordered select w-full select-sm"
          >
            <option value="">All statuses</option>
            <option v-for="s in statuses" :key="s" :value="s">{{ s }}</option>
          </select>
        </div>

        <div v-if="loadingContracts" class="text-base-content/60">Loading contracts…</div>
        <div v-else-if="filteredContracts.length === 0" class="text-base-content/60">No contracts match.</div>
        <ul v-else class="menu w-full rounded-box bg-base-200 p-1">
          <li v-for="contract in filteredContracts" :key="contract.did">
            <button
              type="button"
              :class="{ active: selected?.did === contract.did }"
              class="flex flex-col items-start gap-0"
              @click="selectContract(contract)"
            >
              <span class="font-medium">{{ contract.name ?? contract.did }}</span>
              <span class="flex items-center gap-2">
                <span class="badge badge-ghost badge-xs">{{ contract.state }}</span>
                <span class="truncate font-mono text-[10px] opacity-70">{{ contract.did }}</span>
              </span>
            </button>
          </li>
        </ul>
      </div>

      <!-- Tabbed dashboard for the selected contract -->
      <div class="lg:col-span-2">
        <div v-if="!selected" class="text-base-content/70">Select a contract to inspect its signatures.</div>
        <div v-else>
          <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
            <div role="tablist" class="tabs-boxed tabs">
              <button
                role="tab"
                class="tab"
                :class="{ 'tab-active': activeTab === 'validation' }"
                @click="activeTab = 'validation'"
              >
                Validation
              </button>
              <button
                role="tab"
                class="tab"
                :class="{ 'tab-active': activeTab === 'revocation' }"
                @click="activeTab = 'revocation'"
              >
                Revocation
              </button>
              <button
                role="tab"
                class="tab"
                :class="{ 'tab-active': activeTab === 'compliance' }"
                @click="activeTab = 'compliance'"
              >
                Compliance Checks
              </button>
              <button
                role="tab"
                class="tab"
                :class="{ 'tab-active': activeTab === 'audit' }"
                @click="activeTab = 'audit'"
              >
                Audit Reports
              </button>
            </div>
            <div class="flex gap-2">
              <button class="btn btn-outline btn-sm" @click="exportJson">Export JSON</button>
              <button class="btn btn-outline btn-sm" @click="exportPdf">Export PDF</button>
            </div>
          </div>

          <div v-if="loadingView" class="text-base-content/60">Loading signature data…</div>

          <template v-else>
            <!-- Validation tab: trust anchors, crypto integrity, timestamps -->
            <div v-if="activeTab === 'validation'" class="space-y-4">
              <div class="flex items-center gap-2">
                <button class="btn btn-sm btn-primary" :disabled="!canManage || busy" @click="runValidate">
                  <span v-if="busy" class="loading loading-xs loading-spinner" />
                  Validate
                </button>
                <span v-if="!canManage" class="text-xs text-base-content/50">Requires Contract Manager role.</span>
              </div>

              <div v-if="activeDss" class="card border border-base-content/10 bg-base-100">
                <div class="card-body p-4">
                  <div class="flex items-center gap-2">
                    <h3 class="font-semibold">EU DSS Validation (ETSI EN 319 102-1)</h3>
                    <span class="badge" :class="dssIndicator(activeDss.indication).cls">
                      {{ dssIndicator(activeDss.indication).label }}
                    </span>
                  </div>
                  <dl class="mt-2 grid grid-cols-1 gap-1 text-sm sm:grid-cols-2">
                    <div>
                      <dt class="inline font-medium">Signer identity:</dt>
                      <dd class="inline break-all">{{ activeDss.signed_by ?? '—' }}</dd>
                    </div>
                    <div>
                      <dt class="inline font-medium">Signature level:</dt>
                      <dd class="inline">{{ activeDss.signature_format ?? '—' }}</dd>
                    </div>
                    <div>
                      <dt class="inline font-medium">Timestamp:</dt>
                      <dd class="inline">{{ activeDss.signing_time ?? '—' }}</dd>
                    </div>
                    <div>
                      <dt class="inline font-medium">Sub-indication:</dt>
                      <dd class="inline">{{ activeDss.sub_indication ?? '—' }}</dd>
                    </div>
                  </dl>
                </div>
              </div>

              <div>
                <div class="mb-1 flex items-center gap-2">
                  <h3 class="font-semibold">Cryptographic Integrity</h3>
                  <span
                    v-if="integrityFindings.length"
                    class="badge"
                    :class="integrityIntact ? 'badge-success' : 'badge-error'"
                  >
                    {{ integrityIntact ? 'Intact' : 'Issues found' }}
                  </span>
                </div>
                <ul v-if="integrityFindings.length" class="space-y-1 text-sm">
                  <li v-for="(f, i) in integrityFindings" :key="i" class="flex items-start gap-2">
                    <span class="badge badge-sm" :class="findingIndicator(f).cls">{{ findingIndicator(f).label }}</span>
                    <span>{{ f }}</span>
                  </li>
                </ul>
                <p v-else class="text-sm text-base-content/60">No integrity findings recorded.</p>
              </div>
            </div>

            <!-- Revocation tab: revoke signatures if required -->
            <div v-else-if="activeTab === 'revocation'" class="overflow-x-auto">
              <table class="table w-full table-zebra">
                <thead>
                  <tr>
                    <th>Signer</th>
                    <th>Field</th>
                    <th>Status</th>
                    <th>Signed / Revoked</th>
                    <th>Action</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(sig, i) in view?.signatures ?? []" :key="i">
                    <td class="max-w-48 truncate font-mono text-xs">{{ sig.signer_did }}</td>
                    <td>{{ sig.field_name ?? '—' }}</td>
                    <td>
                      <span class="badge badge-sm" :class="statusIndicator(sig.status).cls">
                        {{ statusIndicator(sig.status).label }}
                      </span>
                    </td>
                    <td class="text-xs">{{ sig.revoked_at ?? sig.signed_at ?? '—' }}</td>
                    <td>
                      <button
                        class="btn btn-outline btn-xs btn-error"
                        :disabled="!canManage || busy || sig.status.toUpperCase() === 'REVOKED'"
                        @click="revoke(sig)"
                      >
                        Revoke
                      </button>
                    </td>
                  </tr>
                  <tr v-if="!view?.signatures?.length">
                    <td colspan="5" class="text-base-content/60">No signatures on this contract.</td>
                  </tr>
                </tbody>
              </table>
              <p v-if="!canManage" class="mt-2 text-xs text-base-content/50">
                Revocation requires the Contract Manager role.
              </p>
            </div>

            <!-- Compliance Checks tab: signature level (QES/AES), credential status, roles -->
            <div v-else-if="activeTab === 'compliance'" class="space-y-4">
              <div class="flex items-center gap-2">
                <button class="btn btn-sm btn-primary" :disabled="!canManage || busy" @click="runCompliance">
                  <span v-if="busy" class="loading loading-xs loading-spinner" />
                  Run Compliance
                </button>
                <span v-if="!canManage" class="text-xs text-base-content/50">Requires Contract Manager role.</span>
              </div>

              <div class="overflow-x-auto">
                <table class="table w-full table-zebra">
                  <thead>
                    <tr>
                      <th>Signer</th>
                      <th>Signature level</th>
                      <th>Credential status</th>
                      <th>Credential binding</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="(sig, i) in view?.signatures ?? []" :key="i">
                      <td class="max-w-48 truncate font-mono text-xs">{{ sig.signer_did }}</td>
                      <td>
                        <span class="badge badge-sm badge-info">{{ signatureLevel(sig) }}</span>
                      </td>
                      <td>
                        <span class="badge badge-sm" :class="statusIndicator(sig.status).cls">
                          {{ statusIndicator(sig.status).label }}
                        </span>
                      </td>
                      <td class="max-w-48 truncate font-mono text-[10px]" :title="sig.kb_sd_hash ?? ''">
                        {{ sig.kb_sd_hash ?? '—' }}
                      </td>
                    </tr>
                    <tr v-if="!view?.signatures?.length">
                      <td colspan="4" class="text-base-content/60">No signatures on this contract.</td>
                    </tr>
                  </tbody>
                </table>
              </div>

              <div v-if="complianceResult?.findings?.length">
                <h3 class="mb-1 font-semibold">Compliance Findings</h3>
                <ul class="space-y-1 text-sm">
                  <li v-for="(f, i) in complianceResult.findings" :key="i" class="flex items-start gap-2">
                    <span class="badge badge-sm" :class="findingIndicator(f).cls">{{ findingIndicator(f).label }}</span>
                    <span>{{ f }}</span>
                  </li>
                </ul>
              </div>
            </div>

            <!-- Audit Reports tab -->
            <div v-else class="space-y-3">
              <div class="flex items-center gap-2">
                <button class="btn btn-sm btn-primary" :disabled="!canAudit || busy" @click="loadAudit">
                  <span v-if="busy" class="loading loading-xs loading-spinner" />
                  Load Audit Report
                </button>
                <span v-if="!canAudit" class="text-xs text-base-content/50">
                  Requires Auditor or Compliance Officer role.
                </span>
              </div>

              <div v-if="auditEntries" class="overflow-x-auto">
                <table class="table w-full table-zebra">
                  <thead>
                    <tr>
                      <th>ID</th>
                      <th>Component</th>
                      <th>Event</th>
                      <th>Created</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="entry in auditEntries" :key="entry.id">
                      <td>{{ entry.id }}</td>
                      <td>{{ entry.component }}</td>
                      <td>{{ entry.event_type }}</td>
                      <td class="text-xs">{{ entry.created_at }}</td>
                    </tr>
                    <tr v-if="auditEntries.length === 0">
                      <td colspan="4" class="text-base-content/60">No audit entries.</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>
