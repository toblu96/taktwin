<script setup lang="ts">
import { onMounted, onUnmounted, watch } from 'vue'
import { panelOpen, panelTab } from '../project-view'
import { useRoute } from 'vue-router'
import { editingConnection, programming, project } from '../sim'
import { detailsWidth as details, panelHeight as panel, treeWidth as tree } from '../layout'
import UiSash from '../ui/UiSash.vue'
import ProgramIde from '../ide/ProgramIde.vue'
import StationViewer from '../viewer/StationViewer.vue'
import BottomPanel from './BottomPanel.vue'
import ConnectionEditor from './ConnectionEditor.vue'
import ContextPanel from './ContextPanel.vue'
import StageAlerts from './StageAlerts.vue'
import SimulationSide from './SimulationSide.vue'

// The simulation of an open project as it runs, laid out as VS Code lays out a folder: the
// station's PLCs and controls on the left, the 3D view in the middle with the panel below it,
// and the module selected on the right. Each edge
// between them can be dragged, and the sizes are kept. The program and a connection's editor
// take the middle while they are open.
const route = useRoute()
const active = () => route.name === 'simulate'

// A project opens on its signals.
watch(
  () => project.value?.name,
  () => (panelTab.value = 'io'),
  { immediate: true },
)

// ⌘J or Ctrl+J shows and hides the panel, as in VS Code.
function key(e: KeyboardEvent) {
  if (!active() || !(e.metaKey || e.ctrlKey) || e.key.toLowerCase() !== 'j') return
  e.preventDefault()
  panelOpen.value = !panelOpen.value
}
onMounted(() => window.addEventListener('keydown', key))
onUnmounted(() => window.removeEventListener('keydown', key))
</script>

<template>
  <div class="flex h-full min-h-0">
    <div class="relative shrink-0 border-r border-line" :style="{ width: `${tree}px` }">
      <SimulationSide />
      <UiSash v-model="tree" side="left" :min="180" :max="480" label="Width of the station's side bar" />
    </div>

    <div class="relative flex min-w-0 flex-1 flex-col">
      <div class="relative grid min-h-0 flex-1">
        <StationViewer :active="active()" />
        <StageAlerts />
      </div>
      <div v-if="panelOpen" class="relative max-h-[70%] shrink-0 border-t border-line" :style="{ height: `${panel}px` }">
        <UiSash v-if="programming === null && !editingConnection" v-model="panel" side="bottom" :min="120" :max="720" label="Height of the panel" />
        <BottomPanel />
      </div>
      <ProgramIde v-if="programming !== null" :plc="programming" @close="programming = null" />
      <ConnectionEditor v-if="editingConnection" :key="editingConnection.id" v-bind="editingConnection" @close="editingConnection = null" />
    </div>

    <div class="relative shrink-0 border-l border-line" :style="{ width: `${details}px` }">
      <UiSash v-model="details" side="right" :min="260" :max="560" label="Width of the details" />
      <ContextPanel />
    </div>
  </div>
</template>
