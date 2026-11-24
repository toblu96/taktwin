<script setup lang="ts">
import { ask } from '../ask'
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { ProgramService, Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { EditorState, ProgramProject, ProgramStatus } from '../../bindings/github.com/toblu96/taktwin/internal/simapp/models'
import type { Diagnostic, WatchVar } from '../../bindings/github.com/toblu96/taktwin/st/models'
import { putBack } from '../original'
import UiMenu, { type MenuEntry } from '../ui/UiMenu.vue'
import UiSelect from '../ui/UiSelect.vue'
import UiTabs from '../ui/UiTabs.vue'
import UiTree, { type TreeRow } from '../ui/UiTree.vue'
import { complete, parseUnit } from './complete'
import StEditor from './StEditor.vue'
import WatchTree from './WatchTree.vue'
import { declared, type Monitored } from './st'
import IconBuild from '~icons/fluent/wrench-16-regular'
import IconDownload from '~icons/fluent/arrow-download-16-regular'
import IconRun from '~icons/fluent/play-16-filled'
import IconStop from '~icons/fluent/stop-16-filled'
import IconReset from '~icons/fluent/arrow-counterclockwise-16-regular'
import IconFormat from '~icons/fluent/text-align-left-16-regular'
import IconWindow from '~icons/fluent/window-new-16-regular'
import IconClose from '~icons/fluent/dismiss-16-regular'
import IconAdd from '~icons/fluent/add-16-regular'
import IconProgram from '~icons/fluent/code-16-regular'
import IconBlock from '~icons/fluent/cube-16-regular'
import IconFunction from '~icons/fluent/math-formula-16-regular'
import IconGlobal from '~icons/fluent/globe-16-regular'
import IconType from '~icons/fluent/braces-16-regular'
import IconIO from '~icons/fluent/plug-connected-16-regular'
import IconRename from '~icons/fluent/rename-16-regular'
import IconDelete from '~icons/fluent/delete-16-regular'
import IconExample from '~icons/fluent/document-16-regular'
import IconUndo from '~icons/fluent/arrow-undo-16-regular'
import IconLink from '~icons/fluent/link-16-regular'

// The programming tool of a PLC, as CODESYS has it: the project's POUs, an editor for each,
// building and downloading into the app's soft PLC, running it, and watching, writing and
// forcing its variables online. It opens again as it was left in the project, with the same
// files open and the same one in front.
const props = defineProps<{ plc: string; window?: boolean }>()
const emit = defineEmits<{ close: [] }>()

interface Tab {
  name: string
  text: string
  saved: string
  readOnly: boolean
  goto: { line: number; col: number; n: number } | null
  place: { cursor: number; top: number } | null
}

const project = ref<ProgramProject | null>(null)
const status = ref<ProgramStatus | null>(null)
const tabs = ref<Tab[]>([])
const active = ref('')
const diagnostics = ref<Diagnostic[]>([])
const built = ref<'ok' | 'failed' | ''>('')
const message = ref('')
const busy = ref(false)
const bottom = ref<string>('messages')
const vars = ref<WatchVar[]>([])
const open = reactive(new Set<string>())
const prepared = reactive<Record<string, string>>({})
const adding = ref<{ name: string; kind: string } | null>(null)
const renaming = ref<{ from: string; to: string } | null>(null)
const task = reactive({ programs: '', cycle: '', step: '' })
const downloads = ref(0)
// The text of every file as saved, for completion across the project.
const sources = reactive<Record<string, string>>({})
// Whether the programs differ from the project's original, so that they can be put back.
const changedFromOriginal = ref(false)

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
    if (!tabs.value.length) await restore()
    await loadSources()
    await checkOriginal()
  } catch (e) {
    fail(e)
  }
}

// restore opens the files that were open when the tool was last left in this project, with
// the one in front and the place in each; or the PLC's main program the first time.
async function restore() {
  let state: EditorState | null = null
  try {
    state = await ProgramService.EditorState(props.plc)
  } catch {
    // Without one, it starts as new.
  }
  const names = new Set((project.value?.files ?? []).map((f) => f.name))
  const openNames = (state?.open ?? []).filter((n) => names.has(n))
  if (!openNames.length) openNames.push('PLC_PRG.st')
  for (const name of openNames) await openFile(name, state?.places?.[name] ?? null)
  if (state?.active && openNames.includes(state.active)) active.value = state.active
  restored = true
}

