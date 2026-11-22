<script setup lang="ts">
import { onMounted } from 'vue'
import ContextPanel from './components/ContextPanel.vue'
import BuilderPage from './components/BuilderPage.vue'
import LibraryPage from './components/LibraryPage.vue'
import SettingsPage from './components/SettingsPage.vue'
import TopBar from './components/TopBar.vue'
import StationDock from './components/StationDock.vue'
import ProgramIde from './ide/ProgramIde.vue'
import ConnectionEditor from './components/ConnectionEditor.vue'
import ProjectsPage from './components/ProjectsPage.vue'
import StatusBar from './components/StatusBar.vue'
import ActivityBar from './components/ActivityBar.vue'
import StageAlerts from './components/StageAlerts.vue'
import { connect, editingConnection, loadError, page, programming, project, view } from './sim'
import { connectUpdates } from './update'
import StationViewer from './viewer/StationViewer.vue'

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
          <div v-show="page === 'simulation' && project && view === 'simulate'" class="page">
            <main class="stage">
              <div class="relative grid min-h-0">
                <StationViewer :active="page === 'simulation' && view === 'simulate'" />
                <StageAlerts />
              </div>
              <StationDock place="bottom" />
              <ProgramIde v-if="programming !== null" :plc="programming" @close="programming = null" />
              <ConnectionEditor v-if="editingConnection" :key="editingConnection.id" v-bind="editingConnection" @close="editingConnection = null" />
            </main>
            <ContextPanel />
          </div>
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
.page {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 320px;
  min-height: 0;
}

.stage {
  display: grid;
  grid-template-rows: minmax(0, 1fr) auto;
  min-width: 0;
}

.error {
  color: var(--danger);
  padding: 24px;
}
</style>
