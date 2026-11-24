<script setup lang="ts">
import { defineAsyncComponent, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { programming, simulation } from '../sim'
import { detailsWidth as details, treeWidth as tree } from '../layout'
import UiSash from '../ui/UiSash.vue'
import StationViewer from '../viewer/StationViewer.vue'
import PagePanel from './PagePanel.vue'
import EventsPanel from './EventsPanel.vue'
import { events } from '../events'
import ContextPanel from './ContextPanel.vue'
import StageAlerts from './StageAlerts.vue'
import SimulationSide from './SimulationSide.vue'

// The programming tool, with its code editor, loads when a PLC's program is first opened.
const ProgramIde = defineAsyncComponent(() => import('../ide/ProgramIde.vue'))

// The simulation of an open project as it runs, laid out as VS Code lays out a folder: the
// station's PLCs and controls on the left, the 3D view in the middle with the panel below it,
// and the module selected on the right. Each edge
// between them can be dragged, and the sizes are kept. The program and a connection's editor
// take the middle while they are open.
const route = useRoute()
const active = () => route.name === 'simulate'

// Delete or Backspace takes the picked workpiece off the station, unless a field is being typed
// in. ⌘J or Ctrl+J for the panel is the app's (panel.ts).
function key(e: KeyboardEvent) {
  if (!active()) return
  const typing = e.target instanceof HTMLElement && (e.target.isContentEditable || /^(input|textarea|select)$/i.test(e.target.tagName))
  if ((e.key === 'Delete' || e.key === 'Backspace') && !typing && simulation.workpiece.value !== null) {
    e.preventDefault()
    void simulation.takeWorkpiece()
  }
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
      <PagePanel :tabs="[{ value: 'events', label: 'Events', count: events.filter((e) => e.kind !== 'restart').length }]" :covered="programming !== null">
        <template #events><EventsPanel /></template>
      </PagePanel>
      <ProgramIde v-if="programming !== null" :plc="programming" @close="programming = null" />
    </div>

    <div class="relative shrink-0 border-l border-line" :style="{ width: `${details}px` }">
      <UiSash v-model="details" side="right" :min="260" :max="560" label="Width of the details" />
      <ContextPanel />
    </div>
  </div>
</template>
