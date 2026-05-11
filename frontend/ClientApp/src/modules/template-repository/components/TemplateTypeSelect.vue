<template>
  <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
    <button type="button"
      class="text-left rounded-2xl border-2 p-8 min-h-50 flex flex-col justify-center transition-all focus:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2"
      :class="modelValue === TemplateType.frameContract
        ? 'border-primary bg-primary/10 shadow-lg'
        : 'border-base-300 hover:border-primary/50 hover:bg-base-200/50'"
      @click="emit('update:modelValue', TemplateType.frameContract)">
      <h3 class="text-xl font-bold text-base-content mb-3">Frame Contract</h3>
      <p class="text-sm text-base-content/70 leading-relaxed">
        All contracts are created from a frame contract. A frame contract is composed of one or more sub contracts.
      </p>
    </button>
    <button type="button"
      class="text-left rounded-2xl border-2 p-8 min-h-50 flex flex-col justify-center transition-all focus:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2"
      :class="modelValue === TemplateType.subContract
        ? 'border-primary bg-primary/10 shadow-lg'
        : 'border-base-300 hover:border-primary/50 hover:bg-base-200/50'"
      @click="emit('update:modelValue', TemplateType.subContract)">
      <h3 class="text-xl font-bold text-base-content mb-3">Sub Contract</h3>
      <p class="text-sm text-base-content/70 leading-relaxed">
        Used by frame contracts. Define the reusable partial contracts that frame contracts can reference and compose.
      </p>
    </button>
  </div>
</template>

<script setup lang="ts">
import { TemplateType, type TemplateTypeValue } from '@/modules/template-repository/models/contract-template'

defineProps<{
  modelValue: TemplateTypeValue
}>()

const emit = defineEmits<{
  'update:modelValue': [value: TemplateTypeValue]
}>()
</script>
