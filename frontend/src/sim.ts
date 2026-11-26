import { Events } from '@wailsio/runtime'
import { computed, inject, ref, shallowRef, watch, type InjectionKey } from 'vue'
import { Service } from '../bindings/github.com/toblu96/taktwin/internal/simapp'
import { choose, tell } from './ask'
import { openSettings, router } from './router'
import type { Base, ProjectInfo, SaveState } from '../bindings/github.com/toblu96/taktwin/internal/simapp/models'
import type { Snapshot } from '../bindings/github.com/toblu96/taktwin/internal/engine'
import type { Scene, SceneInstance } from '../bindings/github.com/toblu96/taktwin/internal/model'

// What the shared components need from whatever runs behind them: the simulation,
// or a preview in the library.
export interface SimApi {
  RunAction(name: string, args: { [key: string]: string | undefined }): PromiseLike<unknown>
  SetControl(name: string, on: boolean): PromiseLike<unknown>
  SetFault(name: string, on: boolean): PromiseLike<unknown>
  SetParameter(id: string, value: number): PromiseLike<unknown>
  SetPin?(state: number, on: boolean): PromiseLike<unknown>
  // Taking a workpiece off by hand; the library has no such thing, so workpieces are not picked.
  TakeWorkpiece?(id: number): PromiseLike<unknown>
}

export function createSim(api: SimApi) {
  const scene = shallowRef<Scene | null>(null)

  // shallowRef on purpose: a new snapshot arrives 30 times a second and is only
  // ever replaced as a whole.
  const snapshot = shallowRef<Snapshot | null>(null)

  // The module instance that is selected. Null shows the station.
  const selection = ref<string | null>(null)

  // A station check the user picked, shown in the 3D view, or null.
  const highlight = shallowRef<{ instances: string[]; at: { x: number; y: number; z: number } | null } | null>(null)

  const selected = computed<SceneInstance | null>(() => scene.value?.instances?.find((i) => i.id === selection.value) ?? null)

  // The workpiece picked in the 3D view, by its id, or null. A module selected lets go of it,
  // and a workpiece picked lets go of the module.
  const workpiece = ref<number | null>(null)
  watch(
    selection,
    (id) => {
      if (id !== null) workpiece.value = null
    },
    { flush: 'sync' },
  )
  function pickWorkpiece(id: number | null) {
    if (id !== null) selection.value = null
    workpiece.value = id
  }
  const pickedWorkpiece = computed(() => (workpiece.value === null ? null : (snapshot.value?.workpieces?.find((w) => w.id === workpiece.value) ?? null)))

  // Takes the picked workpiece off the station, saying why if it may not be.
  async function takeWorkpiece() {
    const id = workpiece.value
    if (id === null || !api.TakeWorkpiece) return
    try {
      await api.TakeWorkpiece(id)
      workpiece.value = null
    } catch (error) {
      showMessage((error as Error).message)
    }
  }

  const stateOn = (state: number) => state >= 0 && (snapshot.value?.states?.[state] ?? 0) >= 0.5

  const stateOfControl = (name: string) => scene.value?.states?.indexOf(`${name}.active`) ?? -1

  // Holds a button while pressed; a key switch flips on press.
  function operate(name: string, down: boolean, toggle: boolean, state?: number) {
    if (toggle) {
      if (down) void api.SetControl(name, !stateOn(state ?? stateOfControl(name)))
      return
    }
    void api.SetControl(name, down)
  }

  // A refused action, such as placing a workpiece on a running belt.
  const actionMessage = ref('')
  let messageTimer = 0

  function showMessage(text: string) {
    window.clearTimeout(messageTimer)
    actionMessage.value = text
    messageTimer = window.setTimeout(() => (actionMessage.value = ''), 4000)
  }

  async function runAction(name: string, args?: { [key: string]: string | undefined } | null) {
    window.clearTimeout(messageTimer)
    actionMessage.value = ''
    try {
      await api.RunAction(name, args ?? {})
    } catch (error) {
      showMessage((error as Error).message)
    }
  }

  async function setParameter(id: string, value: number) {
    try {
      await api.SetParameter(id, value)
    } catch (error) {
      showMessage((error as Error).message)
    }
  }

  // Parameters come back in the snapshot, so a change needs no scene reload.
  const parameterValue = (id: string, fallback: number) => snapshot.value?.parameters?.[id] ?? fallback

  // The PLC that the PLC panel shows: the one selected, of a station with several, else the
  // first. Its id is "" for the first, as the service takes it.
  const plcIndex = computed(() => Math.max(0, scene.value?.plcs?.findIndex((p) => p.module === selection.value) ?? 0))
  const plcId = computed(() => (plcIndex.value > 0 ? (scene.value?.plcs?.[plcIndex.value]?.module ?? '') : ''))
  const plcScene = computed(() => scene.value?.plcs?.[plcIndex.value] ?? scene.value?.plc ?? null)
  const plcView = computed(() => snapshot.value?.plcs?.[plcIndex.value] ?? null)

  return {
    service: api,
    scene,
    snapshot,
    selection,
    selected,
    workpiece,
    pickedWorkpiece,
    pickWorkpiece,
    takeWorkpiece,
    highlight,
    stateOn,
    operate,
    actionMessage,
    runAction,
    setParameter,
    parameterValue,
    plcId,
    plcScene,
    plcView,
  }
}

