<script setup lang="ts">
import { onMounted } from 'vue'
import BuilderPage from './components/BuilderPage.vue'
import LibraryPage from './components/LibraryPage.vue'
import SettingsPage from './components/SettingsPage.vue'
import TopBar from './components/TopBar.vue'
import ProjectsPage from './components/ProjectsPage.vue'
import ProjectWindow from './components/ProjectWindow.vue'
import StatusBar from './components/StatusBar.vue'
import ActivityBar from './components/ActivityBar.vue'
import { connect, loadError, page, project, view } from './sim'
import { connectUpdates } from './update'

onMounted(() => {
  connect()
  connectUpdates()
})
</script>

<template>
  <p v-if="loadError" class="error">Could not load the station: {{ loadError }}</p>
  <!-- The frame: the activity bar along the left, the open project's bar on top, the page, and the status bar along the bottom. -->
  <div v-else class="flex h-full flex-col">
    <div class="flex min-h-0 flex-1">
      <ActivityBar />
      <div class="flex min-w-0 flex-1 flex-col">
        <TopBar v-if="page === 'simulation'" />
        <div class="grid min-h-0 flex-1 grid-rows-[minmax(0,1fr)]">
          <ProjectsPage v-if="page === 'simulation' && !project" />
          <!-- The simulation stays loaded and keeps running behind the settings and the builder. -->
          <ProjectWindow v-show="page === 'simulation' && project && view === 'simulate'" />
          <BuilderPage v-if="page === 'simulation' && project && view === 'build'" />
          <LibraryPage v-if="page === 'library'" />
          <SettingsPage v-if="page === 'settings'" />
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
