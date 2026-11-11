<script setup lang="ts">
import BuilderSetting from './BuilderSetting.vue'
import { computed, onBeforeUnmount, onMounted, provide, ref, watch } from 'vue'
import { LibraryService } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { Entry, SceneCheck } from '../../bindings/github.com/toblu96/taktwin/internal/model'
import type { BuilderModule } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import {
  addStand,
  built,
  busy,
  cancelPlacing,
  check,
  confirming as confirmPlace,
  deleteStation,
  enter,
  exists,
  fit,
  ghost,
  ghostPose,
  handle,
  handleMoved,
  hover,
  hovered,
  leave,
  mode,
  move,
  nudge as step,
  overlay,
  own,
  pending,
  pick,
  placeAt,
  placing,
  problem,
  putDown,
  remove,
  putBack,
  rotate,
  save,
  selected,
  selectedMarks,
  setParam,
  setStation,
  start,
  state,
} from '../builder'
import { grouped } from '../collections'
import { filesRead, loadStation, page, simKey } from '../sim'
import StationViewer from '../viewer/StationViewer.vue'
import WiringView from './WiringView.vue'

// The components below show the station being built, not the running simulation.
provide(simKey, built)

const entries = ref<Entry[]>([])
const from = ref('')
const loadEntries = async () => {
  entries.value = (await LibraryService.Entries()) ?? []
}
onMounted(async () => {
  window.addEventListener('keydown', onKey)
  await enter()
  await loadEntries()
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  leave()
})

// Escape puts a picked module back or deselects, R turns a picked one, Backspace or Delete
// removes the selected one, and the arrow keys move it: along its line a grid step, or a mm
// with Shift, and across to the next slot.
function onKey(event: KeyboardEvent) {
  if (mode.value !== 'build') return
  const target = event.target as HTMLElement | null
  if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT')) return
  if (event.key === 'Escape') {
    if (pending.value) cancelPlacing()
    else built.selection.value = null
    return
  }
  if (pending.value && (event.key === 'r' || event.key === 'R')) {
    rotate()
    return
  }
  if (!selected.value || busy.value) return
  const arrows: Record<string, ['along' | 'across', number]> = {
    ArrowRight: ['along', 1],
    ArrowLeft: ['along', -1],
    ArrowUp: ['across', 1],
    ArrowDown: ['across', -1],
  }
  if (event.key in arrows) {
    event.preventDefault()
    const [axis, sign] = arrows[event.key]
    void step(axis, sign, event.shiftKey)
  } else if (event.key === 'Backspace' || event.key === 'Delete') {
    event.preventDefault()
    void remove(selected.value)
  }
}

const stations = computed(() => entries.value.filter((e) => e.level === 'station' && !e.error))
// To start from, by collection: an empty station of each that has something to stand on, then its stations.
const startGroups = computed(() =>
  grouped([...stations.value, ...entries.value.filter((e) => e.role === 'station' && !e.error)]).map((g) => ({
    ...g,
    empty: g.items.some((e) => e.level !== 'station'),
    stations: g.items.filter((e) => e.level === 'station'),
  })),
)

// The palette shows one collection at a time, by default the one most of the station is from.
const ALL = '*'
const chosenCollection = ref<string | null>(null)
const placeable = computed(() => entries.value.filter((e) => e.mounts?.length && !e.error && e.level !== 'station'))
const paletteCollections = computed(() => grouped(placeable.value).map((g) => ({ id: g.id, name: g.name })))
const builtFrom = computed(() => {
  const counts = new Map<string, number>()
  for (const m of state.value?.modules ?? []) {
    const c = entries.value.find((e) => e.id === m.module && e.level !== 'station')?.collection
    if (c) counts.set(c, (counts.get(c) ?? 0) + 1)
  }
  return [...counts].sort((a, b) => b[1] - a[1])[0]?.[0] ?? paletteCollections.value.find((c) => c.id)?.id ?? ALL
})
const paletteCollection = computed({
  get: () => chosenCollection.value ?? builtFrom.value,
  set: (v: string) => (chosenCollection.value = v),
})
// What can be screwed on somewhere, modules first and accessories by their group.
const palette = computed(() => {
  const placeable = paletteVisible.value
  const groups = new Map<string, Entry[]>()
  for (const e of placeable) {
    const title = e.level === 'module' ? 'Modules' : e.group || 'Accessories'
    groups.set(title, [...(groups.get(title) ?? []), e])
  }
  return [...groups].map(([title, list]) => ({
    title,
    list: list.sort((a, b) => a.name.localeCompare(b.name)),
  }))
})

