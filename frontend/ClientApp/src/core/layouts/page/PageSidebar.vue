<template>
  <div class="flex items-center justify-center h-16 px-4 overflow-hidden border-b border-base-content/10">
    <RouterLink :to="{ name: ROUTES.HOME }" class="font-bold text-2xl tracking-tight text-base-content uppercase">
      <img :src="facisLogo" alt="Home" class="h-10" />
    </RouterLink>
  </div>

  <nav class="overflow-y-auto overflow-x-hidden py-4">
    <ul class="menu px-3 gap-1 w-full text-base-content">
      <li v-for="route in navigationRoutes" :key="route.path">
        <RouterLink :to="route.path" @click="closeMobileDrawer" :class="[
          'flex items-center gap-4 py-3 rounded-btn',
          isSidebarCollapsed ? 'justify-center px-0' : 'px-4'
        ]" active-class="active bg-primary text-primary-content" :data-tip="isSidebarCollapsed ? route.meta?.name : ''">
          <component :is="route.meta?.icon" class="w-6 h-6 shrink-0" aria-hidden="true" />
          <span v-if="!isSidebarCollapsed" class="font-medium whitespace-nowrap">
            {{ route.meta?.name }}
          </span>
        </RouterLink>
      </li>
    </ul>
  </nav>
</template>

<script setup lang="ts">
import facisLogo from '@/assets/FACIS_color.svg'
import { ROUTES } from '@/router/router'
import { useAuthStore } from '@/stores/auth-store'
import { usePageStore } from '@core/store/page'
import { storeToRefs } from 'pinia'
import { computed } from 'vue'
import { RouterLink, useRouter } from 'vue-router'

const router = useRouter()

const pageStore = usePageStore()
const { isSidebarCollapsed } = storeToRefs(pageStore)

const authStore = useAuthStore()
const { user } = storeToRefs(authStore)

const closeMobileDrawer = () => {
  const drawerToggle = document.getElementById(pageStore.pageSidebarId) as HTMLInputElement | null
  if (drawerToggle) drawerToggle.checked = false
}

const navigationRoutes = computed(() => {
  try {
    return router.getRoutes()
      .filter(route =>
        route.name &&
        !route.path.includes(':') &&
        route.meta?.name &&
        route.meta?.hideInSidebar !== true &&
        (!route.meta.roles || user.value?.roles?.some(role => route.meta.roles?.includes(role)))
      )
      .sort((routeA, routeB) => (routeA.meta.order || 999) - (routeB.meta.order || 999))
  } catch (e) {
    return []
  }
})
</script>
