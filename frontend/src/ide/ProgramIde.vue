<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { ProgramService } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { ProgramProject, ProgramStatus } from '../../bindings/github.com/toblu96/taktwin/internal/simapp/models'
import type { Diagnostic, WatchVar } from '../../bindings/github.com/toblu96/taktwin/st/models'
import { complete, parseUnit } from './complete'
import StEditor from './StEditor.vue'
import WatchTree from './WatchTree.vue'
import { declared, type Monitored } from './st'

// The programming tool of a PLC, as CODESYS has it: the project's POUs, an editor for each,
// building and downloading into the app's soft PLC, running it, and watching, writing and
// forcing its variables online.
const props = defineProps<{ plc: string; window?: boolean }>()
const emit = defineEmits<{ close: [] }>()

interface Tab {
  name: string
  text: string
  saved: string
  readOnly: boolean
  goto: { line: number; col: number; n: number } | null
}

const project = ref<ProgramProject | null>(null)
const status = ref<ProgramStatus | null>(null)
const tabs = ref<Tab[]>([])
const active = ref('')
const diagnostics = ref<Diagnostic[]>([])
const built = ref<'ok' | 'failed' | ''>('')
const message = ref('')
const busy = ref(false)
const bottom = ref<'messages' | 'watch' | 'task'>('messages')
const vars = ref<WatchVar[]>([])
const open = reactive(new Set<string>())
const prepared = reactive<Record<string, string>>({})
const adding = ref<{ name: string; kind: string } | null>(null)
const renaming = ref<{ from: string; to: string } | null>(null)
const task = reactive({ programs: '', cycle: '', step: '' })
const downloads = ref(0)
// The text of every file as saved, for completion across the project.
const sources = reactive<Record<string, string>>({})

const tab = computed(() => tabs.value.find((t) => t.name === active.value) ?? null)
const dirty = (t: Tab) => t.text !== t.saved
const online = computed(() => !!status.value?.loaded)
const running = computed(() => status.value?.state === 'running')

function fail(e: unknown) {
  message.value = (e as Error).message ?? String(e)
}

async function load() {
  try {
    project.value = await ProgramService.Open(props.plc)
    status.value = project.value.status
    task.programs = (project.value.meta.programs ?? []).join(', ')
    task.cycle = project.value.meta.cycle
    task.step = project.value.meta.step ?? ''
    if (!tabs.value.length) await openFile('PLC_PRG.st')
    await loadSources()
  } catch (e) {
    fail(e)
  }
}

async function refreshFiles() {
  try {
    project.value = await ProgramService.Open(props.plc)
    await loadSources()
  } catch (e) {
    fail(e)
  }
}

async function loadSources() {
  const names = new Set((project.value?.files ?? []).map((f) => f.name))
  for (const k of Object.keys(sources)) if (!names.has(k)) delete sources[k]
  for (const name of names) sources[name] = await ProgramService.ReadFile(props.plc, name)
}

// suggestions lists what fits at the cursor of the open file, from what every file declares,
// open ones as they are typed.
function suggestions(text: string, offset: number) {
  const name = active.value
  const units = Object.keys(sources)
    .filter((f) => f !== name)
    .map((f) => parseUnit(f, tabs.value.find((t) => t.name === f)?.text ?? sources[f]))
  units.push(parseUnit(name, text))
  return complete(units, name, text, offset)
}

async function openFile(name: string) {
  if (!tabs.value.some((t) => t.name === name)) {
    try {
      const text = await ProgramService.ReadFile(props.plc, name)
      const file = project.value?.files?.find((f) => f.name === name)
      tabs.value.push({ name, text, saved: text, readOnly: !!file?.readOnly, goto: null })
    } catch (e) {
      fail(e)
      return
    }
  }
  active.value = name
}

function closeTab(name: string) {
  const t = tabs.value.find((x) => x.name === name)
  if (t && dirty(t) && !confirm(`${name} has changes that are not saved. Close it anyway?`)) return
  tabs.value = tabs.value.filter((x) => x.name !== name)
  if (active.value === name) active.value = tabs.value[0]?.name ?? ''
}

async function save(t: Tab) {
  if (t.readOnly || !dirty(t)) return
  // Into the project the tab comes from, also if the simulation has changed station since.
  await ProgramService.SaveFile(project.value!.station, project.value!.plc, t.name, t.text)
  t.saved = t.text
}