// A station saved here shows in the list to start from.
watch(
  () => state.value?.saved,
  (saved) => {
    if (saved) {
      filesRead.value++
      void loadEntries()
    }
  },
)

const confirming = ref('')
async function confirmDelete() {
  const id = confirming.value
  confirming.value = ''
  await deleteStation(id)
  if (from.value === id) from.value = ''
  filesRead.value++
  await loadEntries()
}

const name = ref('')
const description = ref('')
watch(
  () => state.value?.station,
  (station) => {
    name.value = station?.name ?? ''
    description.value = station?.description ?? ''
  },
  { immediate: true },
)
function rename() {
  const station = state.value?.station
  if (station && (name.value !== station.name || description.value !== station.description)) void setStation(name.value, description.value)
}

// Where the module is while its handle is dragged, before it is let go.
const dragAt = ref<number | null>(null)
function handled(key: string, along: number, done: boolean) {
  if (!done) {
    dragAt.value = along
    return
  }
  dragAt.value = null
  void handleMoved(key, along)
}
const shownAt = computed(() => dragAt.value ?? selected.value?.place?.at ?? 0)

function setAt(event: Event) {
  const module = selected.value
  const value = Number((event.target as HTMLInputElement).value)
  if (module?.place && Number.isFinite(value)) void move(module, { ...module.place, at: value })
}
function setSlot(event: Event) {
  const module = selected.value
  const value = Number((event.target as HTMLInputElement).value)
  if (module?.place && Number.isInteger(value)) void move(module, { ...module.place, slot: value })
}
function turn(degrees: number) {
  const module = selected.value
  if (module?.place) void move(module, { ...module.place, turn: degrees })
}
const slots = computed(() => {
  const place = selected.value?.place
  if (!place) return 0
  return (state.value?.sites ?? []).filter((s) => `${s.owner}.${s.id}` === place.on).length
})

const parentName = computed(() => {
  const parent = selected.value?.parent
  return (state.value?.modules ?? []).find((m) => m.id === parent)?.name ?? parent ?? ''
})

// How a module is put together comes first; how it behaves folds away. Settings without a
// label are the module's own business, and not offered.
const buildSettings = computed(() => (selected.value?.params ?? []).filter((p) => p.label && p.kind === 'build'))
const physicsSettings = computed(() => (selected.value?.params ?? []).filter((p) => p.label && p.kind !== 'build'))
// An empty value puts a setting back to how the module comes.
function change(param: string, value: string) {
  const module = selected.value
  if (module) void setParam(module.id, param, value)
}

// The station as a tree: each stand holds what stands on it, such as its PLC and C
// interface, each module what is screwed onto it, and its own parts.
const tree = computed(() => {
  const modules = state.value?.modules ?? []
  const ids = new Set(modules.map((m) => m.id))
  const parentOf = (m: BuilderModule) => {
    if (m.parent) return m.parent
    const on = m.place?.on
    if (!on || on === 'floor.area') return ''
    const owner = on.split('.')[0].split('/')[0]
    return ids.has(owner) ? owner : ''
  }
  const out: { module: BuilderModule; depth: number }[] = []
  const visit = (parent: string, depth: number) => {
    for (const m of modules) {
      if (parentOf(m) === parent && !out.some((o) => o.module.id === m.id)) {
        out.push({ module: m, depth })
        visit(m.id, depth + 1)
      }
    }
  }
  visit('', 0)
  return out
})

