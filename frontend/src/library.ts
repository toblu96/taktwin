import { Events } from '@wailsio/runtime'
import { computed, ref, shallowRef } from 'vue'
import { LibraryService } from '../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { Snapshot } from '../bindings/github.com/toblu96/taktwin/internal/engine'
import type { Entry, EntryRef } from '../bindings/github.com/toblu96/taktwin/internal/model'
import { createSim, recall, remember } from './sim'

// The preview shown in the library, with the same shape as the simulation so the
// same components work on it. It runs without a PLC.
export const preview = createSim({
  RunAction: (name, args) => LibraryService.RunAction(name, args),
  SetControl: (name, on) => LibraryService.SetControl(name, on),
  SetFault: (name, on) => LibraryService.SetFault(name, on),
  SetPin: (state, on) => LibraryService.SetPin(state, on),
  // This can change the shape of a module, so it brings back a new scene.
  SetParameter: async (id, value) => {
    preview.scene.value = await LibraryService.SetParameter(id, value)
  },
})

export const entries = shallowRef<Entry[]>([])
export const current = ref<EntryRef | null>(null)
export const openError = ref('')

// The instance a single module is previewed as. A station has none.
export const rootInstance = computed(() => {
  if (!current.value || current.value.level === 'station') return null
  return preview.scene.value?.instances?.find((i) => i.id === current.value?.id) ?? null
})

// Opens an entry in the preview. Only the last one asked for shows, should an earlier one
// answer after it.
let opening = 0
export async function open(entry: EntryRef) {
  const ticket = ++opening
  current.value = entry
  remember('taktwin.entry', `${entry.level}/${entry.id}`)
  openError.value = ''
  preview.selection.value = null
  preview.snapshot.value = null
  try {
    const scene = await LibraryService.Open(entry.level, entry.id)
    if (ticket === opening) preview.scene.value = scene
  } catch (error) {
    if (ticket !== opening) return
    preview.scene.value = null
    openError.value = (error as Error).message
  }
}

let unsubscribe: (() => void) | undefined

// Entering the library lists its entries; the page opens the one its address names.
export async function enter() {
  unsubscribe = Events.On('library:snapshot', (event) => {
    preview.snapshot.value = event.data as Snapshot
  })
  entries.value = (await LibraryService.Entries()) ?? []
}

// The entry to show when the address names none: the one shown last, or the first that loads.
export function lastEntry(): EntryRef | null {
  const saved = recall('taktwin.entry')
  return entries.value.find((e) => `${e.level}/${e.id}` === saved) ?? entries.value.find((e) => !e.error) ?? null
}

export function leave() {
  unsubscribe?.()
  void LibraryService.Close()
}

export const reset = () => LibraryService.Reset()
