<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { LibraryService, Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { Example, ExerciseInfo, ProjectInfo } from '../../bindings/github.com/toblu96/taktwin/internal/simapp/models'
import { grouped } from '../collections'
import { filesRead, importProject, newProject, openProject, projectError } from '../sim'

// The app's start: the user's projects, the latest first, then the stations the collections
// open with, to start a project from. Everything in the simulation belongs to a project.
const projects = ref<ProjectInfo[]>([])
const examples = ref<Example[]>([])
const stations = ref<{ id: string; name: string; collection?: string }[]>([])
const exercises = ref<ExerciseInfo[]>([])
const busy = ref(false)

async function refresh() {
  const [p, e] = await Promise.all([Service.Projects(), Service.Examples()])
  projects.value = p ?? []
  examples.value = e ?? []
}
onMounted(refresh)
watch(filesRead, refresh)

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

// The form for a new project: from a station or an exercise, with a name of its own or that
// of what it is made from.
const creating = ref(false)
const from = ref<'station' | 'exercise'>('station')
const station = ref('')
const exercise = ref('')
const name = ref('')

async function startNew() {
  creating.value = true
  ;[stations.value, exercises.value] = await Promise.all([
    LibraryService.Entries().then((all) => (all ?? []).filter((e) => e.level === 'station' && !e.error)),
    Service.AllExercises().then((all) => all ?? []),
  ])
  station.value ||= examples.value[0]?.station ?? stations.value[0]?.id ?? ''
  exercise.value ||= exercises.value[0]?.id ?? ''
}

const suggested = computed(() =>
  from.value === 'station' ? (stations.value.find((s) => s.id === station.value)?.name ?? '') : (exercises.value.find((e) => e.id === exercise.value)?.name ?? ''),
)

const create = () =>
  act(async () => {
    await newProject(from.value === 'station' ? { station: station.value, exercise: '' } : { station: '', exercise: exercise.value }, name.value.trim())
    creating.value = false
    name.value = ''
  })

// Renaming happens in place, one project at a time.
const renaming = ref('')
const newName = ref('')
const renameInput = ref<HTMLInputElement[]>([])

async function startRename(p: ProjectInfo) {
  renaming.value = p.name
  newName.value = p.name
  await nextTick()
  renameInput.value[0]?.select()
}

const rename = (p: ProjectInfo) =>
  act(async () => {
    if (newName.value.trim() !== p.name) await Service.RenameProject(p.name, newName.value.trim())
    renaming.value = ''
  })

function remove(p: ProjectInfo) {
  if (!confirm(`Delete the project ${p.name}, with its programs? This cannot be undone.`)) return
  void act(() => Service.DeleteProject(p.name))
}

const when = (iso?: string) => (iso ? new Date(iso).toLocaleString() : '')
const what = (p: ProjectInfo) => (p.exerciseName ? `Exercise: ${p.exerciseName} · ${p.stationName}` : p.stationName)
</script>

<template>
  <main class="projects">
    <header class="head">
      <h2>Projects</h2>
      <div class="actions">
        <button :disabled="busy" @click="act(importProject)">Import project file…</button>
        <button class="primary" :disabled="busy" @click="creating ? (creating = false) : startNew()">New project</button>
      </div>
    </header>

    <section v-if="creating" class="new">
      <div class="from" role="radiogroup" aria-label="Start from">
        <label><input v-model="from" type="radio" value="station" /> A station</label>
        <label><input v-model="from" type="radio" value="exercise" :disabled="!exercises.length" /> An exercise</label>
      </div>
      <label v-if="from === 'station'" class="field">
        <span>Station</span>
        <select v-model="station">
          <optgroup v-for="g in grouped(stations)" :key="g.id" :label="g.name">
            <option v-for="s in g.items" :key="s.id" :value="s.id">{{ s.name }}</option>
          </optgroup>
        </select>
      </label>
      <label v-else class="field">
        <span>Exercise</span>
        <select v-model="exercise">
          <optgroup v-for="g in grouped(exercises)" :key="g.id" :label="g.name">
            <option v-for="e in g.items" :key="e.id" :value="e.id">{{ e.name }} ({{ e.stationName }})</option>
          </optgroup>
        </select>
      </label>
      <label class="field">
        <span>Name</span>
        <input v-model="name" type="text" maxlength="80" :placeholder="suggested" @keydown.enter="create" @keydown.esc="creating = false" />
      </label>
      <div class="actions">
        <button class="primary" :disabled="busy || (from === 'station' ? !station : !exercise)" @click="create">Create</button>
        <button @click="creating = false">Cancel</button>
      </div>
    </section>

    <p v-if="projectError" class="problem" role="alert">{{ projectError }}</p>

    <ul class="list">
      <li v-for="p in projects" :key="p.name" :class="{ broken: p.problem }">
        <div class="what">
          <input
            v-if="renaming === p.name"
            ref="renameInput"
            v-model="newName"
            class="rename"
            type="text"
            maxlength="80"
            aria-label="New name"
            @keydown.enter="rename(p)"
            @keydown.esc="renaming = ''"
            @blur="renaming = ''"
          />
          <button v-else class="open" :disabled="busy || !!p.problem" :title="p.folder" @click="act(() => openProject(p.name))">{{ p.name }}</button>
          <p class="hint">{{ what(p) }} · {{ when(p.modified) }}</p>
          <p v-if="p.problem" class="hint warn">{{ p.problem }}</p>
        </div>
        <div class="actions">
          <button :disabled="busy" @click="startRename(p)">Rename</button>
          <button :disabled="busy" @click="act(() => Service.DuplicateProject(p.name))">Duplicate</button>
          <button :disabled="busy" @click="act(() => Service.ExportProject(p.name))">Export…</button>
          <button :disabled="busy" @click="remove(p)">Delete</button>
        </div>
      </li>
      <li v-for="e in examples" :key="'example:' + e.station" class="example">
        <div class="what">
          <button class="open" :disabled="busy" title="Start a project of your own on this station" @click="act(() => newProject({ station: e.station, exercise: '' }))">
            {{ e.stationName }}
          </button>
          <span class="badge">Example</span>
          <p class="hint">
            {{ e.collection }}<template v-if="e.description"> · {{ e.description }}</template>
          </p>
        </div>
      </li>
    </ul>
    <p v-if="!projects.length && !examples.length" class="hint">There is nothing to start from: import a collection in the settings, or a project file.</p>
  </main>
</template>

<style scoped>
.projects {
  max-width: 860px;
  overflow: auto;
  padding: 20px 24px;
}

.head {
  align-items: center;
  display: flex;
  gap: 12px;
  justify-content: space-between;
  margin-bottom: 14px;
}

h2 {
  font-size: 18px;
  margin: 0;
}

.actions {
  align-items: flex-start;
  display: flex;
  gap: 6px;
}

.primary {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--bg);
  font-weight: 600;
}

