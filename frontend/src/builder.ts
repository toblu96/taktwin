import { Events } from '@wailsio/runtime'
import { computed, ref, shallowRef, watch } from 'vue'
import {
  BuilderService,
  type BuilderCheck,
  type BuilderGhost,
  type BuilderModule,
  type BuilderPlace,
  type BuilderState,
  type WiringState,
} from '../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { Entry, SitePlan } from '../bindings/github.com/toblu96/taktwin/internal/model'
import type { Snapshot } from '../bindings/github.com/toblu96/taktwin/internal/engine'
import { createSim } from './sim'
import { GRID, type GhostPose, type Handle, type Overlay, type SiteLine } from './viewer/build'

// The station being built, with the same shape as the simulation so the same components
// work on it. It runs without a PLC.
export const built = createSim({
  RunAction: (name, args) => BuilderService.RunAction(name, args),
  SetControl: (name, on) => BuilderService.SetControl(name, on),
  SetFault: (name, on) => BuilderService.SetFault(name, on),
  SetPin: (state, on) => BuilderService.SetPin(state, on),
  SetParameter: (id, value) => BuilderService.SetParameter(id, value),
})

export const state = shallowRef<BuilderState | null>(null)
// Why the last change was refused, such as a screw beside its slot.
export const problem = ref('')
export const busy = ref(false)

const message = (error: unknown) => (error instanceof Error ? error.message : String(error))

async function apply(change: () => PromiseLike<BuilderState>) {
  busy.value = true
  problem.value = ''
  try {
    const next = await change()
    state.value = next
    built.scene.value = next.scene
    built.highlight.value = null
    return next
  } catch (error) {
    problem.value = message(error)
    return null
  } finally {
    busy.value = false
  }
}

const modules = computed(() => state.value?.modules ?? [])

// The module that is selected: a part that is a module of its own, such as a conveyor's
// sensor, or else the nearest module around the clicked part.
export const selected = computed<BuilderModule | null>(() => {
  const id = built.selection.value
  if (!id) return null
  const path = id.split('/')
  for (let n = path.length; n > 0; n--) {
    const found = modules.value.find((m) => m.id === path.slice(0, n).join('/'))
    if (found) return found
  }
  return null
})

const siteKey = (s: { owner: string; id: string; slot: number }) => `${s.owner}.${s.id}#${s.slot}`
const toLine = (s: SitePlan, current = false): SiteLine => ({
  key: siteKey(s),
  type: s.type,
  from: s.from,
  dir: s.dir,
  min: s.min,
  max: s.max,
  turn: s.turn,
  gaps: s.gaps ?? [],
  current,
})

// The sites a module of these mount kinds could go on, leaving out its own and those of
// what it carries.
function fitting(mounts: string[], self: string | null) {
  return (state.value?.sites ?? []).filter((s) => mounts.includes(s.type) && (!self || (s.owner !== self && !s.owner.startsWith(`${self}/`))))
}

// --- Adding a module: it follows the pointer as a ghost until it is put down. ---

// A module picked from the list, waiting for where it goes.
export const pending = ref<Entry | null>(null)
export const ghost = shallowRef<BuilderGhost | null>(null)
// How far the module being added is turned, in steps of 90 degrees with R.
export const turn = ref(0)
// The place under the pointer, and whether the module fits there.
export const hovered = ref<{ key: string; along: number } | null>(null)
export const check = ref<BuilderCheck | null>(null)
// On a touchscreen, a tap only shows the ghost there; this asks to confirm.
export const confirming = ref(false)

export const placing = computed<SiteLine[] | null>(() => {
  if (!pending.value) return null
  const types = (ghost.value?.mounts ?? []).map((m) => m.type)
  return fitting(types.length ? types : (pending.value.mounts ?? []), null).map((s) => toLine(s))
})

const lineOf = (key: string) => {
  const site = (state.value?.sites ?? []).find((s) => siteKey(s) === key)
  return site ? toLine(site) : null
}

