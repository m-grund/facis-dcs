<template>
  <button type="button" v-bind="$attrs" @click="openModal">Submit</button>
  <Teleport to="body">
    <dialog ref="assigneeModal" class="modal modal-bottom transition-none sm:modal-middle" @close="clearAll">
      <div class="modal-box flex max-h-[85vh] w-full max-w-lg flex-col">
        <h3 class="text-lg font-bold">Assignees for Contract Submission</h3>
        <p class="py-2 text-sm opacity-80">Enter wallet DIDs</p>

        <div class="flex grow flex-col gap-5 overflow-y-auto py-2">
          <section class="flex flex-col gap-2">
            <span class="font-medium">Reviewers</span>
            <ul v-if="reviewers.length > 0" class="flex flex-col gap-1">
              <li
                v-for="did in reviewers"
                :key="did"
                class="flex items-center gap-2 rounded-lg border border-base-300 px-3 py-2"
              >
                <p class="min-w-0 flex-1 truncate font-mono text-xs" :title="did">{{ did }}</p>
                <button
                  type="button"
                  class="btn shrink-0 btn-ghost btn-xs"
                  aria-label="Remove"
                  @click="removeReviewer(did)"
                >
                  ✕
                </button>
              </li>
            </ul>
            <div class="flex flex-col gap-2">
              <input
                v-model="reviewerDraft"
                type="text"
                class="input-bordered input input-sm w-full font-mono text-xs"
                placeholder="did:jwk:..."
                @input="reviewerError = ''"
                @keydown.enter.prevent="addReviewer"
              />
              <button type="button" class="btn w-fit btn-sm btn-primary" @click="addReviewer">+</button>
            </div>
            <p v-if="reviewerError" class="text-xs text-error">{{ reviewerError }}</p>
          </section>

          <section class="flex flex-col gap-2">
            <span class="font-medium">Approvers</span>
            <ul v-if="approvers.length > 0" class="flex flex-col gap-1">
              <li
                v-for="did in approvers"
                :key="did"
                class="flex items-center gap-2 rounded-lg border border-base-300 px-3 py-2"
              >
                <p class="min-w-0 flex-1 truncate font-mono text-xs" :title="did">{{ did }}</p>
                <button
                  type="button"
                  class="btn shrink-0 btn-ghost btn-xs"
                  aria-label="Remove"
                  @click="removeApprover(did)"
                >
                  ✕
                </button>
              </li>
            </ul>
            <div class="flex flex-col gap-2">
              <input
                v-model="approverDraft"
                type="text"
                class="input-bordered input input-sm w-full font-mono text-xs"
                placeholder="did:jwk:..."
                @input="approverError = ''"
                @keydown.enter.prevent="addApprover"
              />
              <button type="button" class="btn w-fit btn-sm btn-primary" @click="addApprover">+</button>
            </div>
            <p v-if="approverError" class="text-xs text-error">{{ approverError }}</p>
          </section>

          <section class="flex flex-col gap-2">
            <span class="font-medium">Negotiators</span>
            <ul v-if="negotiators.length > 0" class="flex flex-col gap-1">
              <li
                v-for="did in negotiators"
                :key="did"
                class="flex items-center gap-2 rounded-lg border border-base-300 px-3 py-2"
              >
                <p class="min-w-0 flex-1 truncate font-mono text-xs" :title="did">{{ did }}</p>
                <button
                  type="button"
                  class="btn shrink-0 btn-ghost btn-xs"
                  aria-label="Remove"
                  @click="removeNegotiator(did)"
                >
                  ✕
                </button>
              </li>
            </ul>
            <div class="flex flex-col gap-2">
              <input
                v-model="negotiatorDraft"
                type="text"
                class="input-bordered input input-sm w-full font-mono text-xs"
                placeholder="did:jwk:..."
                @input="negotiatorError = ''"
                @keydown.enter.prevent="addNegotiator"
              />
              <button type="button" class="btn w-fit btn-sm btn-primary" @click="addNegotiator">+</button>
            </div>
            <p v-if="negotiatorError" class="text-xs text-error">{{ negotiatorError }}</p>
          </section>
        </div>

        <div class="modal-action mt-2">
          <button type="button" class="btn btn-outline" @click="onModalClose">Cancel</button>
          <button type="button" class="btn btn-primary" @click="onModalSubmit">Apply</button>
        </div>
      </div>
      <form method="dialog" class="modal-backdrop">
        <button type="submit" aria-label="Close">close</button>
      </form>
    </dialog>
  </Teleport>