// The right side shows the settings, or the station checks on their own tab.
const panelTab = ref<'settings' | 'checks'>('settings')
const checkCount = computed(() => state.value?.scene.checks?.length ?? 0)
// The left side adds modules, or lists those in the station.
const paletteTab = ref<'add' | 'modules'>('add')
const paletteVisible = computed(() => placeable.value.filter((e) => paletteCollection.value === ALL || (e.collection ?? '') === paletteCollection.value))
const onFloor = computed(() => selected.value?.place?.on === 'floor.area')
const removable = computed(() => {
  const m = selected.value
  return !!m && (m.parent ? m.optional : onFloor.value ? m.optional : !!m.place)
})

// Shows what a check is about in the 3D view, or nothing on a second click.
function show(check: SceneCheck) {
  const current = built.highlight.value
  const instances = check.instances ?? []
  const same = current && current.instances.join() === instances.join() && JSON.stringify(current.at) === JSON.stringify(check.at ?? null)
  built.highlight.value = same ? null : { instances, at: check.at ?? null }
}

async function tryIt() {
  const id = state.value?.station.id
  if (!id) return
  filesRead.value++
  page.value = 'simulation'
  await loadStation(id)
}

const hint = computed(() =>
  pending.value
    ? `Point at a line to place the ${pending.value.name}, click to put it down · R turns it · Esc cancels`
    : onFloor.value
      ? 'Drag an arrow or the dot to move the stand with everything on it · click + to put an empty stand beside it'
      : selected.value?.place
        ? 'Drag an arrow to move it one way, or the dot both ways · 5 mm grid, 1 mm with Shift · tap an arrow or use the arrow keys for one step'
        : 'Click a module to select it · pick one on the left to add it · click a stand to put another beside it',
)
</script>

