<script setup lang="ts">
import { ask } from '../ask'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { Example, ProjectInfo } from '../../bindings/github.com/toblu96/taktwin/internal/simapp/models'
import { choose } from '../ask'
import { filesRead, importProject, newProject, openProject, projectError } from '../sim'
import NewProjectDialog from './NewProjectDialog.vue'
import ProjectCard from './ProjectCard.vue'

// The app's start: the user's projects, the latest first, then the stations the collections
// open with, to start a project from. A project file dropped onto the page opens. Everything
// in the simulation belongs to a project.
const projects = ref<ProjectInfo[]>([])
const examples = ref<Example[]>([])
const busy = ref(false)
const creating = ref(false)

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

// The tabs show while there are projects of both kinds.
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
  <main class="projects" data-file-drop-target>
    <div class="content">
      <header class="head">
        <h2>Projects</h2>
        <nav v-if="counts.station && counts.exercise" class="tabs" aria-label="Show">
          <button :class="{ active: kind === 'all' }" @click="kind = 'all'">All {{ counts.all }}</button>
          <button :class="{ active: kind === 'station' }" @click="kind = 'station'">Stations {{ counts.station }}</button>
          <button :class="{ active: kind === 'exercise' }" @click="kind = 'exercise'">Exercises {{ counts.exercise }}</button>
        </nav>
        <span class="spacer" />
        <input ref="search" v-model="query" class="search" type="search" placeholder="Filter" aria-label="Filter projects" @keydown.esc="query = ''" />
        <button :disabled="busy" title="Open a .taktwin-project file, or drop one onto this page" @click="act(importProject)">Open file…</button>
        <button class="primary" :disabled="busy" @click="creating = true">New project</button>
      </header>

      <p v-if="projectError" class="problem" role="alert">
        {{ projectError }}
        <button class="dismiss" aria-label="Dismiss" @click="projectError = ''">✕</button>
      </p>

      <div class="grid">
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
        <button
          v-for="e in shownExamples"
          :key="'example:' + e.station"
          class="example"
          :disabled="busy"
          title="Start a project of your own on this station"
          @click="act(() => newProject({ station: e.station, exercise: '' }))"
        >
          <span class="picture"><img src="/logo.svg" alt="" /><span class="badge">Example</span></span>
          <span class="body">
            <strong>{{ e.stationName }}</strong>
            <small>{{ e.collection }}</small>
            <span v-if="e.description" class="description">{{ e.description }}</span>
          </span>
        </button>
      </div>

      <p v-if="!shown.length && !shownExamples.length" class="empty">
        <template v-if="query">No project matches “{{ query }}”.</template>
        <template v-else>There is nothing to start from yet: import a collection in the settings, or open a project file.</template>
      </p>
    </div>
    <div class="drop" aria-hidden="true">Drop a project file to open it</div>
    <NewProjectDialog v-if="creating" @close="creating = false" />
  </main>
</template>

<style scoped>
.projects {
  min-height: 0;
  overflow: auto;
  position: relative;
}

.content {
  margin: 0 auto;
  max-width: 1200px;
  padding: 22px 28px 32px;
}

.head {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-bottom: 18px;
}

h2 {
  font-size: 20px;
  margin: 0 6px 0 0;
}

.tabs {
  display: flex;
  gap: 6px;
}

.tabs button {
  background: none;
  border-radius: 12px;
  color: var(--text-dim);
  font-size: 12px;
  padding: 3px 10px;
}

.tabs .active {
  background: var(--panel-raised);
  border-color: var(--text-dim);
  color: var(--text);
}

.spacer {
  flex: 1;
}

.search {
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: 3px;
  color: var(--text);
  padding: 5px 8px;
  width: 200px;
}

.primary {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--bg);
  font-weight: 600;
}

.problem {
  align-items: center;
  border-left: 2px solid var(--danger);
  display: flex;
  font-size: 13px;
  gap: 10px;
  justify-content: space-between;
  margin: 0 0 16px;
  padding: 4px 0 4px 10px;
}

.dismiss {
  background: none;
  border: 0;
  color: var(--text-dim);
}

.grid {
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(auto-fill, minmax(230px, 1fr));
}

.example {
  background: none;
  border: 1px dashed var(--line);
  border-radius: 6px;
  display: flex;
  flex-direction: column;
  padding: 0;
  text-align: left;
}

.example:hover:not(:disabled) {
  border-color: var(--text-dim);
}

.example .picture {
  align-items: center;
  aspect-ratio: 16 / 10;
  background: var(--panel);
  border-bottom: 1px dashed var(--line);
  border-radius: 5px 5px 0 0;
  display: flex;
  justify-content: center;
  position: relative;
  width: 100%;
}

.example .picture img {
  height: 56px;
  opacity: 0.5;
  width: 56px;
}

.badge {
  border: 1px solid var(--text-dim);
  border-radius: 3px;
  color: var(--text-dim);
  font-size: 11px;
  font-weight: 600;
  left: 8px;
  padding: 0 6px;
  position: absolute;
  top: 8px;
}

.example .body {
  display: grid;
  gap: 3px;
  padding: 10px 12px 12px;
}

.example strong {
  font-size: 14px;
}

.example small {
  color: var(--text-dim);
  font-size: 12px;
}

.description {
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  color: var(--text-dim);
  display: -webkit-box;
  font-size: 12px;
  line-height: 1.4;
  overflow: hidden;
}

.empty {
  color: var(--text-dim);
  margin-top: 24px;
  text-align: center;
}

/* Wails marks the drop target while a file is dragged over the window. */
.drop {
  align-items: center;
  background: rgb(11 31 34 / 85%);
  border: 2px dashed var(--accent);
  border-radius: 8px;
  color: var(--accent);
  display: none;
  font-size: 16px;
  font-weight: 600;
  inset: 12px;
  justify-content: center;
  pointer-events: none;
  position: absolute;
}

.projects.file-drop-target-active .drop {
  display: flex;
}
</style>
