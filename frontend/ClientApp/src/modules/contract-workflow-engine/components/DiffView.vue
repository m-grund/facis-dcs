<template>
  <div class="space-y-3">
    <!-- toolbar -->
    <div class="flex items-center justify-between rounded-md border border-base-300 bg-base-100 px-3 py-2">
      <div class="flex items-center gap-6">
        <label
          for="line-number-toggle"
          class="flex items-center gap-3 text-sm text-base-content/80"
        >
          <span class="select-none cursor-pointer">Line numbers</span>
          <input
            id="line-number-toggle"
            v-model="showLineNumbers"
            type="checkbox"
            class="checkbox mt-1"
          />
        </label>
        <label
          for="diff-highlight-toggle"
          class="flex items-center gap-3 text-sm text-base-content/80"
        >
          <span class="select-none cursor-pointer">Highlight changes</span>
          <input
            id="diff-highlight-toggle"
            v-model="highlightDiff"
            type="checkbox"
            class="checkbox mt-1"
          />
        </label>
      </div>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 min-h-[32rem]">
      <DiffPane
        title="Prior Version"
        :blocks="priorBlocks"
        :diff-rows="contractDiffDocument.leftRows"
        :highlight-diff="highlightDiff"
        :show-no-prior-version="!hasPriorContractData"
        :show-line-numbers="showLineNumbers"
      />
      <DiffPane
        title="Current Version"
        :blocks="currentBlocks"
        :diff-rows="contractDiffDocument.rightRows"
        :highlight-diff="highlightDiff"
        :show-line-numbers="showLineNumbers"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ContractData } from '@/models/contract-data'
import {
  type ContractPlainTextBlock,
  useContractPlainTextConverter
} from '@/modules/contract-workflow-engine/composables/useContractPlainTextConverter'
import { useContractBlockDiff } from '@/modules/contract-workflow-engine/composables/useContractBlockDiff'
import DiffPane from '@/modules/contract-workflow-engine/components/diff-view/DiffPane.vue'
import { computed, ref } from 'vue'

const props = defineProps<{
  priorContractData?: ContractData
  currentContractData?: ContractData
}>()

const { convertContractToPlainTextBlocks } = useContractPlainTextConverter()
const { buildContractBlockDiff } = useContractBlockDiff()

// toolbar state
const showLineNumbers = ref(true)
const highlightDiff = ref(false)

const hasPriorContractData = computed(() => !!props.priorContractData)

const priorBlocks = computed<ContractPlainTextBlock[]>(() => {
  if (!props.priorContractData) return []
  return convertContractToPlainTextBlocks(props.priorContractData)
})

const currentBlocks = computed<ContractPlainTextBlock[]>(() => {
  if (!props.currentContractData) return []
  return convertContractToPlainTextBlocks(props.currentContractData)
})

const contractDiffDocument = computed(() => buildContractBlockDiff(priorBlocks.value, currentBlocks.value))

</script>