<template>
  <main class="builder">
    <aside class="palette">
      <nav class="tabs" role="tablist">
        <button role="tab" :class="{ active: paletteTab === 'add' }" :aria-selected="paletteTab === 'add'" @click="paletteTab = 'add'">Add</button>
        <button role="tab" :class="{ active: paletteTab === 'modules' }" :aria-selected="paletteTab === 'modules'" @click="paletteTab = 'modules'">In the station</button>
      </nav>
      <template v-if="paletteTab === 'add'">
        <section class="block">
          <h3 class="section-title">Start from</h3>
          <div class="row">
            <select v-model="from" aria-label="Start from">
              <option value="">An empty station</option>
              <optgroup v-for="g in startGroups" :key="g.id" :label="g.name">
                <option v-if="g.empty" :value="`empty:${g.id}`">An empty station of {{ g.name }}</option>
                <option v-for="s in g.stations" :key="s.id" :value="s.id">{{ s.name }}{{ own.includes(s.id) ? ' (yours)' : '' }}</option>
              </optgroup>
            </select>
            <button :disabled="busy" @click="start(from)">Start</button>
          </div>
          <div v-if="from && own.includes(from)" class="row">
            <button v-if="confirming !== from" class="danger" @click="confirming = from">Delete this station</button>
            <template v-else>
              <span class="hint">Delete its folder for good?</span>
              <button class="danger" @click="confirmDelete">Delete</button>
              <button @click="confirming = ''">Keep</button>
            </template>
          </div>
        </section>
        <p v-if="pending" class="picked">Point at a line in the 3D view and click to put it down. R turns it, Esc cancels.</p>
        <section v-if="paletteCollections.length > 1" class="block">
          <label class="collection">
            <span class="section-title">Collection</span>
            <select v-model="paletteCollection" aria-label="Collection to add from">
              <option v-for="c in paletteCollections" :key="c.id" :value="c.id">{{ c.name }}</option>
              <option :value="ALL">All collections</option>
            </select>
          </label>
        </section>
        <section v-for="group in palette" :key="group.title" class="block">
          <h3 class="section-title">{{ group.title }}</h3>
          <button v-for="e in group.list" :key="e.id" class="item" :class="{ active: pending?.id === e.id }" :disabled="busy" :title="e.description" @click="pick(e)">
            <span>{{ e.name }}</span>
            <small class="mono">{{ (e.mounts ?? []).join(', ') }}</small>
          </button>
        </section>
      </template>
      <section v-else class="block">
        <button
          v-for="{ module: m, depth } in tree"
          :key="m.id"
          class="item"
          :class="{ active: selected?.id === m.id, station: m.place?.on === 'floor.area' }"
          :style="{ paddingLeft: `${4 + depth * 16}px` }"
          @click="built.selection.value = m.id"
        >
          <span>{{ m.name }}</span>
          <small class="mono">{{
            m.place?.on === 'floor.area' ? 'station' : m.place ? `${m.place.on.split('.').slice(-1)[0]} · ${m.place.at}` : m.parent ? 'part' : 'fixed'
          }}</small>
        </button>
      </section>
    </aside>

    <div class="stage">
      <nav class="modes" role="tablist" aria-label="Builder view">
        <button role="tab" :class="{ active: mode === 'build' }" :aria-selected="mode === 'build'" @click="mode = 'build'">Build in 3D</button>
        <button role="tab" :class="{ active: mode === 'wire' }" :aria-selected="mode === 'wire'" @click="((mode = 'wire'), cancelPlacing())">Wiring</button>
      </nav>
      <WiringView v-if="mode === 'wire'" />
      <StationViewer
        v-else
        building
        :hint="hint"
        :overlay="overlay"
        :placing="placing"
        :ghost="ghost?.instances ?? null"
        :ghost-pose="ghostPose"
        :handle="handle"
        @hover="hover"
        @place="placeAt"
        @handle="handled"
        @nudge="(axis, sign) => step(axis, sign)"
        @add-beside="addStand"
      />
    </div>

    <aside class="panel">
      <nav class="tabs" role="tablist">
        <button role="tab" :class="{ active: panelTab === 'settings' }" :aria-selected="panelTab === 'settings'" @click="panelTab = 'settings'">Settings</button>
        <button role="tab" :class="{ active: panelTab === 'checks' }" :aria-selected="panelTab === 'checks'" @click="panelTab = 'checks'">
          Checks
          <span class="count" :class="{ bad: checkCount > 0 }">{{ checkCount }}</span>
        </button>
      </nav>

      <p v-if="problem" class="problem" role="alert">{{ problem }}</p>

      <template v-if="panelTab === 'settings'">
        <section v-if="pending" class="block">
          <h3 class="section-title">Adding: {{ pending.name }}</h3>
          <p v-if="!hovered" class="hint">Point at one of the lines in the 3D view: that is where it can be screwed on.</p>
          <template v-else>
            <p class="hint mono">{{ hovered.key.split('#')[0] }} · {{ hovered.along }} mm</p>
            <p v-if="!check" class="hint">Checking…</p>
            <p v-else-if="check.ok" class="fits">Fits here.</p>
            <template v-else>
              <p class="problem">{{ check.reason }}</p>
              <button v-if="check.fix" :disabled="busy" @click="putDown(true)">{{ check.fix }} and put it here</button>
            </template>
            <div class="row">
              <button v-if="confirmPlace || check?.ok" :disabled="busy || !check?.ok" @click="putDown(false)">Put it here</button>
              <button :disabled="busy" title="R" @click="rotate">Turn 90°</button>
              <button @click="cancelPlacing">Cancel</button>
            </div>
          </template>
        </section>

        <section v-if="selected" class="block">
          <h3 class="section-title">{{ selected.name }}</h3>
          <p class="hint mono">{{ selected.id }} · {{ selected.module }}</p>
          <p v-if="selected.parent" class="hint">Part of {{ parentName }}</p>

          <template v-if="buildSettings.length">
            <h4 class="sub">Build settings</h4>
            <BuilderSetting v-for="p in buildSettings" :key="p.id" :param="p" :busy="busy" @set="(value) => change(p.id, value)" />
          </template>

          <template v-if="selected.place">
            <h4 class="sub">Place</h4>
            <p class="hint">
              On
              <span class="mono">{{ onFloor ? 'the floor' : selected.place.on }}</span>
            </p>
            <label v-if="slots > 1" class="field">
              <span>{{ onFloor ? 'Line across, 50 mm apart' : 'Slot' }}</span>
              <input type="number" min="0" :max="slots - 1" :value="selected.place.slot" @change="setSlot" />
            </label>
            <label class="field">
              <span>Along, mm</span>
              <input type="number" step="1" :value="shownAt" @change="setAt" />
            </label>
            <div class="row nudges">
              <button
                v-for="d in [-10, -1, 1, 10]"
                :key="d"
                :disabled="busy"
                @click="
                  move(selected, {
                    ...selected.place,
                    at: Math.round(selected.place.at + d),
                  })
                "
              >
                {{ d > 0 ? '+' : '' }}{{ d }}
              </button>
            </div>
            <div v-if="!onFloor" class="row nudges">
              <span class="hint">Turn</span>
              <button v-for="t in [0, 90, 180, 270]" :key="t" :class="{ active: selected.place.turn === t }" :disabled="busy" @click="turn(t)">{{ t }}°</button>
            </div>
            <div class="row">
              <button v-if="!onFloor" :disabled="busy" title="Moves it along its line to where it fits best with the rest" @click="fit(selected.id)">Fit</button>
              <button v-if="removable" class="danger" :disabled="busy" title="Backspace" @click="remove(selected)">Remove</button>
            </div>
          </template>
          <template v-else>
            <p class="hint">
              {{ selected.parent ? `Part of ${parentName}, where it stays.` : 'Stands at a fixed position.' }}
            </p>
            <div v-if="removable" class="row">
              <button class="danger" :disabled="busy" title="Backspace" @click="remove(selected)">Remove</button>
            </div>
          </template>
          <template v-if="selected.removed?.length">
            <h4 class="sub">Taken off</h4>
            <div v-for="part in selected.removed" :key="part.id" class="row taken">
              <span class="hint">{{ part.name }}</span>
              <button :disabled="busy" @click="putBack(part.id)">Put back</button>
            </div>
          </template>
          <template v-if="selectedMarks.length">
            <h4 class="sub">Lines up with</h4>
            <p v-for="(m, index) in selectedMarks" :key="index" class="hint mark">
              {{ m.label }}
              <span class="mono">({{ m.at.x.toFixed(1) }}, {{ m.at.y.toFixed(1) }})</span>
            </p>
          </template>
          <details v-if="physicsSettings.length">
            <summary class="sub">Physics</summary>
            <p class="hint">How it moves and senses. The simulation can change some of these while it runs.</p>
            <BuilderSetting v-for="p in physicsSettings" :key="p.id" :param="p" :busy="busy" @set="(value) => change(p.id, value)" />
          </details>
        </section>

        <section class="block">
          <h3 class="section-title">Station</h3>
          <label class="field">
            <span>Name</span>
            <input v-model="name" @change="rename" />
          </label>
          <label class="field">
            <span>Description</span>
            <textarea v-model="description" rows="2" @change="rename" />
          </label>
          <div class="row">
            <button :disabled="busy || !state" @click="save(false)">Save</button>
            <button :disabled="busy || !state?.saved" @click="tryIt">Try in Simulation</button>
          </div>
          <p v-if="exists" class="problem">
            A station with this name is already saved.
            <button @click="save(true)">Replace it</button>
          </p>
          <p v-if="state?.saved" class="hint mono" :title="state.saved">Saved to {{ state.saved }}</p>
        </section>
      </template>

      <section v-else class="block">
        <p v-if="!checkCount" class="hint">Nothing found: the station is built so that it can work.</p>
        <p v-else class="hint">Click one to show it in the 3D view.</p>
        <button
          v-for="(check, index) in state?.scene.checks ?? []"
          :key="index"
          class="check"
          :class="{
            active: built.highlight.value?.instances.join() === (check.instances ?? []).join(),
          }"
          title="Show it in the 3D view"
          @click="show(check)"
        >
          {{ check.text }}
        </button>
      </section>
    </aside>
  </main>
