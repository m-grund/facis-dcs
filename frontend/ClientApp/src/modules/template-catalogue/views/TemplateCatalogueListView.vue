<script setup lang="ts">
import { onMounted } from 'vue'
import TemplateCatalogueList from '@/modules/template-catalogue/components/catalogue-template/TemplateCatalogueList.vue'
import { useTemplateCatalogueList } from '@/modules/template-catalogue/composables/useTemplateCatalogueList'

const { templates, loading, error, refresh } = useTemplateCatalogueList()

onMounted(() => {
  void refresh()
})
</script>

<template>
  <div class="mb-4 flex justify-between border-b border-base-content/10 bg-base-100 p-4">
    <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
      {{ $route.meta.name }}
    </h2>
    <div />
  </div>

  <div>
    <div v-if="loading" class="pl-4">Loading Templates...</div>
    <div v-else-if="error" class="pl-4">{{ error }}</div>
    <div v-else>
      <TemplateCatalogueList :templates="templates" />
    </div>
  </div>
</template>
