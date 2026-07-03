<script setup lang="ts">
import TemplateList from '@/components/lists/template/TemplateList.vue'
import { ROUTES } from '@/router/router'
import { useAuthStore } from '@/stores/auth-store'
import { computed } from 'vue'

const authStore = useAuthStore()

const isTemplateCreator = computed(() => authStore.user?.roles?.includes('TEMPLATE_CREATOR') ?? false)
</script>

<template>
  <div class="mb-4 flex justify-between border-b border-base-content/10 bg-base-100 p-4">
    <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
      {{ $route.meta.name }}
    </h2>

    <RouterLink
      v-if="isTemplateCreator"
      v-slot="{ route }"
      :to="{ name: ROUTES.TEMPLATES.NEW }"
      class="btn gap-2 self-end btn-primary"
    >
      {{ route.meta.name }}
    </RouterLink>
    <div v-else></div>
  </div>

  <TemplateList />
</template>
