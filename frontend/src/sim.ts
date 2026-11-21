import { Events } from '@wailsio/runtime'
import { computed, inject, ref, shallowRef, watch, type InjectionKey } from 'vue'
import { Service } from '../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { Base, ProjectInfo } from '../bindings/github.com/toblu96/taktwin/internal/simapp/models'
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

// The page that is showing.
type Page = 'simulation' | 'library' | 'builder' | 'settings'
const savedPage = recall('taktwin.page')
export const page = ref<Page>(savedPage === 'library' || savedPage === 'builder' || savedPage === 'settings' ? savedPage : 'simulation')
watch(page, (value) => remember('taktwin.page', value))

// Counts how often the user's files were read again, so that lists of stations refresh.
export const filesRead = ref(0)

// Reads the user's files again: the lists of stations refresh, and the open project is opened
// again with its station as its file now has it. It fails when that station is gone or broken.
export async function reloadFiles() {
  filesRead.value++
  if (project.value) await openProject(project.value.name)
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
  const p = await Service.Project()
  project.value = p.name ? p : null
}

// Shows the station the simulation has after opening something, and its project.
async function show(next: Promise<Scene>) {
  programming.value = null
  editingConnection.value = null
  simulation.selection.value = null
  simulation.snapshot.value = null
  simulation.scene.value = await next
  await refreshProject()
  page.value = 'simulation'
}

// Opens a project by name, or makes one of a station or an exercise, named after it unless
// a name is given.
export const openProject = (name: string) => show(Service.OpenProject(name))
export const newProject = (base: Base, name = '') => show(Service.NewProject(name, base))

// What takes a picture of the simulation's 3D view, while it shows.
export const thumbnails: { take: (() => string | null) | null } = { take: null }

// Closes the project: it keeps a picture of its station to be shown by, its PLCs are
// disconnected, and the app shows its projects.
export async function closeProject() {
  programming.value = null
  editingConnection.value = null
  const picture = thumbnails.take?.()
  if (picture) await Service.SaveThumbnail(picture).catch(() => undefined)
  await Service.CloseProject()
  simulation.selection.value = null
  simulation.snapshot.value = null
  simulation.scene.value = null
  project.value = null
  page.value = 'simulation'
}

// Imports a project file the user picks, and opens it; cancelling changes nothing.
export async function importProject() {
  const info = await Service.ImportProject()
  if (info.name) await show(Service.Scene())
}

// The PLC whose programming tool is open over the 3D view, "" for the station's first, or
// null while none is.
export const programming = ref<string | null>(null)

// The connection whose editor is open over the 3D view, with the PLC it is for and the values
// of its form, or null while none is. Saved is the id it was saved under, once it is.
export const editingConnection = ref<{ plc: string; id: string; values: Record<string, unknown> } | null>(null)
export const savedConnection = ref<{ plc: string; id: string; values?: Record<string, unknown> } | null>(null)

export function connect() {
  Events.On('sim:snapshot', (event) => {
    simulation.snapshot.value = event.data as Snapshot
  })
  // The commands of the app's menu, which only a Mac has.
  Events.On('menu:command', (event) => {
    if (event.data === 'settings') {
      page.value = 'settings'
      return
    }
    const run = event.data === 'import' ? importProject : event.data === 'close' && project.value ? closeProject : null
    run?.().catch((error: Error) => (projectError.value = error.message ?? String(error)))
  })
  // Project files dropped onto the projects page open, the first of them.
  Events.On('files:dropped', (event) => {
    const [path] = (event.data as string[] | null) ?? []
    if (!path || project.value) return
    projectError.value = ''
    show(Service.OpenProjectFile(path).then(() => Service.Scene())).catch((error: Error) => (projectError.value = error.message ?? String(error)))
  })
  // A project stays open when the window is reloaded.
  refreshProject()
    .then(async () => {
      if (project.value) simulation.scene.value = await Service.Scene()
    })
    .catch((error: Error) => {
      loadError.value = error.message
    })
}

// What went wrong with a command of the app's menu, for the projects page to show.
export const projectError = ref('')
