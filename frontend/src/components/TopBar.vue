<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Service as service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { useRoute } from 'vue-router'
import { closeProject, project, simulation } from '../sim'
import { viewProblem } from '../project-view'
import UiSegmented from '../ui/UiSegmented.vue'
import IconChevron from '~icons/fluent/chevron-right-16-regular'
import IconPause from '~icons/fluent/pause-20-filled'
import IconPlay from '~icons/fluent/play-20-filled'
import IconRestart from '~icons/fluent/arrow-counterclockwise-20-regular'
import IconWarning from '~icons/fluent/warning-16-filled'

// The bar above an open project: where it is, and how its station runs. The activity bar has the
// ways to go elsewhere; warnings that need an answer sit on the 3D view.
const { snapshot } = simulation

const route = useRoute()
const running = computed(() => route.name === 'simulate')
const paused = computed(() => snapshot.value?.paused ?? false)
const time = computed(() => {
  const total = snapshot.value?.time ?? 0
  const minutes = Math.floor(total / 60)
  const seconds = (total % 60).toFixed(1).padStart(4, '0')
  return `${String(minutes).padStart(2, '0')}:${seconds}`
})

// The speed, kept here so that a click shows at once: the simulation's snapshot only reports it
// a frame later, and changes it here only when it really changes. A PLC over OPC UA runs in real
// time, so only 1× is offered then.
const speed = ref(String(snapshot.value?.speed ?? 1))
watch(
  () => snapshot.value?.speed,
  (v) => (speed.value = String(v ?? 1)),
)
watch(speed, (v) => {
  if (Number(v) !== snapshot.value?.speed) void service.SetSpeed(Number(v))
})
const speeds = computed(() =>
  [1, 2, 5].map((s) => ({
    value: String(s),
    label: `${s}×`,
    disabled: !!snapshot.value?.link && s !== 1,
    title: snapshot.value?.link && s !== 1 ? 'A PLC over OPC UA runs in real time' : undefined,
  })),
)
</script>

<template>
  <header v-if="project" class="flex h-10 shrink-0 items-center gap-2 border-b border-line bg-s1 px-3">
    <button class="t-btn t-btn-ghost h-7 px-1.5" title="Close the project and go back to your projects" @click="closeProject">Projects</button>
    <IconChevron class="size-3.5 shrink-0 text-faint" />
    <span class="min-w-0 truncate font-semibold" :title="project.folder">{{ project.name }}</span>
    <span v-if="project.exerciseName" class="shrink-0 rounded-sm bg-accent-soft px-1.5 text-[11px] leading-4.5 font-medium text-accent-ink">Exercise</span>
    <span v-if="(project.exerciseName || project.stationName) !== project.name" class="min-w-0 truncate text-muted">{{ project.exerciseName || project.stationName }}</span>
    <span v-if="viewProblem" class="flex min-w-0 items-center gap-1.5 truncate text-danger"><IconWarning class="size-4 shrink-0" />{{ viewProblem }}</span>
    <span class="flex-1" />
    <template v-if="running">
      <span class="font-mono text-xs text-muted" title="Simulated time">{{ time }}</span>
      <UiSegmented v-model="speed" label="Simulation speed" :options="speeds" />
      <button class="t-icon-btn" :title="paused ? 'Resume' : 'Pause'" :aria-label="paused ? 'Resume' : 'Pause'" @click="service.SetPaused(!paused)">
        <component :is="paused ? IconPlay : IconPause" class="size-4.5" />
      </button>
      <button class="t-icon-btn" title="Restart the station and its PLC" aria-label="Restart" @click="service.Reset()">
        <IconRestart class="size-4.5" />
      </button>
    </template>
  </header>
</template>