.new {
  border: 1px solid var(--line);
  border-radius: 3px;
  display: grid;
  gap: 10px;
  margin-bottom: 14px;
  padding: 12px;
}

.from {
  display: flex;
  gap: 16px;
}

.field {
  display: grid;
  gap: 3px;
  max-width: 420px;
}

.field span {
  color: var(--text-dim);
  font-size: 12px;
}

.field input,
.field select,
.rename {
  background: var(--bg);
  border: 1px solid var(--line);
  color: var(--text);
  padding: 4px 6px;
}

.list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.list li {
  align-items: center;
  border: 1px solid var(--line);
  border-radius: 3px;
  display: flex;
  gap: 12px;
  margin-bottom: 8px;
  padding: 10px 12px;
}

.list li.example {
  border-style: dashed;
}

.what {
  flex: 1;
  min-width: 0;
}

.open {
  background: none;
  border: none;
  font-size: 14px;
  font-weight: 600;
  padding: 0;
  text-align: left;
}

.open:hover:not(:disabled) {
  color: var(--accent);
}

.broken .open {
  color: var(--text-dim);
}

.hint {
  color: var(--text-dim);
  font-size: 12px;
  margin: 3px 0 0;
}

.warn {
  color: var(--danger);
}

.badge {
  border: 1px solid var(--line);
  border-radius: 8px;
  color: var(--text-dim);
  font-size: 11px;
  margin-left: 8px;
  padding: 0 7px;
}

.problem {
  border-left: 2px solid var(--danger);
  font-size: 13px;
  margin: 0 0 12px;
  padding: 2px 0 2px 8px;
}
</style>
