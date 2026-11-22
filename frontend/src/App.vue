<script setup lang="ts">
import { onMounted } from 'vue'
import { useRoute } from 'vue-router'
import TopBar from './components/TopBar.vue'
import StatusBar from './components/StatusBar.vue'
import ActivityBar from './components/ActivityBar.vue'
import { connect, loadError } from './sim'
import { connectUpdates } from './update'
import { trackEvents } from './events'
import { panelKeys } from './panel'

const route = useRoute()

onMounted(() => {
  connect()
  connectUpdates()
  trackEvents()
  panelKeys()
})
</script>

<template>
  <p v-if="loadError" class="error">Could not load the station: {{ loadError }}</p>
  <!-- The frame: the activity bar along the left, the open project's bar on top, the page, and the status bar along the bottom. -->
  <div v-else class="flex h-full flex-col">
    <div class="flex min-h-0 flex-1">
      <ActivityBar />
      <div class="flex min-w-0 flex-1 flex-col">
        <TopBar v-if="route.meta.project" />
        <div class="grid min-h-0 flex-1 grid-rows-[minmax(0,1fr)]">
          <!-- The project's window is kept while the settings or the builder show, with its 3D view and panel as they were. -->
          <RouterView v-slot="{ Component }">
            <KeepAlive include="ProjectWindow"><component :is="Component" /></KeepAlive>
          </RouterView>
        </div>
      </div>
    </div>
    <StatusBar />
  </div>
</template>

<style scoped>
.error {
  color: var(--danger);
  padding: 24px;
}
</style>
