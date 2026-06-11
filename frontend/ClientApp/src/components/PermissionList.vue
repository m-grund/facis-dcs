<script setup lang="ts">
import { computed, ref } from 'vue'
import { onClickOutside } from '@vueuse/core'
import { useAuthStore } from '@/stores/auth-store'

const authStore = useAuthStore()
const roles = computed(() => [...(authStore.user?.roles ?? [])].sort())

const isOpen = ref(false)
const panelRef = ref(null)

function formatRole(role: string): string {
  return role
    .split('_')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
    .join(' ')
}

onClickOutside(panelRef, () => {
  isOpen.value = false
})
</script>

<template>
  <div ref="panelRef" class="relative">
    <button class="btn gap-2 btn-outline btn-sm" @click="isOpen = !isOpen">User Details</button>

    <div
      v-if="isOpen"
      class="absolute right-0 z-50 mt-2 w-120 rounded-box border border-base-300 bg-base-100 p-4 shadow-lg"
    >
      <div class="mb-3 border-b border-base-300 pb-3">
        <p class="mb-2 text-xs font-bold text-base-content/50 uppercase">Issuer</p>
        <p class="text-xs break-all text-base-content/60">{{ authStore.user?.issuer }}</p>
      </div>

      <div class="mb-3 border-b border-base-300 pb-3">
        <p class="mb-2 text-xs font-bold text-base-content/50 uppercase">Holder</p>
        <p class="text-xs break-all text-base-content/60">{{ authStore.user?.holder }}</p>
      </div>

      <p class="mb-2 text-xs font-bold text-base-content/50 uppercase">Permissions</p>
      <ul class="flex flex-col gap-1">
        <li v-for="role in roles" :key="role" class="w-full justify-start">
          {{ formatRole(role) }}
        </li>
      </ul>
    </div>
  </div>
</template>
