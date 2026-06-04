<script setup lang="ts">
import { contractStorageArchiveService } from '@/services/contract-storage-archive-service'
import { onMounted } from 'vue'

const loadArchive = async () => {
  const archive = await contractStorageArchiveService.retrieve()
  console.log(archive)
}

onMounted(loadArchive)
</script>

<template>
  <div class="mb-4 flex justify-between border-b border-base-content/10 bg-base-100 p-4">
    <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
      {{ $route.meta.name }}
    </h2>
    <button class="btn btn-primary">Store Contract in Archive</button>
  </div>
  <div class="m-5 text-xl">Test</div>
</template>