async function saveAll() {
  for (const t of tabs.value) await save(t)
}

// run does one of the toolbar's actions, saving first where the project is compiled.
async function run(what: () => Promise<unknown>, saveFirst = false) {
  if (busy.value) return
  busy.value = true
  message.value = ''
  try {
    if (saveFirst) await saveAll()
    await what()
    status.value = await ProgramService.Status(props.plc)
  } catch (e) {
    fail(e)
  } finally {
    busy.value = false
  }
}

function showBuild(r: { ok: boolean; diagnostics: Diagnostic[] | null }) {
  diagnostics.value = r.diagnostics ?? []
  built.value = r.ok ? 'ok' : 'failed'
  if (!r.ok || diagnostics.value.length) bottom.value = 'messages'
}

const build = () => run(async () => showBuild(await ProgramService.Build(props.plc)), true)

// Download builds, loads the program into the soft PLC, and connects the simulation to it.
const download = () =>
  run(async () => {
    const r = await ProgramService.Download(props.plc)
    showBuild(r)
    if (!r.ok) return
    downloads.value++
    const s = await ProgramService.Status(props.plc)
    if (!s.connected) await ProgramService.Connect(props.plc)
    bottom.value = 'watch'
  }, true)

const start = () => run(() => ProgramService.Start(props.plc))
const stop = () => run(() => ProgramService.Stop(props.plc))
const reset = () => run(() => ProgramService.Reset(props.plc, true))
const connect = () => run(() => ProgramService.Connect(props.plc))

async function jump(d: Diagnostic) {
  if (!d.pos.file || d.pos.file === 'task') {
    bottom.value = 'task'
    return
  }
  await openFile(d.pos.file)
  const t = tabs.value.find((x) => x.name === d.pos.file)
  if (t) t.goto = { line: d.pos.line, col: d.pos.col, n: (t.goto?.n ?? 0) + 1 }
}

const marksOf = (name: string) => diagnostics.value.filter((d) => d.pos.file === name).map((d) => ({ line: d.pos.line, severity: d.severity as string, msg: d.msg }))

// The program's variables that have a value of their own, by their path, to show in the code.
const values = computed(() => {
  const m = new Map<string, WatchVar>()
  const walk = (list: WatchVar[] | null | undefined) => {
    for (const v of list ?? []) {
      if (v.value !== '' && !v.children?.length) m.set(v.path.toUpperCase(), v)
      walk(v.children)
    }
  }
  walk(vars.value)
  return m
})
const lists = computed(() => vars.value.filter((v) => v.type === 'GVL').map((v) => v.name))

// scope is whose variables the open file shows, as CODESYS shows a POU online: a program's
// own, or for a function block, those of its first instance.
const scope = computed(() => {
  const d = tab.value ? declared(tab.value.text) : null
  if (d?.kind === 'PROGRAM') return d.name
  if (d?.kind === 'FUNCTION_BLOCK') {
    const find = (list: WatchVar[] | null | undefined): string => {
      for (const v of list ?? []) {
        if (v.type.toUpperCase() === d.name.toUpperCase()) return v.path
        const inner = find(v.children)
        if (inner) return inner
      }
      return ''
    }
    return find(vars.value)
  }
  if (d?.kind === 'GVL') return tab.value!.name.replace(/\.st$/, '')
  return ''
})

function monitor(name: string): Monitored | null {
  const candidates: string[] = []
  if (scope.value) candidates.push(`${scope.value}.${name}`)
  for (const l of lists.value) candidates.push(`${l}.${name}`)
  candidates.push(name)
  for (const c of candidates) {
    const v = values.value.get(c.toUpperCase())
    if (v) return { path: v.path, value: v.value, type: v.type, forced: v.forced, writable: v.writable }
  }
  return null
}

function prepare(path: string, value: string | null) {
  if (value === null) delete prepared[path]
  else prepared[path] = value
}

// writeAll writes every prepared value once (Ctrl+F7), forceAll forces them (F7).
const writeAll = () => applyAll(ProgramService.Write)
const forceAll = () => applyAll(ProgramService.Force)
async function applyAll(how: (plc: string, path: string, value: string) => Promise<void>) {
  const list = Object.entries(prepared)
  if (!list.length) return
  await run(async () => {
    for (const [path, value] of list) {
      await how(props.plc, path, value)
      delete prepared[path]
    }
  })
}

