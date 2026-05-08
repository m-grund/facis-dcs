<template>
  <div
    class="min-h-11 flex items-center select-none rounded-lg border border-base-300 bg-base-100 px-3 py-2 cursor-pointer hover:bg-base-200 transition-colors"
    role="button" tabindex="0" @click="$emit('select')" @keydown.enter.prevent="$emit('select')"
    @keydown.space.prevent="$emit('select')">
    <span class="text-sm font-medium">{{ label }}</span>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  label: string
}>()
</script>
