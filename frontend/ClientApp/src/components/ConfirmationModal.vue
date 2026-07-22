<script setup lang="ts">
import { useConfirmDialog } from '@vueuse/core'
import { computed, type Ref, ref, useTemplateRef, watch } from 'vue'

interface Editor {
  requiredText: boolean
  placeholder?: string
}

interface ModalData {
  message: string
  editor?: Editor
}

interface ConfirmData {
  isCanceled: boolean
  data?: string
}

const actionModal = useTemplateRef('action-modal')
const modalData: Ref<ModalData> = ref({ message: 'Confirm selection' })
const dialogTitleId = 'confirmation-modal-title'
const dialogDescriptionId = 'confirmation-modal-description'
const editorLabelId = 'confirmation-modal-editor-label'

const inputText = ref('')

const hasEditor = computed(() => !!modalData.value.editor)

const inputRequired = computed(() => !!modalData.value.editor?.requiredText && !inputText.value.trim())

const { isRevealed, reveal, confirm, cancel, onReveal } = useConfirmDialog<ModalData, string | undefined>()

onReveal((data) => {
  if (data) {
    modalData.value = data
  }
})

watch(isRevealed, (value) => {
  if (value) {
    inputText.value = ''
    actionModal.value?.showModal()
    focusFirstControl()
  } else {
    actionModal.value?.close()
  }
})

function focusFirstControl() {
  window.requestAnimationFrame(() => {
    const dialog = actionModal.value
    if (!dialog) return

    const firstControl = dialog.querySelector<HTMLElement>('button:not([disabled]), textarea')

    if (firstControl) {
      firstControl.focus()
    } else {
      dialog.focus()
    }
  })
}

const handleConfirm = () => {
  if (hasEditor.value) {
    if (inputRequired.value) return
    confirm(inputText.value)
  } else {
    confirm()
  }
}

interface ModalExpose {
  reveal: (data: ModalData) => Promise<ConfirmData>
}

defineExpose<ModalExpose>({ reveal: reveal })
</script>

<template>
  <dialog
    ref="action-modal"
    class="modal modal-bottom sm:modal-middle"
    role="dialog"
    aria-modal="true"
    :aria-labelledby="dialogTitleId"
    :aria-describedby="dialogDescriptionId"
    @close="cancel"
  >
    <div class="modal-box">
      <h3 :id="dialogTitleId" class="text-lg font-bold">Confirmation</h3>
      <p :id="dialogDescriptionId" class="text-md py-4">{{ modalData.message }}</p>
      <div v-if="modalData.editor" class="mx-auto flex w-full max-w-4xl flex-col gap-2 py-3">
        <label :id="editorLabelId" class="sr-only" for="confirmation-text-input">Comment</label>
        <textarea
          id="confirmation-text-input"
          v-model="inputText"
          class="textarea mt-0.5 min-h-10 w-full resize-y rounded-lg border textarea-ghost border-base-300/50 text-sm textarea-sm"
          :placeholder="modalData.editor.placeholder ?? 'Comment'"
          :aria-invalid="inputRequired"
          :aria-describedby="inputRequired ? 'confirmation-input-help' : undefined"
          rows="4"
        />
        <p v-if="inputRequired" id="confirmation-input-help" class="text-xs text-error">
          A comment is required before submitting.
        </p>
      </div>
      <div class="modal-action flex-col" :class="{ 'flex-row-reverse justify-start': hasEditor }">
        <button
          type="button"
          class="btn btn-sm btn-primary"
          :class="{ 'btn-disabled': inputRequired }"
          :disabled="inputRequired"
          @click="handleConfirm"
        >
          {{ hasEditor ? 'Submit' : 'Confirm' }}
        </button>
        <button type="button" class="btn btn-outline btn-sm" @click="cancel">Cancel</button>
      </div>
    </div>
    <div v-if="!hasEditor" class="modal-backdrop" @click="cancel"></div>
  </dialog>
</template>
