import { Events } from '@wailsio/runtime'
import { computed, inject, ref, shallowRef, watch, type InjectionKey } from 'vue'
import { Service } from '../bindings/github.com/toblu96/taktwin/internal/simapp'
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

// Reads the user's files again: the station list refreshes, and the station in the
// simulation is loaded again from its file. It fails when that station is gone or broken.
export async function reloadFiles() {
  filesRead.value++
  const id = simulation.scene.value?.id
  if (id) await loadStation(id)
}

// Reads the files again after a collection changed. When the station in the simulation went
// with it, the library's default one takes its place.
export async function reloadCollections() {
  try {
    await reloadFiles()
  } catch {
    const id = await Service.DefaultStation()
    if (id) await loadStation(id)
  }
}

// Puts another station from the catalog into the simulation.
export async function loadStation(id: string) {
  simulation.selection.value = null
  simulation.snapshot.value = null
  simulation.scene.value = await Service.LoadStation(id)
}

// The PLC whose programming tool is open over the 3D view, "" for the station's first, or
// null while none is.
export const programming = ref<string | null>(null)

// Puts the station of an exercise into the simulation, set up as the exercise changes it.
export async function openExercise(id: string) {
  simulation.selection.value = null
  simulation.snapshot.value = null
  simulation.scene.value = await Service.OpenExercise(id)
}

export function connect() {
  Events.On('sim:snapshot', (event) => {
    simulation.snapshot.value = event.data as Snapshot
  })
  Service.Scene()
    .then((value) => {
      simulation.scene.value = value
    })
    .catch((error: Error) => {
      loadError.value = error.message
    })
}