async function write(path: string) {
  await run(() => ProgramService.Write(props.plc, path, prepared[path] ?? ''))
  delete prepared[path]
}

async function force(path: string) {
  await run(() => ProgramService.Force(props.plc, path, prepared[path] ?? ''))
  delete prepared[path]
}

const release = (path: string) => run(() => ProgramService.Release(props.plc, path))

function toggle(path: string) {
  if (open.has(path)) open.delete(path)
  else open.add(path)
}

const KINDS = ['PROGRAM', 'FUNCTION_BLOCK', 'FUNCTION', 'GVL', 'TYPE']
const kindLabel: Record<string, string> = { PROGRAM: 'PRG', FUNCTION_BLOCK: 'FB', FUNCTION: 'FUN', GVL: 'GVL', TYPE: 'DUT', EMPTY: '—' }

async function addFile() {
  const a = adding.value
  if (!a?.name.trim()) return
  try {
    const name = await ProgramService.AddFile(props.plc, a.name.trim(), a.kind)
    adding.value = null
    await refreshFiles()
    await openFile(name)
  } catch (e) {
    fail(e)
  }
}

async function removeFile(name: string) {
  if (!confirm(`Delete ${name}?`)) return
  try {
    await ProgramService.RemoveFile(props.plc, name)
    tabs.value = tabs.value.filter((t) => t.name !== name)
    if (active.value === name) active.value = tabs.value[0]?.name ?? ''
    await refreshFiles()
  } catch (e) {
    fail(e)
  }
}

// loadExample copies the example program of the station's collection into the project,
// over the files of the same names, after asking.
async function loadExample() {
  const names = project.value?.example ?? []
  if (!names.length || !confirm(`Load the example program of this station? It replaces ${names.join(', ')} in your project.`)) return
  try {
    const written = (await ProgramService.LoadExample(props.plc)) ?? []
    for (const t of tabs.value) {
      if (!written.includes(t.name)) continue
      const text = await ProgramService.ReadFile(props.plc, t.name)
      t.text = text
      t.saved = text
    }
    await refreshFiles()
    message.value = ''
  } catch (e) {
    fail(e)
  }
}

async function renameFile() {
  const r = renaming.value
  if (!r?.to.trim()) return
  try {
    const t = tabs.value.find((x) => x.name === r.from)
    if (t) await save(t)
    await ProgramService.RenameFile(props.plc, r.from, r.to.trim())
    const to = r.to.trim().endsWith('.st') ? r.to.trim() : r.to.trim() + '.st'
    if (t) t.name = to
    if (active.value === r.from) active.value = to
    renaming.value = null
    await refreshFiles()
  } catch (e) {
    fail(e)
  }
}

async function saveTask() {
  await run(() =>
    ProgramService.SetTask(props.plc, {
      programs: task.programs
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean),
      cycle: task.cycle.trim(),
      step: task.step.trim(),
    }),
  )
}

// The PLC's state and its variables come every moment, as a programming tool online shows.
let timer = 0
async function poll() {
  try {
    status.value = await ProgramService.Status(props.plc)
    if (project.value && status.value.station !== project.value.station) return await follow()
    if (status.value.loaded && (bottom.value === 'watch' || tab.value)) vars.value = (await ProgramService.Variables(props.plc)) ?? []
  } catch {
    // The station may be changing; the next round tries again.
  }
}

// follow opens the project of the station the simulation changed to, after saving what was
// typed into the one it left.
let following = false
async function follow() {
  if (following) return
  following = true
  try {
    await saveAll()
    tabs.value = []
    diagnostics.value = []
    vars.value = []
    message.value = 'The simulation changed to another station: this is the program of its PLC.'
    await load()
  } finally {
    following = false
  }
}

function onKey(e: KeyboardEvent) {
  const keys: Record<string, () => unknown> = { F11: build, F5: start }
  if (e.key === 'F7') {
    // As CODESYS: F7 forces the prepared values, Ctrl+F7 writes them, Alt+F7 releases all.
    e.preventDefault()
    if (!online.value) return
    if (e.altKey) void release('')
    else if (e.ctrlKey || e.metaKey) void writeAll()
    else void forceAll()
  } else if (e.key === 'F8' && e.altKey) {
    e.preventDefault()
    void download()
  } else if (e.key === 'F8' && e.shiftKey) {
    e.preventDefault()
    void stop()
  } else if (keys[e.key]) {
    e.preventDefault()
    void keys[e.key]()
  }
}