// Where the ghost stands on a place, as the backend would screw the module on there.
function poseOn(key: string, along: number, extra: number): { at: { x: number; y: number; z: number }; rotZ: number } | null {
  const line = lineOf(key)
  const mount = ghost.value?.mounts?.find((m) => m.type === line?.type) ?? ghost.value?.mounts?.[0]
  if (!line || !mount) return null
  const rotZ = (Math.atan2(line.dir.y, line.dir.x) * 180) / Math.PI + line.turn + mount.turn + extra
  const r = (rotZ * Math.PI) / 180
  const m = {
    x: mount.at.x * Math.cos(r) - mount.at.y * Math.sin(r),
    y: mount.at.x * Math.sin(r) + mount.at.y * Math.cos(r),
    z: mount.at.z,
  }
  const p = {
    x: line.from.x + line.dir.x * along,
    y: line.from.y + line.dir.y * along,
    z: line.from.z + line.dir.z * along,
  }
  return { at: { x: p.x - m.x, y: p.y - m.y, z: p.z - m.z }, rotZ }
}

const placeOf = (key: string, along: number, clearRail = false): BuilderPlace => {
  const [on, slot] = key.split('#')
  return { on, slot: Number(slot), at: along, turn: turn.value, clearRail }
}

export const ghostPose = computed<GhostPose | null>(() => {
  if (!pending.value || !hovered.value) return null
  const pose = poseOn(hovered.value.key, hovered.value.along, turn.value)
  return pose ? { ...pose, ok: check.value ? check.value.ok : null } : null
})

// Checks one place at a time, always the latest the pointer reached.
let checking = false
async function recheck() {
  if (checking || !pending.value || !hovered.value) return
  checking = true
  const asked = { ...hovered.value, turn: turn.value }
  try {
    const result = await BuilderService.Check(pending.value.id, '', placeOf(asked.key, asked.along))
    const now = hovered.value
    if (now && now.key === asked.key && now.along === asked.along && turn.value === asked.turn) check.value = result
  } finally {
    checking = false
  }
  const now = hovered.value
  if (now && (now.key !== asked.key || now.along !== asked.along || turn.value !== asked.turn)) void recheck()
}

export async function pick(entry: Entry) {
  if (pending.value?.id === entry.id) {
    cancelPlacing()
    return
  }
  built.selection.value = null
  pending.value = entry
  hovered.value = null
  check.value = null
  confirming.value = false
  turn.value = 0
  ghost.value = null
  try {
    const shape = await BuilderService.Ghost(entry.id)
    if (pending.value?.id === entry.id) ghost.value = shape
  } catch (error) {
    problem.value = message(error)
  }
}

export function cancelPlacing() {
  pending.value = null
  ghost.value = null
  hovered.value = null
  check.value = null
  confirming.value = false
}

export function hover(key: string, along: number) {
  if (confirming.value) return
  if (hovered.value?.key === key && hovered.value.along === along) return
  hovered.value = { key, along }
  check.value = null
  void recheck()
}

// A click puts it down; a tap on a touchscreen first shows it there.
export function placeAt(key: string, along: number, touch: boolean) {
  hovered.value = { key, along }
  check.value = null
  void recheck()
  if (touch) {
    confirming.value = true
    return
  }
  void putDown(false)
}

export function rotate() {
  turn.value = (turn.value + 90) % 360
  check.value = null
  void recheck()
}

// Puts the module being added where the ghost stands. With clearRail, what stands where a
// rail has to be cut for a slide, such as a conveyor's sensor, is removed first.
export async function putDown(clearRail: boolean) {
  const entry = pending.value
  const at = hovered.value
  if (!entry || !at) return
  const before = new Set(modules.value.map((m) => m.id))
  const next = await apply(() => BuilderService.AddAt(entry.id, placeOf(at.key, at.along, clearRail)))
  if (next) {
    cancelPlacing()
    const added = next.modules?.find((m) => !before.has(m.id))
    if (added) built.selection.value = added.id
  } else {
    confirming.value = false
  }
}