export type Sim = ReturnType<typeof createSim>

// The station in the simulation.
export const simulation = createSim(Service)
export const loadError = ref('')

const simKey: InjectionKey<Sim> = Symbol('sim')
export { simKey }

// The simulation, unless a page provides another one for its components.
export const useSim = () => inject(simKey, simulation)

export const bit = (image: number[] | null | undefined, byte: number, index: number) => ((image?.[byte] ?? 0) & (1 << index)) !== 0

// Survives a reload of the window, but not closing the app. Storage can be blocked.
export function recall(key: string) {
  try {
    return sessionStorage.getItem(key)
  } catch {
    return null
  }
}

export function remember(key: string, value: string) {
  try {
    sessionStorage.setItem(key, value)
  } catch {
    // It only means starting from the first page after a reload.
  }
}

// Settles once the app knows whether a project is open, which the first page waits for.
let markReady = () => {}
export const ready = new Promise<void>((resolve) => (markReady = resolve))

// Counts how often the user's files were read again, so that lists of stations refresh.
export const filesRead = ref(0)

// Reads the user's files again: the lists of stations refresh, and the open project's
// station is loaded again as its file now has it. It fails when that station is gone or broken.
export async function reloadFiles() {
  filesRead.value++
  if (project.value) await reloadStation()
}

// Loads the open project's station again, such as after building on it. Its PLCs are
// disconnected, as on a restart.
export async function reloadStation() {
  simulation.selection.value = null
  simulation.snapshot.value = null
  simulation.scene.value = await Service.ReloadStation()
  await refreshProject()
}

// Reads the files again after a collection changed. When the open project's station went
// with it, the project is closed.
export async function reloadCollections() {
  try {
    await reloadFiles()
  } catch {
    await closeProject()
  }
}

// The project the simulation works in, or null while none is open and the app shows its
// projects. Everything in the simulation belongs to a project.
export const project = ref<ProjectInfo | null>(null)

export async function refreshProject() {
  const [p, st] = await Promise.all([Service.Project(), Service.SaveState()])
  project.value = p.name ? p : null
  saveState.value = p.name ? st : null
}

// How the open project's file stands: saved, about to be, read only, changed from elsewhere,
// or not written; null while no project is open. The project is saved into its file as it
// changes.
export const saveState = ref<SaveState | null>(null)

// Asks what to do about a project file changed, moved or deleted from elsewhere while it is
// open, once for each time it happens.
let resolving = false
async function resolveConflict() {
  const p = project.value
  if (resolving || !p || saveState.value?.state !== 'conflict') return
  resolving = true
  try {
    const KEEP = 'Keep mine'
    const THEIRS = 'Open theirs'
    const COPY = 'Save mine as a copy…'
    const gone = saveState.value.problem?.includes('moved or deleted')
    const answer = await choose(
      gone
        ? `${p.name} was moved or deleted while it was open. Write your project back where it was, or save it as a copy somewhere else?`
        : `${p.name} was changed from elsewhere while it was open. Keep your project and write it over the file, open the file as it is now and drop your changes, or save yours as a copy?`,
      gone ? [KEEP, COPY] : [KEEP, THEIRS, COPY],
    )
    if (answer === KEEP) saveState.value = await Service.KeepMine()
    else if (answer === THEIRS) await show(Service.OpenTheirs())
    else if (answer === COPY) await saveProjectAs()
  } catch (error) {
    report(error)
  } finally {
    resolving = false
  }
}

// Saves the open project as a copy the user picks, and goes on working in the copy.
export async function saveProjectAs() {
  if (await Service.SaveProjectAs()) await refreshProject()
}

// Shows the station the simulation has after opening something, and its project.
async function show(next: Promise<Scene>, to: 'simulate' | 'build' = 'simulate') {
  programming.value = null
  editingConnection.value = null
  simulation.selection.value = null
  simulation.snapshot.value = null
  simulation.scene.value = await next
  await refreshProject()
  await router.push({ name: to })
}

// Makes a project of a station or an exercise, named after it unless a name is given.
export const newProject = (base: Base, name = '') => show(Service.NewProject(name, base))

// Makes a project with an empty station of its own, standing on what a collection has to
// stand on, and opens it to be built on.
export async function newEmptyProject(collection: string, name = '') {
  await show(Service.NewEmptyProject(name, collection), 'build')
}

// What takes a picture of the simulation's 3D view, while it shows.
export const thumbnails: { take: (() => string | null) | null } = { take: null }