onMounted(() => {
  void load()
  timer = window.setInterval(poll, 300)
  window.addEventListener('keydown', onKey)
})
onUnmounted(() => {
  window.clearInterval(timer)
  window.removeEventListener('keydown', onKey)
})
watch(
  () => props.plc,
  () => {
    tabs.value = []
    void load()
  },
)

const stateLabel = computed(() => {
  const s = status.value
  if (!s?.loaded) return 'No program'
  return { running: 'Running', stopped: 'Stopped', failed: 'Error' }[s.state] ?? s.state
})
</script>

<template>
  <div class="ide" :class="{ window }">
    <header class="toolbar">
      <strong class="title">{{ project?.title ?? 'PLC' }}</strong>
      <div class="group">
        <button :disabled="busy" title="Build the project (F11)" @click="build">Build</button>
        <button :disabled="busy" class="primary" title="Build, load into the soft PLC and connect the simulation (Alt+F8)" @click="download">Download</button>
      </div>
      <div class="group">
        <button :disabled="busy || !online || running" title="Run the program (F5)" @click="start">▶ Run</button>
        <button :disabled="busy || !running" title="Hold the program (Shift+F8)" @click="stop">■ Stop</button>
        <button :disabled="busy || !online" title="Every variable to its initial value, stopped" @click="reset">Reset</button>
      </div>
      <span class="state" :class="status?.state" :title="status?.endpoint ? `OPC UA ${status.endpoint}` : ''">
        <span class="dot" />{{ stateLabel }}<template v-if="online"> · {{ status?.cycles }} cycles</template>
      </span>
      <button v-if="online && !status?.connected" :disabled="busy" title="Connect the simulation to this PLC over OPC UA" @click="connect">Connect simulation</button>
      <span v-else-if="status?.connected" class="linked">Simulation connected</span>
      <span class="spacer" />
      <button v-if="!window" title="Open in a window of its own, beside the simulation" @click="ProgramService.OpenWindow(plc).catch(fail)">Open in window</button>
      <button v-if="!window" class="close" title="Close the editor" @click="emit('close')">✕</button>
    </header>
    <p v-if="message" class="message">{{ message }}</p>
    <p v-if="status?.error" class="message">The program stopped at {{ status.error.pos.file }}:{{ status.error.pos.line }}: {{ status.error.msg }}. Reset it to start again.</p>
    <div class="body">
      <aside class="pous">
        <h3 class="section-title">POUs</h3>
        <ul>
          <li v-for="f in project?.files ?? []" :key="f.name" :class="{ active: f.name === active }">
            <template v-if="renaming?.from === f.name">
              <input v-model="renaming.to" class="mono rename" @keydown.enter="renameFile" @keydown.esc="renaming = null" />
            </template>
            <template v-else>
              <button class="file" :title="f.readOnly ? 'Written from the station\'s I/O table' : f.name" @click="openFile(f.name)">
                <span class="kind">{{ kindLabel[f.kind] ?? f.kind }}</span
                >{{ f.name.replace(/\.st$/, '') }}
              </button>
              <span v-if="!f.readOnly" class="file-actions">
                <button title="Rename" @click="renaming = { from: f.name, to: f.name.replace(/\.st$/, '') }">✎</button>
                <button title="Delete" @click="removeFile(f.name)">🗑</button>
              </span>
            </template>
          </li>
        </ul>
        <div v-if="adding" class="add">
          <input v-model="adding.name" class="mono" placeholder="Name" @keydown.enter="addFile" @keydown.esc="adding = null" />
          <select v-model="adding.kind">
            <option v-for="k in KINDS" :key="k" :value="k">{{ k }}</option>
          </select>
          <div class="row">
            <button @click="addFile">Add</button>
            <button @click="adding = null">Cancel</button>
          </div>
        </div>
        <button v-else class="new" @click="adding = { name: '', kind: 'PROGRAM' }">+ New POU</button>
        <button v-if="project?.example?.length" class="new example" title="The program the station's collection comes with, to try it out or to start from" @click="loadExample">
          Load the example program
        </button>
        <p class="folder" :title="project?.folder">Saved in your folder, under programs/{{ project?.station }}/{{ project?.plc }}</p>
      </aside>
      <section class="main">
        <nav class="tabs">
          <button v-for="t in tabs" :key="t.name" class="tab" :class="{ active: t.name === active }" @click="active = t.name">
            {{ t.name.replace(/\.st$/, '') }}<span v-if="dirty(t)" class="dirty">●</span>
            <span class="x" @click.stop="closeTab(t.name)">✕</span>
          </button>
        </nav>
        <div class="editor-area">
          <StEditor
            v-if="tab"
            :key="tab.name"
            v-model="tab.text"
            :read-only="tab.readOnly"
            :marks="marksOf(tab.name)"
            :monitor="online ? monitor : undefined"
            :prepared="prepared"
            :scope="scope"
            :downloads="downloads"
            :goto="tab.goto"
            :complete="suggestions"
            @save="save(tab)"
            @prepare="prepare"
          />
          <p v-else class="empty">Open a POU on the left.</p>
        </div>
        <div class="bottom">
          <nav class="tabs small">
            <button class="tab" :class="{ active: bottom === 'messages' }" @click="bottom = 'messages'">
              Messages<span v-if="diagnostics.length" class="count">{{ diagnostics.length }}</span>
            </button>
            <button class="tab" :class="{ active: bottom === 'watch' }" @click="bottom = 'watch'">Watch</button>
            <button class="tab" :class="{ active: bottom === 'task' }" @click="bottom = 'task'">Task</button>
          </nav>
          <div v-if="bottom === 'messages'" class="panel">
            <p v-if="built === 'ok' && !diagnostics.length" class="ok">Built without errors.</p>
            <p v-else-if="!built" class="hint">Build with F11 to check the project.</p>
            <button v-for="(d, i) in diagnostics" :key="i" class="diag" :class="d.severity" @click="jump(d)">
              <span class="sev">{{ d.severity }}</span>
              <span class="mono where">{{ d.pos.file }}{{ d.pos.line ? `:${d.pos.line}:${d.pos.col}` : '' }}</span>
              {{ d.msg }}
            </button>
          </div>
          <div v-else-if="bottom === 'watch'" class="panel">
            <p v-if="!online" class="hint">Download the program to watch its variables.</p>
            <template v-else>
              <div class="watch-actions">
                <span class="hint">Prepare a value here or by clicking it in the code, then write it once (Ctrl+F7) or force it (F7). Alt+F7 releases all.</span>
                <span class="group">
                  <button :disabled="busy || !Object.keys(prepared).length" @click="writeAll">Write all</button>
                  <button :disabled="busy || !Object.keys(prepared).length" @click="forceAll">Force all</button>
                  <button v-if="status?.forced?.length" @click="release('')">Release all ({{ status.forced.length }})</button>
                </span>
              </div>
              <table class="watch">
                <thead>
                  <tr>
                    <th>Expression</th>
                    <th>Type</th>
                    <th>Value</th>
                    <th>Prepared value</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  <WatchTree :vars="vars" :depth="0" :open="open" :prepared="prepared" @toggle="toggle" @write="write" @force="force" @release="release" />
                </tbody>
              </table>
            </template>
          </div>
          <div v-else class="panel task">
            <label>Programs the task calls, in order <input v-model="task.programs" class="mono" placeholder="PLC_PRG" /></label>
            <label>Interval <input v-model="task.cycle" class="mono" placeholder="T#20ms" /></label>
            <label>Step shown in the simulation <input v-model="task.step" class="mono" placeholder="GVL.currentStep" /></label>
            <button :disabled="busy" @click="saveTask">Save task</button>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.ide {
  background: var(--bg);
  display: flex;
  flex-direction: column;
  height: 100%;
  inset: 0;
  min-height: 0;
  position: absolute;
  z-index: 5;
}