// --- Moving the selected module: by its handles, or the arrow keys. ---

// What is screwed onto a module, and onto that, so that it goes along when it moves.
function carried(id: string) {
  const out: string[] = []
  for (let added = true; added;) {
    added = false
    for (const m of modules.value) {
      const owner = m.place?.on.split('.')[0].split('/')[0]
      if (!m.parent && owner && (owner === id || out.includes(owner)) && !out.includes(m.id)) {
        out.push(m.id)
        added = true
      }
    }
  }
  return out
}

export const handle = computed<Handle | null>(() => {
  const module = selected.value
  if (pending.value || !module?.place || !module.line) return null
  // Its line, and the parallel repeats of its site, such as the plate's slots.
  const lines = fitting(module.mounts ?? [], module.id)
    .filter((s) => `${s.owner}.${s.id}` === module.place?.on)
    .map((s) => toLine(s, s.slot === module.place?.slot))
  const line = lines.find((l) => l.current)
  if (!line) return null
  return {
    id: module.id,
    at: module.place.at,
    line,
    lines,
    carry: carried(module.id),
    sides: module.place.on === FLOOR ? freeSides(module) : undefined,
  }
})

const FLOOR = 'floor.area'
const width = (m: BuilderModule) => m.floor?.width ?? 0

// Which sides of a stand have room for another one, as wide as it is.
function freeSides(stand: BuilderModule) {
  const at = stand.place!.at
  const w = width(stand)
  const taken = (from: number, to: number) =>
    modules.value.some((m) => m.id !== stand.id && m.place?.on === FLOOR && m.place.slot === stand.place!.slot && m.place.at < to - 0.5 && m.place.at + width(m) > from + 0.5)
  return {
    left: !taken(at - w, at),
    right: !taken(at + w, at + 2 * w),
  }
}

// Moves a module onto its new place. A part of another module stays on that module.
export function move(module: BuilderModule, place: BuilderPlace) {
  return apply(() => BuilderService.Move(module.id, place))
}

export function handleMoved(key: string, along: number) {
  const module = selected.value
  if (!module?.place) return
  if (key === handle.value?.line.key) return move(module, { ...module.place, at: along })
  const [on, slot] = key.split('#')
  return move(module, { ...module.place, on, slot: Number(slot), at: along })
}

// One step along its line, a grid step or a mm, or one slot across.
export function nudge(axis: 'along' | 'across', sign: number, fine = false) {
  const module = selected.value
  const h = handle.value
  if (!module?.place || !h) return
  if (axis === 'along') {
    // To the next grid point that way, also from a place between two.
    const step = fine ? 1 : GRID
    const next = sign > 0 ? Math.floor(module.place.at / step + 1e-9) * step + step : Math.ceil(module.place.at / step - 1e-9) * step - step
    const along = Math.min(h.line.max, Math.max(h.line.min, next))
    return move(module, { ...module.place, at: along })
  }
  const next = h.lines[h.lines.findIndex((l) => l.current) + sign]
  if (next) return handleMoved(next.key, module.place.at)
}

// --- What modules line up with: where workpieces travel and the special positions. ---

export const overlay = computed<Overlay | null>(() => {
  if (!state.value) return null
  const paths = (state.value.paths ?? []).map((p) => ({
    owner: p.owner,
    points: p.points ?? [],
  }))
  return {
    paths,
    marks: state.value.marks ?? [],
    focus: selected.value?.id.split('/')[0] ?? null,
  }
})

export const selectedMarks = computed(() => {
  const id = selected.value?.id.split('/')[0]
  return (state.value?.marks ?? []).filter((m) => m.owner === id)
})

// --- The rest of building. ---

export const fit = (id: string) => apply(() => BuilderService.Fit(id))
export const setParam = (id: string, param: string, value: string) => apply(() => BuilderService.SetParam(id, param, value))
export const setStation = (name: string, description: string) => apply(() => BuilderService.SetStation(name, description))

