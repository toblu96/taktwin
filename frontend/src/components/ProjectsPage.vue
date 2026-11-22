<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { Example, ProjectInfo } from '../../bindings/github.com/toblu96/taktwin/internal/simapp/models'
import { ask, choose } from '../ask'
import { filesRead, importProject, newProject, openProject, projectError } from '../sim'
import NewProjectDialog from './NewProjectDialog.vue'
import ProjectCard from './ProjectCard.vue'
import UiSegmented from '../ui/UiSegmented.vue'
import IconSearch from '~icons/fluent/search-20-regular'
import IconAdd from '~icons/fluent/add-20-regular'
import IconOpen from '~icons/fluent/folder-open-20-regular'
import IconWrench from '~icons/fluent/wrench-20-regular'
import IconWarning from '~icons/fluent/warning-16-filled'
import IconDismiss from '~icons/fluent/dismiss-16-regular'

// The app's start: the user's projects, the latest first, then the stations the collections
// open with, to start a project from. A project file dropped onto the page opens. Everything
// in the simulation belongs to a project.
const projects = ref<ProjectInfo[]>([])
const examples = ref<Example[]>([])
const busy = ref(false)
const creating = ref(false)
const start = ref<'station' | 'exercise' | 'empty'>('station')
const create = (from: 'station' | 'exercise' | 'empty') => {
  start.value = from
  creating.value = true
}

async function refresh() {
  const [p, e] = await Promise.all([Service.Projects(), Service.Examples()])
  projects.value = p ?? []
  examples.value = e ?? []
}
onMounted(refresh)
watch(filesRead, refresh)
// A dropped file may be imported and still not open.
watch(projectError, refresh)

// act runs an action and shows what went wrong, in the same place as the app menu's errors.
async function act(what: () => Promise<unknown>) {
  busy.value = true
  projectError.value = ''
  try {
    await what()
  } catch (e) {
    projectError.value = (e as Error).message ?? String(e)
  } finally {
    busy.value = false
    await refresh()
  }
}

async function remove(p: ProjectInfo) {
  if (!(await ask(`Delete the project ${p.name}, with its programs? This cannot be undone.`, 'Delete'))) return
  await act(() => Service.DeleteProject(p.name))
}

// Hands an exercise out as one file, with the programs as a start only when asked to, so
// that a solution does not go out by mistake.
async function handout(p: ProjectInfo) {
  const WITH = 'With the programs'
  const answer = await choose(
    `Hand out ${p.exerciseName || p.name} as one file? It brings the station, the exercise and the collections students may not have. Should the programs of this project go along as a start?`,
    ['Without programs', WITH],
  )
  if (answer) await act(() => Service.ExportExercise(p.name, answer === WITH))
}

// The kinds to show, once there are projects of both.
type Kind = 'all' | 'station' | 'exercise'
const kind = ref<Kind>('all')
const query = ref('')
const search = ref<HTMLInputElement | null>(null)
const matches = (...texts: (string | undefined)[]) => {
  const q = query.value.trim().toLowerCase()
  return !q || texts.some((t) => t?.toLowerCase().includes(q))
}
const counts = computed(() => ({
  all: projects.value.length,
  station: projects.value.filter((p) => !p.exercise).length,
  exercise: projects.value.filter((p) => p.exercise).length,
}))
const shown = computed(() => projects.value.filter((p) => (kind.value === 'all' || (kind.value === 'exercise') === !!p.exercise) && matches(p.name, p.stationName, p.exerciseName)))
const shownExamples = computed(() => (kind.value === 'exercise' ? [] : examples.value.filter((e) => matches(e.stationName, e.collection, e.description))))
const kinds = computed(() => [
  { value: 'all', label: `All ${counts.value.all}` },
  { value: 'station', label: `Stations ${counts.value.station}` },
  { value: 'exercise', label: `Exercises ${counts.value.exercise}` },
])
watch(counts, (c) => {
  if (!c.station || !c.exercise) kind.value = 'all'
})

// ⌘F or Ctrl+F goes to the filter, Esc clears it.
function key(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'f') {
    e.preventDefault()
    search.value?.focus()
  }
}
onMounted(() => window.addEventListener('keydown', key))
onUnmounted(() => window.removeEventListener('keydown', key))
</script>