// Closes the project: it keeps a picture of its station to be shown by, its file is written,
// its PLCs are disconnected, and the app shows its projects. Changes that cannot be written
// into the file are saved as a copy or dropped, after asking; cancelling keeps it open.
export async function closeProject() {
  const name = project.value?.name ?? ''
  // A file that cannot be written gets no new picture, which would be a change to ask about.
  const picture = saveState.value?.state === 'readOnly' ? null : thumbnails.take?.()
  if (picture) await Service.SaveThumbnail(picture).catch(() => undefined)
  const st = await Service.SaveProject()
  if (st.unsaved) {
    const COPY = 'Save a copy…'
    const why = st.state === 'readOnly' ? 'its file cannot be written' : st.problem || 'they could not be written into its file'
    const answer = await choose(`${name} has changes that are not saved: ${why.replace(/\.$/, '')}. Save them as a copy, or close without them?`, [COPY, 'Close without saving'])
    if (!answer) return
    if (answer === COPY && !(await Service.SaveProjectAs())) return
    if (answer !== COPY) await Service.CloseProjectDiscarding()
  }
  programming.value = null
  editingConnection.value = null
  await Service.CloseProject()
  simulation.selection.value = null
  simulation.snapshot.value = null
  simulation.scene.value = null
  project.value = null
  saveState.value = null
  await router.push({ name: 'projects' })
}

// Opens a project file the user picks; cancelling changes nothing.
export async function importProject() {
  const path = await Service.ChooseProjectFile()
  if (path) await openProjectFile(path)
}

// Opens a project file where it is, after asking whether to go on with changes a crash left
// that its file lacks. Its station loads from the collections it carries.
export async function openProjectFile(path: string) {
  const info = await Service.InspectProjectFile(path)
  const list = (rs: { name?: string; collection: string; version?: string }[]) => rs.map((r) => `${r.name || r.collection}${r.version ? ` ${r.version}` : ''}`).join(', ')
  const missing = info.missing ?? []
  if (missing.length) {
    const hidden = missing.every((r) => r.hidden)
    throw new Error(
      hidden
        ? `${info.name} needs the collection ${list(missing)}, which is hidden: show it in Settings › Collections.`
        : `${info.name} needs the collection ${list(missing)}: import it in Settings › Collections first.`,
    )
  }
  const options = { restore: false }
  if (info.hasUnsaved) {
    const RESTORE = 'Restore them'
    const when = info.unsaved ? ` from ${new Date(info.unsaved).toLocaleString()}` : ''
    const answer = await choose(`${info.name} has changes${when} that were not saved into its file, such as when the app was stopped. Restore them?`, [RESTORE, 'Drop them'])
    if (!answer) return
    options.restore = answer === RESTORE
  }
  await show(Service.OpenProjectFile(path, options).then(() => Service.Scene()))
}

// Shows what went wrong on the projects page, or in a dialog while a project is open.
function report(error: unknown) {
  const text = error instanceof Error ? error.message : String(error)
  projectError.value = text
  if (project.value) void tell(text)
}

// Opens a file the system asked the app to open, such as a double-clicked project file.
async function takeOpened() {
  const path = await Service.TakeOpenedFile()
  if (path) await openProjectFile(path).catch(report)
}

// The PLC whose programming tool is open over the 3D view, "" for the station's first, or
// null while none is.
export const programming = ref<string | null>(null)

// The PLC whose connection's bindings are open in the wiring, with the values of its form, or
// null while none is, and the page they were opened from, which closing them goes back to. Saved
// names the PLC once its bindings were saved into the project.
export const editingConnection = ref<{ plc: string; values: Record<string, unknown>; back?: string } | null>(null)
export const savedConnection = ref<{ plc: string } | null>(null)

// Counts the times the PLCs' connections changed outside their panel, such as when one was put
// back as the project's original has it, for the panel to show them as they are now.
export const connectionsChanged = ref(0)

export function connect() {
  Events.On('sim:snapshot', (event) => {
    simulation.snapshot.value = event.data as Snapshot
  })
  // The commands of the app's menu, which only a Mac has.
  Events.On('menu:command', (event) => {
    if (event.data === 'settings') {
      openSettings()
      return
    }
    const run = event.data === 'import' ? importProject : event.data === 'close' && project.value ? closeProject : null
    run?.().catch(report)
  })
  // Project files dropped onto the projects page open, the first of them.
  Events.On('files:dropped', (event) => {
    const [path] = (event.data as string[] | null) ?? []
    if (!path || project.value) return
    projectError.value = ''
    openProjectFile(path).catch(report)
  })
  // A project file double-clicked opens, also when it started the app.
  Events.On('files:opened', () => void takeOpened())
  // How the open project's file stands, as it changes.
  Events.On('project:save', (event) => {
    if (!project.value) return
    saveState.value = event.data as SaveState
    void resolveConflict()
  })
  // A project stays open when the window is reloaded.
  refreshProject()
    .then(async () => {
      if (project.value) simulation.scene.value = await Service.Scene()
    })
    .catch((error: Error) => {
      loadError.value = error.message
    })
    .finally(() => {
      markReady()
      void takeOpened()
    })
}

// What went wrong with a command of the app's menu, for the projects page to show.
export const projectError = ref('')