export async function start(from: string) {
  built.selection.value = null
  built.snapshot.value = null
  cancelPlacing()
  await apply(() => BuilderService.Start(from))
}

export async function remove(module: BuilderModule) {
  if (module.parent && !module.optional) {
    problem.value = `${modules.value.find((m) => m.id === module.parent)?.name ?? module.parent} needs its ${module.name}. Move it instead.`
    return
  }
  if (!module.parent && (!module.place || (module.place.on === 'floor.area' && !module.optional))) {
    problem.value = module.place ? `${module.name} carries the PLC, and stays.` : `${module.name} is what the station stands on, and stays.`
    return
  }
  built.selection.value = null
  await apply(() => BuilderService.Remove(module.id))
}

// Puts an empty stand beside the selected one, on its left (-1) or right (1), and selects it.
export async function addStand(side: number) {
  const beside = selected.value
  if (!beside) return
  const before = new Set(modules.value.map((m) => m.id))
  const next = await apply(() => BuilderService.AddStand(beside.id, side))
  const added = next?.modules?.find((m) => !before.has(m.id))
  if (added) built.selection.value = added.id
}

// Gives a module back a part it comes with, such as a conveyor's sensor, where it was.
export async function putBack(id: string) {
  const next = await apply(() => BuilderService.PutBack(id))
  if (next) built.selection.value = id
}

// Saves into the user folder. Without overwrite, an existing station of the same name is
// reported in exists instead.
export const exists = ref('')
export async function save(overwrite: boolean) {
  problem.value = ''
  exists.value = ''
  try {
    const path = await BuilderService.Save(overwrite)
    state.value = state.value ? { ...state.value, saved: path } : state.value
    await refreshOwn()
  } catch (error) {
    const text = message(error)
    if (text.includes('file already exists')) exists.value = text.split(':')[0]
    else problem.value = text
  }
}

// The stations saved in the user folder, which can be deleted.
export const own = ref<string[]>([])
export async function refreshOwn() {
  own.value = (await BuilderService.UserStations()) ?? []
}
export async function deleteStation(id: string) {
  problem.value = ''
  try {
    await BuilderService.DeleteStation(id)
  } catch (error) {
    problem.value = message(error)
  }
  await refreshOwn()
}

// Selecting something else ends adding a module.
watch(
  () => built.selection.value,
  (id) => {
    if (id && pending.value) cancelPlacing()
  },
)

let unsubscribe: (() => void) | undefined

export async function enter() {
  unsubscribe = Events.On('builder:snapshot', (event) => {
    built.snapshot.value = event.data as Snapshot
  })
  await refreshOwn()
  if (state.value) return
  const next = await BuilderService.State().catch(() => null)
  if (next) {
    state.value = next
    built.scene.value = next.scene
  } else await start('')
}

export function leave() {
  unsubscribe?.()
}

// --- Wiring: which cable goes where, which pin carries what, and what the PLC calls it. ---

// The builder shows the station in 3D to build it, or its wiring.
export const mode = ref<'build' | 'wire'>('build')
export const wiring = shallowRef<WiringState | null>(null)

export async function loadWiring() {
  try {
    wiring.value = await BuilderService.Wiring()
  } catch (error) {
    problem.value = message(error)
  }
}

async function rewire(change: () => PromiseLike<BuilderState>) {
  const next = await apply(change)
  if (next) await loadWiring()
  return next
}

// Joins two connectors; a cable on one moves to the other, cables on both swap.
export const plug = (a: string, b: string) => rewire(() => BuilderService.Plug(a, b))
export const unplug = (ref: string) => rewire(() => BuilderService.Unplug(ref))
// Moves what a pin of a terminal carries onto another of its pins, swapping them.
export const movePin = (terminal: string, from: string, to: string) => rewire(() => BuilderService.MovePin(terminal, from, to))
export const setSignal = (plc: string, address: string, name: string) => rewire(() => BuilderService.SetSignal(plc, address, name))