</template>

<script setup lang="ts">
import { isDuplicateInList, mergeDraftIntoList } from '@/utils/submit-selection'
import type { SubmitContractAssignees } from '@/utils/submit-selection'
import { nextTick, ref, type Ref } from 'vue'

defineOptions({ inheritAttrs: false })

const emit = defineEmits<{
  submit: [value: SubmitContractAssignees]
}>()

const assigneeModal = ref<HTMLDialogElement | null>(null)

const reviewers = ref<string[]>([])
const approvers = ref<string[]>([])
const negotiators = ref<string[]>([])
const reviewerDraft = ref('')
const approverDraft = ref('')
const negotiatorDraft = ref('')

const reviewerError = ref('')
const approverError = ref('')
const negotiatorError = ref('')

function clearErrors() {
  reviewerError.value = ''
  approverError.value = ''
  negotiatorError.value = ''
}

function clearAll() {
  reviewers.value = []
  approvers.value = []
  negotiators.value = []
  reviewerDraft.value = ''
  approverDraft.value = ''
  negotiatorDraft.value = ''
  clearErrors()
}

async function openModal() {
  clearAll()
  await nextTick()
  assigneeModal.value?.showModal()
}

function addReviewer() {
  reviewerError.value = ''
  const trimmed = reviewerDraft.value.trim()
  if (!trimmed) return
  if (isDuplicateInList(trimmed, reviewers.value)) {
    reviewerError.value = 'This reviewer is already in the list.'
    return
  }
  reviewers.value.push(trimmed)
  reviewerDraft.value = ''
}

function addApprover() {
  approverError.value = ''
  const trimmed = approverDraft.value.trim()
  if (!trimmed) return
  if (isDuplicateInList(trimmed, approvers.value)) {
    approverError.value = 'This approver is already in the list.'
    return
  }
  approvers.value.push(trimmed)
  approverDraft.value = ''
}

function addNegotiator() {
  negotiatorError.value = ''
  const trimmed = negotiatorDraft.value.trim()
  if (!trimmed) return
  if (isDuplicateInList(trimmed, negotiators.value)) {
    negotiatorError.value = 'This negotiator is already in the list.'
    return
  }
  negotiators.value.push(trimmed)
  negotiatorDraft.value = ''
}

function removeFromList(list: Ref<string[]>, did: string) {
  list.value = list.value.filter((entry) => entry !== did)
}

function removeReviewer(did: string) {
  removeFromList(reviewers, did)
}

function removeApprover(did: string) {
  removeFromList(approvers, did)
}

function removeNegotiator(did: string) {
  removeFromList(negotiators, did)
}

function collectAssignees(): SubmitContractAssignees {
  return {
    reviewers: mergeDraftIntoList(reviewers.value, reviewerDraft.value),
    approvers: mergeDraftIntoList(approvers.value, approverDraft.value),
    negotiators: mergeDraftIntoList(negotiators.value, negotiatorDraft.value),
  }
}

function validateBeforeSubmit(): boolean {
  clearErrors()
  const { reviewers: finalReviewers, approvers: finalApprovers, negotiators: finalNegotiators } = collectAssignees()
  let valid = true

  if (finalReviewers.length === 0) {
    reviewerError.value = 'Add at least one reviewer.'
    valid = false
  }
  if (finalApprovers.length === 0) {
    approverError.value = 'Add at least one approver.'
    valid = false
  }
  if (finalNegotiators.length === 0) {
    negotiatorError.value = 'Add at least one negotiator.'
    valid = false
  }

  return valid
}

function onModalSubmit() {
  if (!validateBeforeSubmit()) return
  emit('submit', collectAssignees())
  assigneeModal.value?.close()
}

function onModalClose() {
  assigneeModal.value?.close()
}
</script>
