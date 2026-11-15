<script setup lang="ts">
import { Events } from '@wailsio/runtime'
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { LibraryService, Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { ProjectInfo } from '../../bindings/github.com/toblu96/taktwin/internal/simapp/models'
import { closeProject, importProject, newProject, openProject, project, saveProjectAs, simulation } from '../sim'

// The project menu, as a programming tool's File menu has it: new, open, recent, save, save
// as, close, and a project file to take elsewhere. Work without a name is "Untitled" and kept
// all the same; Save gives it one. A named project is saved as it changes. The app's File
// menu has the same commands with the system's shortcuts, and sends them here.
const open = ref(false)
const view = ref<'menu' | 'open' | 'name' | 'new'>('menu')
const projects = ref<ProjectInfo[]>([])
const stations = ref<{ id: string; name: string }[]>([])
const name = ref('')
const station = ref('')
const message = ref('')
const busy = ref(false)
const input = ref<HTMLInputElement | null>(null)

const title = computed(() => (project.value?.saved ? project.value.name : 'Untitled'))
const recent = computed(() => projects.value.filter((p) => p.name !== project.value?.name).slice(0, 5))

async function show(next: typeof view.value = 'menu') {
  message.value = ''
  view.value = next
  open.value = true
  // The name and station first, before anything is waited for, so that typing right away
  // is not overwritten.
  if (next === 'name' || next === 'new') {
    name.value = next === 'name' && project.value?.saved ? `${project.value.name} (copy)` : (project.value?.stationName ?? '')
    station.value = simulation.scene.value?.id ?? ''
    await nextTick()
    input.value?.select()
  }
  if (next === 'new') {
    stations.value = ((await LibraryService.Entries()) ?? []).filter((e) => e.level === 'station' && !e.error)
    if (!station.value) station.value = stations.value[0]?.id ?? ''
  }
  projects.value = (await Service.Projects()) ?? []
}

function close() {
  open.value = false
}

async function act(what: () => Promise<unknown>) {
  busy.value = true
  message.value = ''
  try {
    await what()
    close()
  } catch (e) {
    message.value = (e as Error).message ?? String(e)
  } finally {
    busy.value = false
  }
}

// Save: an untitled project asks for its name; a named one is saved already.
function save() {
  if (project.value?.saved) {
    message.value = 'Saved: the project keeps every change as it is made.'
    open.value = true
    view.value = 'menu'
    return
  }
  void show('name')
}

const confirm = () =>
  view.value === 'new' ? act(() => newProject(name.value.trim(), station.value)) : act(() => saveProjectAs(name.value.trim()))

async function remove(p: ProjectInfo) {
  try {
    await Service.DeleteProject(p.name)
    projects.value = (await Service.Projects()) ?? []
  } catch (e) {
    message.value = (e as Error).message
  }
}

// Writes the project into a file the user picks; cancelling changes nothing.
const exportFile = () => act(() => Service.ExportProject())

// The commands of the app's File menu, which has the system's shortcuts for them.
const commands: Record<string, () => unknown> = {
  new: () => show('new'),
  open: () => show('open'),
  save,
  saveAs: () => show('name'),
  close: () => (project.value?.saved ? act(closeProject) : undefined),
  export: exportFile,
  import: () => act(importProject),
}

function escape(e: KeyboardEvent) {
  if (e.key === 'Escape') close()
}

let off: (() => void) | undefined
onMounted(() => {
  off = Events.On('project:command', (event) => void commands[event.data as string]?.())
  window.addEventListener('keydown', escape)
})
onUnmounted(() => {
  off?.()
  window.removeEventListener('keydown', escape)
})

const when = (iso?: string) => (iso ? new Date(iso).toLocaleString() : '')
</script>

<template>
  <div class="project">
    <button class="current" :class="{ untitled: !project?.saved }" :title="project?.folder" aria-haspopup="menu" :aria-expanded="open" @click="open ? close() : show()">
      <span class="label">Project</span>
      <span class="title">{{ title }}</span>
    </button>
    <div v-if="open" class="backdrop" @click="close" />
    <div v-if="open" class="menu" role="menu">
      <template v-if="view === 'menu'">
        <button role="menuitem" @click="show('new')">New project…</button>
        <button role="menuitem" @click="show('open')">Open project…</button>
        <template v-if="recent.length">
          <p class="group">Recent</p>
          <button v-for="p in recent" :key="p.name" role="menuitem" class="recent" :title="p.folder" @click="act(() => openProject(p.name))">
            {{ p.name }} <small>{{ p.stationName || p.station }}</small>
          </button>
        </template>
        <hr />
        <button role="menuitem" @click="save">{{ project?.saved ? 'Save' : 'Save…' }}</button>
        <button role="menuitem" @click="show('name')">Save as…</button>
        <button v-if="project?.saved" role="menuitem" @click="act(closeProject)">Close project</button>
        <hr />
        <button role="menuitem" @click="exportFile">Export project file…</button>
        <button role="menuitem" @click="act(importProject)">Import project file…</button>
        <p class="note">
          {{
            project?.saved
              ? `Saved as it changes, in your folder under projects/${project.name}.`
              : 'Untitled: your work on this station is kept as it changes. Save gives it a name of its own.'
          }}
        </p>
      </template>
      <template v-else-if="view === 'open'">
        <p class="group">Projects</p>
        <p v-if="!projects.length" class="note">There are no projects yet: Save gives your work a name.</p>
        <div v-for="p in projects" :key="p.name" class="row">
          <button class="pick" :class="{ active: p.name === project?.name }" :title="p.folder" @click="act(() => openProject(p.name))">
            {{ p.name }}
            <small>{{ p.stationName || p.station }} · {{ when(p.modified) }}</small>
          </button>
          <button class="delete" :disabled="p.name === project?.name" title="Delete the project" @click="remove(p)">✕</button>
        </div>
        <button @click="view = 'menu'">Back</button>
      </template>
      <template v-else>
        <p class="group">{{ view === 'new' ? 'New project' : project?.saved ? 'Save as' : 'Save' }}</p>
        <label v-if="view === 'new'" class="field">
          <span>Station</span>
          <select v-model="station">
            <option v-for="s in stations" :key="s.id" :value="s.id">{{ s.name }}</option>
          </select>
        </label>
        <label class="field">
          <span>Name</span>
          <input ref="input" v-model="name" type="text" maxlength="80" @keydown.enter="confirm" />
        </label>
        <div class="actions">
          <button class="primary" :disabled="busy || !name.trim()" @click="confirm">{{ view === 'new' ? 'Create' : 'Save' }}</button>
          <button @click="view = 'menu'">Cancel</button>
        </div>
      </template>
      <p v-if="message" class="message" :class="{ error: !message.startsWith('Saved') }" role="status">{{ message }}</p>
    </div>
  </div>
</template>

<style scoped>
.project {
  position: relative;
}

.current {
  align-items: baseline;
  background: var(--panel);
  border: 1px solid var(--line);
  display: flex;
  gap: 6px;
  max-width: 260px;
  padding: 3px 8px;
}

.label {
  color: var(--text-dim);
  font-size: 11px;
}

.title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.untitled .title {
  color: var(--text-dim);
  font-style: italic;
}

.backdrop {
  inset: 0;
  position: fixed;
  z-index: 19;
}

.menu {
  background: var(--panel);
  border: 1px solid var(--line);
  box-shadow: 0 6px 24px rgb(0 0 0 / 35%);
  display: flex;
  flex-direction: column;
  gap: 2px;
  left: 0;
  max-height: 70vh;
  overflow: auto;
  padding: 6px;
  position: absolute;
  top: calc(100% + 4px);
  width: 320px;
  z-index: 20;
}

.menu > button,
.pick {
  background: none;
  border: none;
  display: flex;
  justify-content: space-between;
  padding: 5px 8px;
  text-align: left;
}

.menu > button:hover,
.pick:hover {
  background: var(--bg);
}

small {
  color: var(--text-dim);
  font-size: 11px;
}

.pick {
  flex: 1;
  flex-direction: column;
}

.pick.active {
  color: var(--accent);
}

.row {
  align-items: center;
  display: flex;
}

.delete {
  background: none;
  border: none;
  color: var(--text-dim);
}

.group {
  color: var(--text-dim);
  font-size: 11px;
  margin: 6px 8px 2px;
  text-transform: uppercase;
}

.note {
  color: var(--text-dim);
  font-size: 11px;
  margin: 6px 8px 2px;
}

hr {
  border: none;
  border-top: 1px solid var(--line);
  margin: 4px 0;
  width: 100%;
}

.field {
  display: grid;
  gap: 3px;
  margin: 4px 8px;
}

.field span {
  color: var(--text-dim);
  font-size: 12px;
}

.field input,
.field select {
  background: var(--bg);
  border: 1px solid var(--line);
  color: var(--text);
  padding: 4px 6px;
}

.actions {
  display: flex;
  gap: 6px;
  margin: 6px 8px;
}

.primary {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--bg);
  font-weight: 600;
}

.message {
  font-size: 12px;
  margin: 6px 8px;
}

.message.error {
  color: var(--danger);
}
</style>
