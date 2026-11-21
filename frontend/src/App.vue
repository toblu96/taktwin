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
  <div v-else class="app">
    <TopBar />
    <ProjectsPage v-if="page === 'simulation' && !project" />
    <!-- The simulation stays loaded and keeps running behind the settings and the builder. -->
    <div v-show="page === 'simulation' && project && view === 'simulate'" class="page">
      <main class="stage">
        <StationViewer :active="page === 'simulation' && view === 'simulate'" />
        <StationDock place="bottom" />
        <ProgramIde v-if="programming !== null" :plc="programming" @close="programming = null" />
        <ConnectionEditor v-if="editingConnection" :key="editingConnection.id" v-bind="editingConnection" @close="editingConnection = null" />
      </main>
      <ContextPanel />
    </div>
    <BuilderPage v-if="page === 'simulation' && project && view === 'build'" />
    <LibraryPage v-if="page === 'library'" />
    <SettingsPage v-if="page === 'settings'" />
    <StatusBar />
  </div>
</template>

<style scoped>
.app {
  display: grid;
  grid-template-rows: 46px minmax(0, 1fr) 26px;
  height: 100%;
}

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