// Where the tool is, kept for the next time, shortly after it changes.
let restored = false
let stateTimer = 0
watch(
  () => [tabs.value.map((t) => [t.name, t.place?.cursor, t.place?.top]), active.value],
  () => {
    if (!restored) return
    window.clearTimeout(stateTimer)
    stateTimer = window.setTimeout(() => {
      const places = Object.fromEntries(tabs.value.filter((t) => t.place).map((t) => [t.name, t.place!]))
      void ProgramService.SetEditorState(props.plc, { open: tabs.value.map((t) => t.name), active: active.value, places }).catch(() => undefined)
    }, 400)
  },
  { deep: true },
)

async function refreshFiles() {
  try {
    project.value = await ProgramService.Open(props.plc)
    await loadSources()
    await checkOriginal()
  } catch (e) {
    fail(e)
  }
}

async function loadSources() {
  const names = new Set((project.value?.files ?? []).map((f) => f.name))
  for (const k of Object.keys(sources)) if (!names.has(k)) delete sources[k]
  for (const name of names) sources[name] = await ProgramService.ReadFile(props.plc, name)
}

async function checkOriginal() {
  try {
    const o = await Service.Original()
    changedFromOriginal.value = !!o.at && (o.programs ?? []).includes(project.value?.plc ?? '')
  } catch {
    changedFromOriginal.value = false
  }
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

async function openFile(name: string, place: Tab['place'] = null) {
  if (!tabs.value.some((t) => t.name === name)) {
    try {
      const text = await ProgramService.ReadFile(props.plc, name)
      const file = project.value?.files?.find((f) => f.name === name)
      tabs.value.push({ name, text, saved: text, readOnly: !!file?.readOnly, goto: null, place })
    } catch (e) {
      fail(e)
      return
    }
  }
  active.value = name
}

async function closeTab(name: string) {
  const t = tabs.value.find((x) => x.name === name)
  if (t && dirty(t) && !(await ask(`${name} has changes that are not saved. Close it anyway?`, 'Close'))) return
  const i = tabs.value.findIndex((x) => x.name === name)
  tabs.value = tabs.value.filter((x) => x.name !== name)
  if (active.value === name) active.value = tabs.value[Math.min(i, tabs.value.length - 1)]?.name ?? ''
}

async function save(t: Tab) {
  if (t.readOnly || !dirty(t)) return
  // Into the project the tab comes from, also if the simulation has changed station since.
  await ProgramService.SaveFile(project.value!.project, project.value!.plc, t.name, t.text)
  t.saved = t.text
  sources[t.name] = t.text
  void checkOriginal()
}

async function saveAll() {
  for (const t of tabs.value) await save(t)
}

// format lays out the file in front as CODESYS's Format Document does; a file with a syntax
// error stays as it is.
async function format() {
  const t = tab.value
  if (!t || t.readOnly) return
  try {
    t.text = await ProgramService.Format(t.name, t.text)
  } catch (e) {
    message.value = `Format Document needs code without syntax errors: ${(e as Error).message ?? e}`
  }
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

// The project is checked as it is typed, without saving, so that mistakes are underlined.
let checkTimer = 0
watch(
  () => tabs.value.map((t) => t.text),
  () => {
    window.clearTimeout(checkTimer)
    checkTimer = window.setTimeout(async () => {
      const texts = Object.fromEntries(tabs.value.filter((t) => !t.readOnly).map((t) => [t.name, t.text]))
      try {
        const r = await ProgramService.Check(props.plc, texts)
        diagnostics.value = r.diagnostics ?? []
        built.value = r.ok ? 'ok' : 'failed'
      } catch {
        // A station that is changing is checked again with the next change.
      }
    }, 500)
  },
  { deep: true },
)

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

const marksOf = (name: string) =>
  diagnostics.value.filter((d) => d.pos.file === name).map((d) => ({ line: d.pos.line, col: d.pos.col, severity: d.severity as string, msg: d.msg }))

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

const KINDS = [
  { value: 'PROGRAM', label: 'Program' },
  { value: 'FUNCTION_BLOCK', label: 'Function block' },
  { value: 'FUNCTION', label: 'Function' },
  { value: 'GVL', label: 'Global variable list' },
  { value: 'TYPE', label: 'Data type' },
]
const kindLabel: Record<string, string> = { PROGRAM: 'PRG', FUNCTION_BLOCK: 'FB', FUNCTION: 'FUN', GVL: 'GVL', TYPE: 'DUT', EMPTY: '—' }
const kindIcon: Record<string, typeof IconProgram> = { PROGRAM: IconProgram, FUNCTION_BLOCK: IconBlock, FUNCTION: IconFunction, GVL: IconGlobal, TYPE: IconType }

// The POUs as a tree, the one in front selected; the I/O list written from the station shows
// as such.
const pous = computed<TreeRow[]>(() =>
  (project.value?.files ?? []).map((f) => ({
    key: f.name,
    label: f.name.replace(/\.st$/, ''),
    icon: f.readOnly ? IconIO : (kindIcon[f.kind] ?? IconProgram),
    hint: f.readOnly ? 'I/O' : (kindLabel[f.kind] ?? f.kind),
    selected: f.name === active.value,
    title: f.readOnly ? "Written from the station's I/O table" : f.name,
  })),
)
const activeFile = computed(() => project.value?.files?.find((f) => f.name === active.value) ?? null)

const pouMenu = computed<MenuEntry[]>(() => {
  const f = activeFile.value
  return [
    ...(f && !f.readOnly
      ? [
          { label: `Rename ${f.name.replace(/\.st$/, '')}`, icon: IconRename, run: () => (renaming.value = { from: f.name, to: f.name.replace(/\.st$/, '') }) },
          { label: `Delete ${f.name.replace(/\.st$/, '')}…`, icon: IconDelete, danger: true, run: () => void removeFile(f.name) },
          {},
        ]
      : []),
    ...(project.value?.example?.length ? [{ label: 'Load the example program…', icon: IconExample, run: () => void loadExample() }] : []),
    { label: 'Put back the programs…', icon: IconUndo, disabled: !changedFromOriginal.value, run: () => void putBackPrograms() },
  ]
})

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
  if (!(await ask(`Delete ${name}?`, 'Delete'))) return
  try {
    await ProgramService.RemoveFile(props.plc, name)
    tabs.value = tabs.value.filter((t) => t.name !== name)
    if (active.value === name) active.value = tabs.value[0]?.name ?? ''
    await refreshFiles()
  } catch (e) {
    fail(e)
  }
}

// reread takes the open files as they are saved now, after something else wrote them.
async function reread() {
  await refreshFiles()
  const names = new Set((project.value?.files ?? []).map((f) => f.name))
  tabs.value = tabs.value.filter((t) => names.has(t.name))
  for (const t of tabs.value) {
    const text = await ProgramService.ReadFile(props.plc, t.name)
    t.text = text
    t.saved = text
  }
  if (!tabs.value.some((t) => t.name === active.value)) active.value = tabs.value[0]?.name ?? ''
  if (!tabs.value.length && names.has('PLC_PRG.st')) await openFile('PLC_PRG.st')
}

// loadExample copies the example program of the station's collection into the project,
// over the files of the same names, after asking.
async function loadExample() {
  const names = project.value?.example ?? []
  if (!names.length || !(await ask(`Load the example program of this station? It replaces ${names.join(', ')} in your project.`, 'Load the example'))) return
  try {
    await ProgramService.LoadExample(props.plc)
    await reread()
    message.value = ''
  } catch (e) {
    fail(e)
  }
}

// putBackPrograms puts the PLC's programs back as the project's original has them, after
// asking.
async function putBackPrograms() {
  try {
    await saveAll()
    await putBack('programs', project.value?.plc ?? props.plc, 'the programs of this PLC')
    await reread()
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
    if (project.value && status.value.project !== project.value.project) return await follow()
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
    restored = false
    tabs.value = []
    diagnostics.value = []
    vars.value = []
    message.value = 'The simulation changed to another project: this is the program of its PLC.'
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
  window.clearTimeout(checkTimer)
  window.clearTimeout(stateTimer)
  window.removeEventListener('keydown', onKey)
})
watch(
  () => props.plc,
  () => {
    restored = false
    tabs.value = []
    void load()
  },
)

const stateLabel = computed(() => {
  const s = status.value
  if (!s?.loaded) return 'No program loaded'
  return { running: 'Running', stopped: 'Stopped', failed: 'Error' }[s.state] ?? s.state
})
const lamp = computed(() => (status.value?.state === 'running' ? 'bg-ok' : status.value?.state === 'failed' ? 'bg-danger' : status.value?.loaded ? 'bg-accent' : 'bg-faint'))

const bottomTabs = computed(() => [
  { value: 'messages', label: 'Messages', count: diagnostics.value.length },
  { value: 'watch', label: 'Watch' },
  { value: 'task', label: 'Task' },
])
const btn = 't-btn h-7 gap-1.5 px-2'
</script>

<template>
  <div class="z-20 flex min-h-0 flex-col bg-s0" :class="window ? 'fixed inset-0' : 'absolute inset-0'">
    <header class="flex shrink-0 flex-wrap items-center gap-2 border-0 border-b border-solid border-line bg-s1 px-3 py-1.5">
      <h2 class="m-0 mr-1 truncate text-[13px] font-semibold">{{ project?.title ?? 'PLC' }}</h2>
      <button :class="btn" :disabled="busy" title="Build the project (F11)" @click="build"><IconBuild class="size-4" />Build</button>
      <button :class="btn" class="t-btn-primary" :disabled="busy" title="Build, load into the soft PLC and connect the simulation (Alt+F8)" @click="download">
        <IconDownload class="size-4" />Download
      </button>
      <span class="mx-1 h-4 w-px bg-line" />
      <button :class="btn" :disabled="busy || !online || running" title="Run the program (F5)" @click="start"><IconRun class="size-4 text-ok" />Run</button>
      <button :class="btn" :disabled="busy || !running" title="Hold the program (Shift+F8)" @click="stop"><IconStop class="size-4" />Stop</button>
      <button :class="btn" :disabled="busy || !online" title="Every variable to its initial value, stopped" @click="reset"><IconReset class="size-4" />Reset</button>
      <span class="mx-1 h-4 w-px bg-line" />
      <button :class="btn" :disabled="!tab || tab.readOnly" title="Format Document (Shift+Alt+F): capitals, spaces and indentation as CODESYS lays them out" @click="format">
        <IconFormat class="size-4" />Format
      </button>
      <span class="ml-1 flex items-center gap-1.5 text-xs text-muted" role="status" :title="status?.endpoint ? `OPC UA ${status.endpoint}` : ''">
        <span class="size-2 rounded-full" :class="lamp" />{{ stateLabel }}<template v-if="online"> · {{ status?.cycles }} cycles</template>
      </span>
      <button v-if="online && !status?.connected" :class="btn" :disabled="busy" title="Connect the simulation to this PLC over OPC UA" @click="connect">
        <IconLink class="size-4" />Connect simulation
      </button>
      <span v-else-if="status?.connected" class="text-xs text-muted">· Simulation connected</span>
      <span class="flex-1" />
      <button v-if="!window" :class="btn" title="Open in a window of its own, beside the simulation" @click="ProgramService.OpenWindow(plc).catch(fail)">
        <IconWindow class="size-4" />Open in window
      </button>
      <button v-if="!window" class="t-icon-btn" title="Close the editor" aria-label="Close the editor" @click="emit('close')"><IconClose class="size-4" /></button>
    </header>
    <p v-if="message" class="m-0 shrink-0 border-0 border-b border-solid border-line bg-s1 px-3 py-1.5 text-danger" role="alert">{{ message }}</p>
    <p v-if="status?.error" class="m-0 shrink-0 border-0 border-b border-solid border-line bg-s1 px-3 py-1.5 text-danger" role="alert">
      The program stopped at {{ status.error.pos.file }}:{{ status.error.pos.line }}: {{ status.error.msg }}. Reset it to start again.
    </p>
    <div class="flex min-h-0 flex-1">
      <aside class="flex w-56 shrink-0 flex-col border-0 border-r border-solid border-line bg-s1" aria-label="POUs">
        <div class="flex h-9 shrink-0 items-center gap-1 pr-1.5 pl-3">
          <h3 class="t-section-title m-0 flex-1">POUs</h3>
          <button class="t-icon-btn size-7" title="New POU" aria-label="New POU" @click="adding = { name: '', kind: 'PROGRAM' }"><IconAdd class="size-4" /></button>
          <UiMenu :items="pouMenu" label="More for the POUs" class="size-7" />
        </div>
        <div class="min-h-0 flex-1 overflow-auto px-1 pb-2">
          <UiTree :items="pous" label="POUs" @select="(name: string) => openFile(name)" />
        </div>
        <form v-if="adding" class="flex flex-col gap-2 border-0 border-t border-solid border-line p-3" @submit.prevent="addFile">
          <label class="flex flex-col gap-1">
            <span class="t-label">Name of the new POU</span>
            <input v-model="adding.name" class="t-input font-mono" placeholder="Name" spellcheck="false" @keydown.esc="adding = null" />
          </label>
          <UiSelect v-model="adding.kind" label="Kind" :options="KINDS" />
          <div class="flex gap-2">
            <button class="t-btn t-btn-primary" type="submit">Add</button>
            <button class="t-btn" type="button" @click="adding = null">Cancel</button>
          </div>
        </form>
        <form v-else-if="renaming" class="flex flex-col gap-2 border-0 border-t border-solid border-line p-3" @submit.prevent="renameFile">
          <label class="flex flex-col gap-1">
            <span class="t-label">Rename {{ renaming.from.replace(/\.st$/, '') }} to</span>
            <input v-model="renaming.to" class="t-input font-mono" spellcheck="false" @keydown.esc="renaming = null" />
          </label>
          <div class="flex gap-2">
            <button class="t-btn t-btn-primary" type="submit">Rename</button>
            <button class="t-btn" type="button" @click="renaming = null">Cancel</button>
          </div>
        </form>
        <p class="m-0 shrink-0 truncate border-0 border-t border-solid border-line px-3 py-1.5 text-xs text-muted" :title="project?.folder">Saved in the project file</p>
      </aside>
      <section class="flex min-w-0 flex-1 flex-col">
        <nav class="flex h-9 shrink-0 items-end gap-0.5 overflow-x-auto border-0 border-b border-solid border-line bg-s1 px-2" aria-label="Open files">
          <div
            v-for="t in tabs"
            :key="t.name"
            class="relative -mb-px flex h-8 shrink-0 items-center rounded-t-[7px] border border-b-0 border-solid"
            :class="
              t.name === active
                ? 'border-line bg-s0 text-fg before:absolute before:inset-x-0 before:top-0 before:h-0.5 before:bg-accent'
                : 'border-transparent text-muted hover:bg-hover'
            "
          >
            <button
              class="flex h-full cursor-default items-center gap-1.5 border-0 bg-transparent py-0 pr-1 pl-3 text-inherit focus-visible:-outline-offset-2"
              :aria-current="t.name === active ? 'page' : undefined"
              @click="active = t.name"
            >
              <component :is="t.readOnly ? IconIO : IconProgram" class="size-3.5 text-muted" />{{ t.name.replace(/\.st$/, '') }}
              <span v-if="dirty(t)" class="size-2 rounded-full bg-accent" title="Not saved yet (Ctrl+S or ⌘S)" />
            </button>
            <button class="t-icon-btn mr-1 size-6" :aria-label="`Close ${t.name}`" :title="`Close ${t.name}`" @click="closeTab(t.name)"><IconClose class="size-3.5" /></button>
          </div>
        </nav>
        <div class="min-h-0 flex-1">
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
            :place="tab.place"
            @save="save(tab)"
            @format="format"
            @prepare="prepare"
            @place="(p) => (tab!.place = p)"
          />
          <p v-else class="m-0 p-6 text-muted">Open a POU on the left.</p>
        </div>
        <UiTabs v-model="bottom" :tabs="bottomTabs" label="Messages, watch and task" class="h-[38%] shrink-0 border-0 border-t border-solid border-line">
          <template #messages>
            <p v-if="built === 'ok' && !diagnostics.length" class="m-0 text-ok">No mistakes in the project.</p>
            <p v-else-if="!built" class="m-0 text-muted">The project is checked as you type; Build (F11) checks it as saved.</p>
            <ul class="m-0 flex list-none flex-col p-0">
              <li v-for="(d, i) in diagnostics" :key="i">
                <button
                  class="flex w-full cursor-default items-baseline gap-2 rounded-sm border-0 bg-transparent px-1.5 py-1 text-left text-fg hover:bg-hover focus-visible:-outline-offset-2"
                  @click="jump(d)"
                >
                  <span class="w-14 shrink-0 text-xs font-semibold" :class="d.severity === 'error' ? 'text-danger' : 'text-accent-ink'">{{ d.severity }}</span>
                  <span class="shrink-0 font-mono text-xs text-muted">{{ d.pos.file }}{{ d.pos.line ? `:${d.pos.line}:${d.pos.col}` : '' }}</span>
                  <span>{{ d.msg }}</span>
                </button>
              </li>
            </ul>
          </template>
          <template #watch>
            <p v-if="!online" class="m-0 text-muted">Download the program to watch its variables.</p>
            <template v-else>
              <div class="mb-2 flex flex-wrap items-center gap-2">
                <span class="flex-1 text-xs text-muted"
                  >Prepare a value here or by clicking it in the code, then write it once (Ctrl+F7) or force it (F7). Alt+F7 releases all.</span
                >
                <button :class="btn" :disabled="busy || !Object.keys(prepared).length" @click="writeAll">Write all</button>
                <button :class="btn" :disabled="busy || !Object.keys(prepared).length" @click="forceAll">Force all</button>
                <button v-if="status?.forced?.length" :class="btn" @click="release('')">Release all ({{ status.forced.length }})</button>
              </div>
              <table class="w-full border-collapse text-[13px]">
                <thead>
                  <tr class="text-left text-xs text-muted">
                    <th class="px-1.5 py-1 font-medium">Expression</th>
                    <th class="px-1.5 py-1 font-medium">Type</th>
                    <th class="px-1.5 py-1 font-medium">Value</th>
                    <th class="px-1.5 py-1 font-medium">Prepared value</th>
                    <th class="px-1.5 py-1"><span class="sr-only">Actions</span></th>
                  </tr>
                </thead>
                <tbody>
                  <WatchTree :vars="vars" :depth="0" :open="open" :prepared="prepared" @toggle="toggle" @write="write" @force="force" @release="release" />
                </tbody>
              </table>
            </template>
          </template>
          <template #task>
            <form class="flex max-w-lg flex-col gap-3" @submit.prevent="saveTask">
              <label class="flex flex-col gap-1">
                <span class="t-label">Programs the task calls, in order</span>
                <input v-model="task.programs" class="t-input font-mono" placeholder="PLC_PRG" spellcheck="false" />
              </label>
              <label class="flex flex-col gap-1">
                <span class="t-label">Interval</span>
                <input v-model="task.cycle" class="t-input font-mono" placeholder="T#20ms" spellcheck="false" />
              </label>
              <label class="flex flex-col gap-1">
                <span class="t-label">Step shown in the simulation</span>
                <input v-model="task.step" class="t-input font-mono" placeholder="GVL.currentStep" spellcheck="false" />
              </label>
              <button class="t-btn self-start" type="submit" :disabled="busy">Save task</button>
            </form>
          </template>
        </UiTabs>
      </section>
    </div>
  </div>
</template>
