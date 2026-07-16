<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useErrorStore } from '@/stores/error-store'

const errorStore = useErrorStore()
const { errors } = storeToRefs(errorStore)
</script>

<template>
  <div class="toast">
    <div
      v-for="error in errors"
      :key="error.id"
      :class="['alert', `alert-${error.type}`, 'max-h-40 max-w-md overflow-y-auto text-wrap shadow-lg']"
      :aria-live="error.type === 'error' ? 'assertive' : 'polite'"
      aria-atomic="true"
      :role="error.type === 'error' ? 'alert' : 'status'"
    >
      <span>{{ error.message }}</span>
    </div>
  </div>
</template>