</template>

<style scoped>
.builder {
  display: grid;
  grid-template-columns: 240px minmax(0, 1fr) 340px;
  min-height: 0;
}

.palette,
.panel {
  background: var(--panel);
  min-height: 0;
  overflow: auto;
}

.collection {
  align-items: center;
  display: flex;
  gap: 8px;
}

.collection select {
  flex: 1;
}

.palette {
  border-right: 1px solid var(--line);
}

.panel {
  border-left: 1px solid var(--line);
}

.stage {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr) auto;
  min-width: 0;
  min-height: 0;
}

.modes {
  background: var(--panel);
  border-bottom: 1px solid var(--line);
  display: flex;
  gap: 4px;
  padding: 4px 8px;
}

.modes button {
  background: none;
  border: 1px solid transparent;
  border-radius: 4px;
  color: var(--text-dim);
  padding: 4px 12px;
}

.modes button.active {
  background: var(--panel-raised);
  border-color: var(--accent);
  color: var(--text);
}

.item.station span {
  font-weight: 600;
}

.block {
  border-bottom: 1px solid var(--line);
  padding: 12px;
}

.row {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}

.row select {
  flex: 1;
  min-width: 0;
}

.nudges button {
  min-width: 44px;
  padding: 4px 6px;
}

.item {
  align-items: baseline;
  background: none;
  border: 1px solid transparent;
  border-radius: 0;
  display: flex;
  gap: 8px;
  justify-content: space-between;
  padding: 5px 4px;
  text-align: left;
  width: 100%;
}

