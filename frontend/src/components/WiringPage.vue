<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { enter, leave, loadWiring, problem, state, wiring } from '../builder'
import { treeWidth, wiringDetailsWidth } from '../layout'
import { editingConnection, simulation } from '../sim'
import UiSash from '../ui/UiSash.vue'
import { loadStationWiring, stationWiring, stationWiringProblem, wiringIndex } from '../wiring'
import ConnectionEditor from './ConnectionEditor.vue'
import PagePanel from './PagePanel.vue'
import ProblemsPanel from './ProblemsPanel.vue'
import WiringDetails from './WiringDetails.vue'
import WiringDiagram from './WiringDiagram.vue'
import WiringSide from './WiringSide.vue'

// The wiring view of an open project: the station's PLCs and I/O terminals, the cables between
// them as a diagram, and what is clicked beside it, such as a PLC's connection or a terminal's
// pins. It runs and is edited alike: as it runs it shows the wiring of the station as it runs,
// read only, though a PLC can be connected; edited, in expert mode, it builds on the project's
// own station, and what is wrong shows in the panel below. A PLC's bindings to its controller
// take the diagram's place while they are edited.
const route = useRoute()
const editable = computed(() => !!route.meta.edit)

const shown = computed(() => (editable.value ? wiring.value : stationWiring.value))
const trouble = computed(() => (editable.value ? problem.value : stationWiringProblem.value))
const checks = computed(() => state.value?.scene.checks ?? [])

// Editing opens the builder on the project's station, and running leaves it again.
let entered = false
async function start(edit: boolean) {
  if (edit) {
    entered = true
    await enter()
    await loadWiring()
    return
  }
  if (entered) leave()
  entered = false
  await loadStationWiring()
}
onMounted(() => void start(editable.value))
watch(editable, (edit) => void start(edit))
onBeforeUnmount(() => {
  if (entered) leave()
})
// Each change while editing shows at once; as it runs, a station loaded again shows anew.
watch(state, () => editable.value && void loadWiring())
watch(simulation.scene, () => !editable.value && void loadStationWiring())

// What is shown beside the diagram: the box clicked, or the first PLC.
const selected = ref<string | null>(null)
const boxes = computed(() => wiringIndex(shown.value).boxes)
const box = computed(() => boxes.value.find((b) => b.id === selected.value) ?? null)
watch(
  boxes,
  (list) => {
    if (!list.some((b) => b.id === selected.value)) selected.value = list.find((b) => b.kind === 'plc')?.id ?? list[0]?.id ?? null
  },
  { immediate: true },
)
</script>

<template>
  <div class="flex h-full min-h-0">
    <div class="relative shrink-0 border-0 border-r border-solid border-line" :style="{ width: `${treeWidth}px` }">
      <div v-if="editingConnection" id="wiring-nodes" class="h-full" />
      <WiringSide v-else :wiring="shown" :selected="selected" @select="(id) => (selected = id)" />
      <UiSash v-model="treeWidth" side="left" :min="180" :max="480" label="Width of the wiring's side bar" />
    </div>

    <div class="relative flex min-w-0 flex-1 flex-col">
      <p v-if="trouble" class="m-0 shrink-0 border-0 border-b border-solid border-line bg-s1 px-3 py-2 text-danger" role="alert">{{ trouble }}</p>
      <ConnectionEditor v-if="editingConnection" :key="editingConnection.id" class="min-h-0 flex-1" v-bind="editingConnection" @close="editingConnection = null" />
      <WiringDiagram v-else class="min-h-0 flex-1" :wiring="shown" :editable="editable" :selected="selected" @select="(id) => (selected = id)" />
      <PagePanel v-if="editable" :tabs="[{ value: 'problems', label: 'Problems', count: checks.length }]">
        <template #problems><ProblemsPanel :checks="checks" /></template>
      </PagePanel>
    </div>

    <div class="relative shrink-0 border-0 border-l border-solid border-line" :style="{ width: `${wiringDetailsWidth}px` }">
      <UiSash v-model="wiringDetailsWidth" side="right" :min="320" :max="720" label="Width of the details" />
      <WiringDetails :box="box" :wiring="shown" :editable="editable" />
    </div>
  </div>
</template>