.ide.window {
  position: fixed;
}

.toolbar {
  align-items: center;
  background: var(--panel);
  border-bottom: 1px solid var(--line);
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  padding: 6px 10px;
}

.title {
  margin-right: 6px;
}

.group {
  display: flex;
  gap: 4px;
}

.primary {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--bg);
  font-weight: 600;
}

.state {
  align-items: center;
  color: var(--text-dim);
  display: flex;
  gap: 6px;
}

.dot {
  background: var(--text-dim);
  border-radius: 50%;
  height: 9px;
  width: 9px;
}

.state.running .dot {
  background: var(--signal);
}

.state.failed {
  color: var(--danger);
}

.state.failed .dot {
  background: var(--danger);
}

.state.stopped .dot {
  background: var(--accent);
}

.linked {
  color: var(--signal);
  font-size: 12px;
}

.spacer {
  flex: 1;
}

.close {
  padding: 4px 8px;
}

.message {
  background: rgba(255, 122, 102, 0.12);
  border-bottom: 1px solid var(--line);
  color: var(--danger);
  margin: 0;
  padding: 5px 10px;
}

.body {
  display: flex;
  flex: 1;
  min-height: 0;
}

.pous {
  background: var(--panel);
  border-right: 1px solid var(--line);
  display: flex;
  flex: none;
  flex-direction: column;
  overflow: auto;
  padding: 8px;
  width: 210px;
}

