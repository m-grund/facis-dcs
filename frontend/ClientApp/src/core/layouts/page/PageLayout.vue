<template>
  <div :class="[drawerClasses.root, 'min-h-screen']">
    <input :id="pageSidebarId" type="checkbox" class="drawer-toggle" />
    <div :class="[drawerClasses.header, 'flex flex-col h-screen overflow-hidden bg-base-100']">
      <!-- Navbar -->
      <header class="navbar w-full bg-base-200 border-b border-base-content/10 sticky top-0 z-30">
        <slot name="navbar">
          <PageNavBar />
        </slot>
      </header>

      <!-- Main Content -->
      <main class="grow overflow-y-auto" ref="scroll-container">
        <slot>
          <RouterView />
        </slot>
      </main>
    </div>

    <!-- Sidebar -->
    <div :class="[drawerClasses.sidebar, 'z-40']">
      <label :for="pageSidebarId" aria-label="close sidebar" class="drawer-overlay"></label>
      <aside
        :class="[
          'flex flex-col min-h-full bg-base-200 border-r border-base-content/5 transition-all duration-300 ease-in-out',
          isSidebarCollapsed ? 'lg:w-20' : 'w-72',
        ]"
      >
        <slot name="sidebar">
          <PageSidebar />
        </slot>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import PageNavBar from '@/core/layouts/page/PageNavBar.vue'
import PageSidebar from '@/core/layouts/page/PageSidebar.vue'
import { useScrollStore } from '@/core/store/scroll'
import { usePageStore } from '@core/store/page'
import { storeToRefs } from 'pinia'
import { onMounted, useTemplateRef } from 'vue'
import { RouterView } from 'vue-router'

const scrollContainer = useTemplateRef<HTMLElement>('scroll-container')

const pageStore = usePageStore()
const { isSidebarCollapsed, pageSidebarId } = storeToRefs(pageStore)
const scrollStore = useScrollStore()

// Functional classes for DaisyUI drawer behavior (structure/toggle), not layout or styling
const drawerClasses = {
  root: ['drawer', 'lg:drawer-open'],
  header: ['drawer-content'],
  sidebar: ['drawer-side'],
}

onMounted(() => {
  scrollStore.scrollContainer = scrollContainer.value
})
</script>