<template>
  <main class="group relative min-h-0 overflow-auto" data-file-drop-target>
    <div class="mx-auto max-w-295 px-8 pt-7 pb-10">
      <header class="mb-5 flex items-end gap-3">
        <div>
          <h1 class="m-0 text-xl font-semibold tracking-tight">Projects</h1>
          <p class="m-0 mt-0.5 text-muted">Your work on stations and exercises, saved as you go.</p>
        </div>
        <span class="flex-1" />
        <button class="t-btn t-btn-ghost" :disabled="busy" title="Open a .taktwin-project file, or drop one onto this page" @click="act(importProject)">
          <IconOpen class="size-4" />Open file…
        </button>
        <button class="t-btn t-btn-primary" :disabled="busy" @click="create('station')"><IconAdd class="size-4" />New project</button>
      </header>

      <div class="mb-4 flex flex-wrap items-center gap-3">
        <label class="relative w-64">
          <IconSearch class="pointer-events-none absolute top-1.5 left-2 size-4 text-faint" />
          <input ref="search" v-model="query" class="t-input pl-8" type="search" placeholder="Search projects" aria-label="Search projects" @keydown.esc="query = ''" />
        </label>
        <UiSegmented v-if="counts.station && counts.exercise" v-model="kind" label="Show" :options="kinds" />
      </div>

      <p v-if="projectError" class="m-0 mb-4 flex items-start gap-2 rounded-card border border-danger/40 bg-s1 px-3 py-2" role="alert">
        <IconWarning class="mt-0.5 size-4 shrink-0 text-danger" />
        <span class="flex-1">{{ projectError }}</span>
        <button class="t-icon-btn -my-1 size-6" aria-label="Dismiss" @click="projectError = ''"><IconDismiss class="size-4" /></button>
      </p>

      <div v-if="shown.length" class="grid grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-4">
        <ProjectCard
          v-for="p in shown"
          :key="p.name"
          :project="p"
          :busy="busy"
          @open="act(() => openProject(p.name))"
          @rename="(to) => act(() => Service.RenameProject(p.name, to))"
          @duplicate="act(() => Service.DuplicateProject(p.name))"
          @reveal="act(() => Service.RevealProject(p.name))"
          @export="act(() => Service.ExportProject(p.name))"
          @handout="handout(p)"
          @remove="remove(p)"
        />
      </div>
      <p v-else-if="query" class="m-0 text-muted">No project matches “{{ query }}”.</p>

      <h2 v-if="kind !== 'exercise'" class="m-0 mt-9 mb-3 text-[13px] font-semibold text-muted">Start from</h2>
      <div v-if="kind !== 'exercise'" class="grid grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-4">
        <button
          v-for="e in shownExamples"
          :key="'example:' + e.station"
          class="flex cursor-default items-center gap-3 rounded-card border border-dashed border-control bg-transparent p-3 text-left text-fg hover:bg-hover"
          :disabled="busy"
          :title="e.description || 'Start a project of your own on this station'"
          @click="act(() => newProject({ station: e.station, exercise: '' }))"
        >
          <span class="flex h-12 w-19 shrink-0 items-center justify-center rounded-[5px] bg-s3"><img src="/logo.svg" alt="" class="size-7 opacity-70" /></span>
          <span class="min-w-0">
            <span class="block truncate font-semibold">{{ e.stationName }}</span>
            <span class="block truncate text-xs text-muted">Example · {{ e.collection }}</span>
          </span>
        </button>
        <button
          class="flex cursor-default items-center gap-3 rounded-card border border-dashed border-control bg-transparent p-3 text-left text-fg hover:bg-hover"
          :disabled="busy"
          @click="create('empty')"
        >
          <span class="flex h-12 w-19 shrink-0 items-center justify-center rounded-[5px] bg-s3 text-muted"><IconWrench class="size-5" /></span>
          <span class="min-w-0">
            <span class="block font-semibold">Build your own</span>
            <span class="block text-xs text-muted">An empty station to put together</span>
          </span>
        </button>
      </div>
    </div>
    <!-- Wails marks the page while a file is dragged over the window. -->
    <div
      class="pointer-events-none absolute inset-3 hidden items-center justify-center rounded-dialog border-2 border-dashed border-accent bg-s0/85 text-base font-semibold text-accent-ink group-[.file-drop-target-active]:flex"
      aria-hidden="true"
    >
      Drop a project file to open it
    </div>
    <NewProjectDialog v-model:open="creating" :start="start" />
  </main>
</template>
