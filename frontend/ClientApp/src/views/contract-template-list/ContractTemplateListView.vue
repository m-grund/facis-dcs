<template>
  <div class="flex justify-between p-4 mb-4">
    <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
      {{ $route.meta.name }}
    </h2>

    <RouterLink
      v-if="isTemplateCreator"
      :to="{ name: ROUTES.TEMPLATES.NEW }"
      class="btn rounded-box self-end btn-secondary gap-2"
      #default="{ route }"
    >
      {{ route.meta.name }}
    </RouterLink>
    <div v-else></div>
  </div>
  <div>
    <div v-if="loading">Lade Templates...</div>
    <div v-else-if="error">{{ error }}</div>
    <div v-else>
      <TemplateList :items="templates" :has-review-task="hasReviewTask" :has-approval-task="hasApprovalTask" />
    </div>
  </div>
</template>

<script setup lang="ts">
import TemplateList from '@/components/lists/template/TemplateList.vue'
import { ROUTES } from '@/router/router'
import { computed } from 'vue'
import { useTemplateList } from './ContractTemplateListController'

const { templates, roles, loading, error, hasReviewTask, hasApprovalTask } = useTemplateList()

const isTemplateCreator = computed(() => roles.value.includes('TEMPLATE_CREATOR'))
</script>