.item:hover:not(:disabled) {
  background: var(--panel-raised);
}

.item.part {
  padding-left: 18px;
}

.taken {
  align-items: center;
  justify-content: space-between;
}

.item.active {
  background: var(--panel-raised);
  border-color: var(--accent);
}

.item small {
  color: var(--text-dim);
  font-size: 11px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.picked {
  background: var(--panel-raised);
  border-left: 2px solid var(--accent);
  font-size: 13px;
  margin: 0;
  padding: 8px 12px;
}

button.active {
  border-color: var(--accent);
}

.danger {
  color: var(--danger);
}

.field {
  display: grid;
  font-size: 13px;
  gap: 3px;
  margin-top: 8px;
}

.field span {
  color: var(--text-dim);
}

.field input,
.field textarea,
select {
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: 2px;
  color: var(--text);
  font: inherit;
  padding: 4px 6px;
}

.sub {
  color: var(--text-dim);
  cursor: pointer;
  font-size: 12px;
  font-weight: 600;
  margin: 12px 0 0;
}

.hint {
  color: var(--text-dim);
  font-size: 12px;
  line-height: 1.4;
  margin: 6px 0 0;
  overflow-wrap: anywhere;
}

.problem,
.check {
  border: 0;
  border-left: 2px solid var(--danger);
  border-radius: 0;
  font-size: 13px;
  line-height: 1.4;
  margin: 6px 0 0;
  padding: 2px 0 2px 8px;
}

.check {
  background: none;
  display: block;
  text-align: left;
  width: 100%;
}

.check:hover,
.check.active {
  background: var(--panel-raised);
}

.fits {
  border-left: 2px solid #3cc97a;
  font-size: 13px;
  margin: 6px 0 0;
  padding: 2px 0 2px 8px;
}

.mark::before {
  color: var(--accent);
  content: '◆ ';
}

.panel > .problem {
  margin: 10px 12px;
}

.tabs {
  background: var(--panel);
  border-bottom: 1px solid var(--line);
  display: flex;
  position: sticky;
  top: 0;
  z-index: 1;
}

.tabs button {
  background: none;
  border: 0;
  border-bottom: 2px solid transparent;
  border-radius: 0;
  color: var(--text-dim);
  flex: 1;
  padding: 9px 6px;
}

.tabs button.active {
  border-bottom-color: var(--accent);
  color: var(--text);
}

.count {
  background: var(--panel-raised);
  border-radius: 8px;
  font-size: 11px;
  margin-left: 4px;
  padding: 1px 6px;
}

.count.bad {
  background: var(--danger);
  color: #fff;
}

.field small {
  margin-left: 4px;
}

.field.flag {
  align-items: center;
  display: flex;
  gap: 8px;
}

.field.flag span {
  color: var(--text);
}

.reset {
  background: none;
  border: 0;
  color: var(--accent);
  cursor: pointer;
  font-size: 12px;
  padding: 0 4px;
}

details .sub {
  margin-top: 14px;
}
</style>
