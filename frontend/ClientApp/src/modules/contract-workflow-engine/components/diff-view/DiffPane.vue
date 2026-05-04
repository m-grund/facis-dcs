<template>
  <div class="card bg-base-100 border border-base-300 shadow-sm h-full min-h-0">
    <div class="card-body p-4 min-h-0">
      <div class="font-semibold text-sm text-base-content/70 mb-2">
        {{ title }}
      </div>
      <div class="overflow-y-auto min-h-0 flex-1">
        <div
          v-if="showNoPriorVersion"
          class="h-full flex items-center justify-center text-base-content/50"
        >
          no prior version
        </div>
        <template v-else>
          <div
            v-for="(block, index) in blocks"
            :key="`${block.type}-${index}`"
            class="flex items-start"
            :class="getRowBackgroundClass(index + 1)"
          >
            <div
              v-if="showLineNumbers"
              class="relative w-12 shrink-0 pr-2 mr-4 pt-0 text-right text-base leading-6 text-base-content/40 border-r border-base-300/60 select-none"
            >
              <span class="block">{{ index + 1 }}</span>
            </div>
            <div class="min-w-0 flex-1">
              <DiffSectionBlock
                v-if="isSectionPlainTextBlock(block)"
                :block="block"
                :segments="getSegmentsByLine(index + 1)"
                :highlight-segments="shouldHighlightSegments(index + 1)"
              />
              <DiffTextBlock
                v-else
                :block="block"
                :segments="getSegmentsByLine(index + 1)"
                :highlight-segments="shouldHighlightSegments(index + 1)"
              />
            </div>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  type ContractDiffRow,
  type DiffType,
  type TextDiffSegment
} from '@/modules/contract-workflow-engine/composables/useContractBlockDiff'
import {
  isSectionPlainTextBlock,
  type ContractPlainTextBlock
} from '@/modules/contract-workflow-engine/composables/useContractPlainTextConverter'
import { computed } from 'vue'
import DiffSectionBlock from './DiffSectionBlock.vue'
import DiffTextBlock from './DiffTextBlock.vue'

const props = withDefaults(defineProps<{
  title: string
  blocks: ContractPlainTextBlock[]
  diffRows?: ContractDiffRow[]
  highlightDiff?: boolean
  showNoPriorVersion?: boolean
  showLineNumbers?: boolean
}>(), {
  diffRows: () => [],
  highlightDiff: false,
  showNoPriorVersion: false,
  showLineNumbers: true
})

const diffRowByLine = computed(() => {
  return new Map(props.diffRows.map((row) => [row.lineNumber, row]))
})

function getDiffTypeByLine(lineNumber: number): DiffType | null {
  if (!props.highlightDiff) return null
  return diffRowByLine.value.get(lineNumber)?.type ?? null
}

function getSegmentsByLine(lineNumber: number): TextDiffSegment[] | undefined {
  return diffRowByLine.value.get(lineNumber)?.segments
}

function shouldHighlightSegments(lineNumber: number): boolean {
  if (!props.highlightDiff) return false
  return getDiffTypeByLine(lineNumber) === 'modified'
}

function getRowBackgroundClass(lineNumber: number): string {
  const diffType = getDiffTypeByLine(lineNumber)
  if (diffType === 'added') return 'bg-green-100/70'
  if (diffType === 'removed') return 'bg-red-100/70'
  if (diffType === 'modified') return 'bg-amber-100/20'
  return ''
}

</script>