.pous ul {
  list-style: none;
  margin: 0 0 8px;
  padding: 0;
}

.pous li {
  align-items: center;
  border-radius: 2px;
  display: flex;
}

.pous li.active {
  background: var(--panel-raised);
}

.file {
  background: none;
  border: 0;
  flex: 1;
  overflow: hidden;
  padding: 4px 6px;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.kind {
  color: var(--text-dim);
  display: inline-block;
  font-size: 10px;
  width: 30px;
}

.file-actions {
  display: none;
}

.pous li:hover .file-actions {
  display: flex;
}

.file-actions button {
  background: none;
  border: 0;
  padding: 2px 4px;
}

.rename,
.add input,
.add select {
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: 2px;
  padding: 3px 5px;
  width: 100%;
}

.add {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.row {
  display: flex;
  gap: 4px;
}

.new {
  align-self: flex-start;
}

.example {
  margin-top: 6px;
}

.folder {
  color: var(--text-dim);
  font-size: 11px;
  margin-top: auto;
}

.main {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
}

.tabs {
  background: var(--panel);
  border-bottom: 1px solid var(--line);
  display: flex;
  flex: none;
  overflow-x: auto;
}

.tab {
  align-items: center;
  background: none;
  border: 0;
  border-right: 1px solid var(--line);
  border-radius: 0;
  display: flex;
  gap: 6px;
  padding: 6px 10px;
  white-space: nowrap;
}

.tab.active {
  background: #0d2326;
  box-shadow: inset 0 2px 0 var(--accent);
}

.dirty {
  color: var(--accent);
}

.x {
  color: var(--text-dim);
  font-size: 10px;
}

.editor-area {
  flex: 1;
  min-height: 0;
}

.empty,
.hint {
  color: var(--text-dim);
  margin: 10px;
}

.bottom {
  border-top: 1px solid var(--line);
  display: flex;
  flex: none;
  flex-direction: column;
  height: 34%;
  min-height: 140px;
}

.tabs.small .tab {
  padding: 4px 10px;
}

.count {
  background: var(--danger);
  border-radius: 8px;
  color: var(--bg);
  font-size: 10px;
  padding: 0 5px;
}

.panel {
  flex: 1;
  overflow: auto;
}

.ok {
  color: var(--signal);
  margin: 10px;
}

.diag {
  background: none;
  border: 0;
  border-bottom: 1px solid rgba(35, 74, 80, 0.5);
  border-radius: 0;
  display: block;
  padding: 4px 10px;
  text-align: left;
  user-select: text;
  width: 100%;
}

.sev {
  display: inline-block;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  width: 62px;
}

.diag.error .sev {
  color: var(--danger);
}

.diag.warning .sev {
  color: var(--accent);
}

.where {
  color: var(--text-dim);
  margin-right: 8px;
}

.watch-actions {
  align-items: center;
  display: flex;
  justify-content: space-between;
  padding: 0 10px 0 0;
}

.watch {
  border-collapse: collapse;
  width: 100%;
}

.watch th {
  background: var(--panel);
  color: var(--text-dim);
  font-size: 11px;
  font-weight: 500;
  padding: 3px 6px;
  position: sticky;
  text-align: left;
  top: 0;
}

.task {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-width: 460px;
  padding: 10px;
}

.task label {
  color: var(--text-dim);
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.task input {
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: 2px;
  color: var(--text);
  padding: 4px 6px;
}

.task button {
  align-self: flex-start;
}
</style>
