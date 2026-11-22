<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, provide, ref } from 'vue'
import { LibraryService } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { Entry } from '../../bindings/github.com/toblu96/taktwin/internal/model'
import {
  addStand,
  built,
  busy,
  cancelPlacing,
  enter,
  ghost,
  ghostPose,
  handle,
  handleMoved,
  hover,
  leave,
  nudge as step,
  overlay,
  pending,
  placeAt,
  placing,
  remove,
  rotate,
  selected,
  state,
} from '../builder'
import { detailsWidth, treeWidth } from '../layout'
import { simKey } from '../sim'
import UiSash from '../ui/UiSash.vue'
import StationViewer from '../viewer/StationViewer.vue'
import BuilderDetails from './BuilderDetails.vue'
import BuilderSide from './BuilderSide.vue'
import PagePanel from './PagePanel.vue'
import ProblemsPanel from './ProblemsPanel.vue'

// The simulation's view edited: the builder. The station's name and modules, and what can be
// added, on the left; the station in 3D in the middle, with what does not fit together below
// it; the module being added or the one selected on the right. Every change is kept in the
// project, and the station runs as it is built once editing is switched off.

// The components below show the station being built, not the running simulation.
provide(simKey, built)

const entries = ref<Entry[]>([])
onMounted(async () => {
  window.addEventListener('keydown', onKey)
  await enter()
  entries.value = (await LibraryService.Entries()) ?? []
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  leave()
})

// Escape puts a picked module back or deselects, R turns a picked one, Backspace or Delete
// removes the selected one, and the arrow keys move it: along its line a grid step, or a mm
// with Shift, and across to the next slot.
function onKey(event: KeyboardEvent) {
  const target = event.target as HTMLElement | null
  if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT' || target.isContentEditable)) return
  if (event.key === 'Escape') {
    if (pending.value) cancelPlacing()
    else built.selection.value = null
    return
  }
  if (pending.value && (event.key === 'r' || event.key === 'R')) {
    rotate()
    return
  }
  if (!selected.value || busy.value) return
  const arrows: Record<string, ['along' | 'across', number]> = {
    ArrowRight: ['along', 1],
    ArrowLeft: ['along', -1],
    ArrowUp: ['across', 1],
    ArrowDown: ['across', -1],
  }
  if (event.key in arrows) {
    event.preventDefault()
    const [axis, sign] = arrows[event.key]
    void step(axis, sign, event.shiftKey)
  } else if (event.key === 'Backspace' || event.key === 'Delete') {
    event.preventDefault()
    void remove(selected.value)
  }
}

// Where the module is while its handle is dragged, before it is let go.
const draggedAt = ref<number | null>(null)
function handled(key: string, along: number, done: boolean) {
  if (!done) {
    draggedAt.value = along
    return
  }
  draggedAt.value = null
  void handleMoved(key, along)
}

const checks = computed(() => state.value?.scene.checks ?? [])
const onFloor = computed(() => selected.value?.place?.on === 'floor.area')
const hint = computed(() =>
  pending.value
    ? `Point at a line to place the ${pending.value.name}, click to put it down · R turns it · Esc cancels`
    : onFloor.value
      ? 'Drag an arrow or the dot to move the stand with everything on it · click + to put an empty stand beside it'
      : selected.value?.place
        ? 'Drag an arrow to move it one way, or the dot both ways · 5 mm grid, 1 mm with Shift · tap an arrow or use the arrow keys for one step'
        : 'Click a module to select it · pick one under Add to put it in · click a stand to put another beside it',
)
</script>

<template>
  <div class="flex h-full min-h-0">
    <div class="relative shrink-0 border-0 border-r border-solid border-line" :style="{ width: `${treeWidth}px` }">
      <BuilderSide :entries="entries" />
      <UiSash v-model="treeWidth" side="left" :min="200" :max="480" label="Width of the builder's side bar" />
    </div>

    <div class="relative flex min-w-0 flex-1 flex-col">
      <div class="relative grid min-h-0 flex-1">
        <StationViewer
          building
          :hint="hint"
          :overlay="overlay"
          :placing="placing"
          :ghost="ghost?.instances ?? null"
          :ghost-pose="ghostPose"
          :handle="handle"
          @hover="hover"
          @place="placeAt"
          @handle="handled"
          @nudge="(axis, sign) => step(axis, sign)"
          @add-beside="addStand"
        />
      </div>
      <PagePanel :tabs="[{ value: 'problems', label: 'Problems', count: checks.length }]">
        <template #problems><ProblemsPanel :checks="checks" /></template>
      </PagePanel>
    </div>

    <div class="relative shrink-0 border-0 border-l border-solid border-line" :style="{ width: `${detailsWidth}px` }">
      <UiSash v-model="detailsWidth" side="right" :min="260" :max="560" label="Width of the details" />
      <BuilderDetails :dragged-at="draggedAt" />
    </div>
  </div>
</template>
