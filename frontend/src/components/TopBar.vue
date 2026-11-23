<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Service as service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { useRoute } from 'vue-router'
import { closeProject, project, saveProjectAs, saveState, simulation } from '../sim'
import { expert } from '../expert'
import { setEditing } from '../project-view'
import { editing, viewProblem } from '../router'
import UiModeToggle from '../ui/UiModeToggle.vue'
import IconRun from '~icons/fluent/play-16-regular'
import IconEdit from '~icons/fluent/edit-16-regular'
import UiSegmented from '../ui/UiSegmented.vue'
import IconChevron from '~icons/fluent/chevron-right-16-regular'
import IconPause from '~icons/fluent/pause-20-filled'
import IconPlay from '~icons/fluent/play-20-filled'
import IconRestart from '~icons/fluent/arrow-counterclockwise-20-regular'
import IconWarning from '~icons/fluent/warning-16-filled'
import IconUndo from '~icons/fluent/arrow-undo-20-regular'
import IconKeep from '~icons/fluent/bookmark-20-regular'
import IconShare from '~icons/fluent/share-20-regular'
import IconCopy from '~icons/fluent/copy-20-regular'
import IconFolder from '~icons/fluent/folder-20-regular'
import UiMenu, { type MenuEntry } from '../ui/UiMenu.vue'
import { handOut, keepAsOriginal, original, putBack, refreshOriginal } from '../original'

// The bar above an open project: where it is, how its station runs, and in expert mode the switch
// between running its views and editing them. The activity bar has the ways to go elsewhere;
// warnings that need an answer sit on the 3D view.
const { scene, snapshot } = simulation

// The project's menu: what can be put back as its original has it, keeping it as the original
// in expert mode, and handing it out, saving a copy or showing its file. What differs from the
// original is looked at again whenever the file is saved.
watch([project, () => saveState.value?.state, scene], () => void refreshOriginal().catch(() => undefined), { immediate: true })
const plcName = (key: string) => {
  const first = scene.value?.plcs?.[0]?.module ?? scene.value?.plc?.module ?? ''
  const module = key || first
  const name = scene.value?.instances?.find((m) => m.id === module)?.name ?? 'the PLC'
  return (scene.value?.plcs?.length ?? 1) > 1 ? name : 'the PLC'
}
const menuProblem = ref('')
const run = (what: () => Promise<unknown>) => () => {
  menuProblem.value = ''
  what().catch((e: Error) => (menuProblem.value = e.message ?? String(e)))
}
const projectMenu = computed<MenuEntry[]>(() => {
  const p = project.value
  const o = original.value
  if (!p) return []
  const back: MenuEntry[] = []
  for (const key of o?.connections ?? [])
    back.push({ label: `Put back how ${plcName(key)} is connected`, icon: IconUndo, run: run(() => putBack('connection', key, `how ${plcName(key)} is connected`)) })
  for (const key of o?.programs ?? [])
    back.push({ label: `Put back the programs of ${plcName(key)}`, icon: IconUndo, run: run(() => putBack('programs', key, `the programs of ${plcName(key)}`)) })
  if (o?.station && !p.exercise) back.push({ label: 'Put back the station', icon: IconUndo, run: run(() => putBack('station', '', 'the station, as built and wired')) })
  if (o?.settings) back.push({ label: "Put back the station's settings", icon: IconUndo, run: run(() => putBack('settings', '', "the station's settings")) })
  if (o?.changed) back.push({ label: 'Put back everything…', icon: IconUndo, run: run(() => putBack('everything', '', 'everything')) })
  if (!back.length) back.push({ label: o?.at ? 'Nothing changed since the original' : 'No original to put back', icon: IconUndo, disabled: true })
  return [
    ...back,
    {},
    ...(expert.value ? [{ label: 'Keep this as the original', icon: IconKeep, disabled: !o?.changed, run: run(keepAsOriginal) }] : []),
    { label: 'Hand out…', icon: IconShare, run: run(() => handOut(p)) },
    { label: 'Save a copy…', icon: IconCopy, run: run(() => service.SaveProjectCopy(p.path)) },
    { label: 'Show in folder', icon: IconFolder, run: run(() => service.RevealProject(p.path)) },
  ]
})

const route = useRoute()
const running = computed(() => route.name === 'simulate')

// Why the switch to editing is off, if it is: an exercise's station stays as the exercise has
// it, and writing exercises comes later.
const noEdit = computed(() =>
  project.value?.exercise ? 'The station of an exercise stays as the exercise has it' : route.meta.view === 'exercise' ? 'Writing exercises comes in a later version' : '',
)
const edit = computed({ get: () => editing.value && !noEdit.value, set: (on: boolean) => void setEditing(on) })
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
    <span class="min-w-0 truncate font-semibold" :title="project.path">{{ project.name }}</span>
    <template v-if="saveState?.state === 'readOnly'">
      <span class="shrink-0 rounded-sm border border-control px-1.5 text-[11px] leading-4 text-muted" :title="`${project.path} cannot be written`">Read only</span>
      <button class="t-btn h-6 shrink-0 px-2 text-xs" title="Save the project as a copy that can be written, and go on in it" @click="saveProjectAs().catch(() => undefined)">
        Save a copy…
      </button>
    </template>
    <span v-if="project.exerciseName" class="shrink-0 rounded-sm bg-accent-soft px-1.5 text-[11px] leading-4.5 font-medium text-accent-ink">Exercise</span>
    <span v-if="(project.exerciseName || project.stationName) !== project.name" class="min-w-0 truncate text-muted">{{ project.exerciseName || project.stationName }}</span>
    <UiMenu :items="projectMenu" :label="`More for ${project.name}`" class="-mx-1 size-7" />
    <span v-if="viewProblem || menuProblem" class="flex min-w-0 items-center gap-1.5 truncate text-danger" :title="viewProblem || menuProblem">
      <IconWarning class="size-4 shrink-0" />{{ viewProblem || menuProblem }}
    </span>
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
    <template v-if="expert">
      <span class="mx-1 h-4 w-px bg-line" />
      <UiModeToggle
        v-model="edit"
        label="Edit"
        :off="{ icon: IconRun, label: 'Running' }"
        :on="{ icon: IconEdit, label: 'Editing' }"
        :disabled="!!noEdit"
        :title="noEdit || undefined"
      />
    </template>
  </header>
</template>
