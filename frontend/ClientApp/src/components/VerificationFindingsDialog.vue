<script setup lang="ts">
import { useDcsDraftStore } from '@/modules/template-repository/store/dcsDraftStore'
import { contractTemplateService } from '@/services/contract-template-service'
import { nextTick, ref } from 'vue'

const draftStore = useDcsDraftStore()

const findingsModal = ref<HTMLDialogElement | null>(null)

const findings = ref<string[]>([])

const error = ref('')

const isSubmitting = ref(false)

const verifyTemplate = async () => {
  const did = draftStore.did
  if (!did) {
    console.error('Missing did for verification')
    return
  }
  isSubmitting.value = true
  try {
    const verificationResult = await contractTemplateService.verify({
      did,
    })

    findings.value = verificationResult.findings
  } catch (err) {
    console.error('Submission failed', err)
  } finally {
    isSubmitting.value = false
  }
}

function clearAll() {
  error.value = ''
  findings.value = []
}

async function openModal() {
  clearAll()
  await verifyTemplate()
  await nextTick()
  findingsModal.value?.showModal()
}

function onModalClose() {
  findingsModal.value?.close()
}
</script>

<template>
  <button type="button" v-bind="$attrs" @click="openModal">
    <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
    Verify
  </button>

  <Teleport to="body">
    <dialog ref="findingsModal" class="modal modal-bottom transition-none sm:modal-middle" @close="clearAll">
      <div class="modal-box flex max-h-[85vh] w-full max-w-lg flex-col">
        <h3 class="text-lg font-bold">Verification Findings</h3>
        <p v-if="error" class="mb-5 text-xs text-error">{{ error }}</p>

        <div v-if="findings.length > 0" class="mt-4 mb-4">
          <div
            v-for="(finding, idx) in findings"
            :key="idx"
            class="card mb-2 border border-base-300 bg-base-100 shadow-sm"
          >
            <div class="card-title p-3 text-sm">{{ finding }}</div>
          </div>
        </div>

        <div class="modal-action mt-2">
          <button type="button" class="btn btn-outline" @click="onModalClose">Close</button>
        </div>
      </div>
    </dialog>
  </Teleport>
</template>
