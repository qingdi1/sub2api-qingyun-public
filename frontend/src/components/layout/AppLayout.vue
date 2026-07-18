<template>
  <div class="ui-theme-shell min-h-screen bg-gray-50 dark:bg-dark-950">
    <!-- Background Decorations -->
    <div class="pointer-events-none fixed inset-0 bg-mesh-gradient"></div>
    <ThemePointerTrail />

    <!-- Sidebar -->
    <AppSidebar />

    <!-- Main Content Area -->
    <div
      class="relative min-h-screen transition-all duration-300"
      :class="[sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64']"
    >
      <!-- Header -->
      <AppHeader />

      <div
        v-if="authStore.isDemo"
        role="status"
        class="border-b border-amber-200 bg-amber-50 px-4 py-2 text-center text-sm font-medium text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/40 dark:text-amber-200"
      >
        演示模式 · 当前数据为模拟数据，不会保存
      </div>

      <!-- Main Content -->
      <main class="p-4 md:p-6 lg:p-8">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import { applyUiStyle, readStoredUiStyle } from '@/themes/catalog'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'
import ThemePointerTrail from '@/components/theme/ThemePointerTrail.vue'

const appStore = useAppStore()
const authStore = useAuthStore()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')

const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  applyUiStyle(readStoredUiStyle())
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>
