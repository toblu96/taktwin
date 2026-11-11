<script setup lang="ts">
import { onMounted } from 'vue'
import ContextPanel from './components/ContextPanel.vue'
import BuilderPage from './components/BuilderPage.vue'
import LibraryPage from './components/LibraryPage.vue'
import SettingsPage from './components/SettingsPage.vue'
import TopBar from './components/TopBar.vue'
import StationDock from './components/StationDock.vue'
import ProgramIde from './ide/ProgramIde.vue'
import { connect, loadError, page, programming } from './sim'
import StationViewer from './viewer/StationViewer.vue'

onMounted(connect)
</script>

<template>
  <p v-if="loadError" class="error">Could not load the station: {{ loadError }}</p>
  <div v-else class="app">
    <TopBar />
    <!-- The simulation stays loaded and keeps running behind the library. -->
    <div v-show="page === 'simulation'" class="page">
      <main class="stage">
        <StationViewer :active="page === 'simulation'" />
        <StationDock place="bottom" />
        <ProgramIde v-if="programming !== null" :plc="programming" @close="programming = null" />
      </main>
      <ContextPanel />
    </div>
    <LibraryPage v-if="page === 'library'" />
    <BuilderPage v-if="page === 'builder'" />
    <SettingsPage v-if="page === 'settings'" />
  </div>
</template>

<style scoped>
.app {
  display: grid;
  grid-template-rows: 46px minmax(0, 1fr);
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
