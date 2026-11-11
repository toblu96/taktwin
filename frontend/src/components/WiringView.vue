<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { WiringBox, WiringConnector, WiringPin } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { busy, loadWiring, movePin, plug, setSignal, state, unplug, wiring } from '../builder'

// The wiring of the station, as boxes from the PLC on the left to the I/O terminals on the
// right, with the cables drawn between their connectors. A connector picked, then another of
// its kind, moves the cable; a pin picked, then another of its kind on the same terminal,
// moves what it carries.

onMounted(() => {
  void loadWiring()
  window.addEventListener('resize', layout)
})
onBeforeUnmount(() => window.removeEventListener('resize', layout))
watch(
  () => state.value,
  () => void loadWiring(),
)

// Each connector type in its colour from the library, and the kind of box each connector is on.
const colors = computed(() => {
  const m = new Map<string, string>()
  for (const s of wiring.value?.stations ?? []) for (const b of s.boxes ?? []) for (const c of b.connectors ?? []) if (c.color) m.set(c.type, c.color)
  return m
})
const color = (type: string) => colors.value.get(type) ?? '#9aa7ad'
const kindOf = computed(() => {
  const m = new Map<string, string>()
  for (const s of wiring.value?.stations ?? []) for (const b of s.boxes ?? []) for (const c of b.connectors ?? []) m.set(c.ref, b.kind)
  return m
})

const stations = computed(() =>
  (wiring.value?.stations ?? []).map((station) => {
    const s = {
      ...station,
      boxes: (station.boxes ?? []).map((b) => ({
        ...b,
        connectors: (b.connectors ?? []).map((c) => ({ ...c, pins: c.pins ?? [] })).sort((x, y) => x.pins.length - y.pins.length),
      })),
    }
    return {
      ...s,
      columns: [
        { title: 'PLC and console', boxes: s.boxes.filter((b) => b.kind === 'plc' || b.kind === 'console') },
        { title: 'Interfaces', boxes: s.boxes.filter((b) => b.kind === 'interface') },
        { title: 'I/O terminals', boxes: s.boxes.filter((b) => b.kind === 'terminal') },
      ],
    }
  }),
)

// Which side of its box a connector's cable leaves from: towards the next column.
function side(box: WiringBox, c: WiringConnector) {
  if (box.kind === 'plc' || box.kind === 'console') return 'right'
  // An interface faces the PLC, on its left, with what is cabled to it or would fit the PLC's
  // connectors, and the terminals on its right with the rest.
  if (box.kind === 'interface') return towardsPLC(c) ? 'left' : 'right'
  return 'left'
}

// What each connector is known as, to say where its cable goes.
const names = computed(() => {
  const out = new Map<string, string>()
  for (const s of wiring.value?.stations ?? []) {
    for (const b of s.boxes ?? []) {
      for (const c of b.connectors ?? []) out.set(c.ref, `${b.of ? b.of + ' · ' : ''}${b.name} ${c.id}`)
    }
  }
  return out
})
function towardsPLC(c: WiringConnector) {
  const other = partner(c.ref)
  if (other) return ['plc', 'console'].includes(kindOf.value.get(other) ?? '')
  return (wiring.value?.stations ?? []).some((s) => (s.boxes ?? []).some((b) => b.kind === 'plc' && (b.connectors ?? []).some((x) => x.type === c.type)))
}

function partner(ref: string) {
  for (const c of wiring.value?.cables ?? []) {
    if (c.from === ref) return c.to
    if (c.to === ref) return c.from
  }
  return ''
}

// --- Picking a connector, then where its cable goes. ---

const picked = ref<WiringConnector | null>(null)
function clickConnector(c: WiringConnector) {
  if (busy.value) return
  const p = picked.value
  pickedPin.value = null
  if (!p || p.type !== c.type) {
    picked.value = c
    return
  }
  picked.value = null
  if (p.ref !== c.ref) void plug(p.ref, c.ref)
}
function unplugPicked() {
  const p = picked.value
  picked.value = null
  if (p) void unplug(p.ref)
}

// --- Picking a pin, then where what it carries goes. ---

const pinClass = (pin: string) =>
  pin.startsWith('AI') ? 'analogue input' : pin.startsWith('AQ') ? 'analogue output' : pin.startsWith('I') ? 'input' : pin.startsWith('Q') ? 'output' : pin
const pickedPin = ref<{ terminal: string; pin: WiringPin } | null>(null)
function clickPin(box: WiringBox, c: WiringConnector, pin: WiringPin) {
  if (busy.value) return
  const terminal = `${box.id}.${c.id}`
  const p = pickedPin.value
  picked.value = null
  if (!p || p.terminal !== terminal || pinClass(p.pin.pin) !== pinClass(pin.pin)) {
    pickedPin.value = { terminal, pin }
    return
  }
  pickedPin.value = null
  if (p.pin.pin !== pin.pin) void movePin(terminal, p.pin.pin, pin.pin)
}
const pinTarget = (box: WiringBox, c: WiringConnector, pin: WiringPin) =>
  !!pickedPin.value && pickedPin.value.terminal === `${box.id}.${c.id}` && pickedPin.value.pin.pin !== pin.pin && pinClass(pickedPin.value.pin.pin) === pinClass(pin.pin)

function rename(pin: WiringPin, event: Event) {
  const value = (event.target as HTMLInputElement).value
  if (pin.address && value !== pin.name) void setSignal(pin.plc, pin.address, value)
}

// --- The cables, drawn between the connectors' dots. ---

const canvas = ref<HTMLElement | null>(null)
const size = ref({ width: 0, height: 0 })
const paths = ref<{ key: string; d: string; color: string; active: boolean }[]>([])
function layout() {
  const root = canvas.value
  if (!root) return
  const base = root.getBoundingClientRect()
  const dots = new Map<string, { x: number; y: number; dir: number; type: string }>()
  root.querySelectorAll<HTMLElement>('[data-ref]').forEach((el) => {
    const r = el.getBoundingClientRect()
    dots.set(el.dataset.ref!, {
      x: r.left + r.width / 2 - base.left + root.scrollLeft,
      y: r.top + r.height / 2 - base.top + root.scrollTop,
      dir: el.dataset.side === 'right' ? 1 : -1,
      type: el.dataset.type ?? '',
    })
  })
  size.value = { width: root.scrollWidth, height: root.scrollHeight }
  paths.value = (wiring.value?.cables ?? []).flatMap((c) => {
    const a = dots.get(c.from)
    const b = dots.get(c.to)
    if (!a || !b) return []
    const pull = Math.max(50, Math.abs(b.x - a.x) / 2)
    const active = !!picked.value && (picked.value.ref === c.from || picked.value.ref === c.to)
    return [
      {
        key: `${c.from}-${c.to}`,
        d: `M ${a.x} ${a.y} C ${a.x + pull * a.dir} ${a.y}, ${b.x + pull * b.dir} ${b.y}, ${b.x} ${b.y}`,
        color: color(a.type),
        active,
      },
    ]
  })
}
watch([wiring, picked], () => nextTick(layout), { deep: false })

const hint = computed(() => {
  if (picked.value) {
    const to = partner(picked.value.ref)
    return `${names.value.get(picked.value.ref)}${to ? ` is cabled to ${names.value.get(to) ?? to}` : ' is free'}. Click another ${picked.value.label} connector to plug its cable there; where both have one, they swap.`
  }
  if (pickedPin.value) {
    const p = pickedPin.value.pin
    return `${pickedPin.value.pin.pin}${p.source ? ` carries ${p.function || 'a signal'} of ${p.source}` : ' is free'}. Click another ${pinClass(p.pin)} of this terminal to move it there; what is there comes here.`
  }
  return 'Click a connector, then another of its kind, to move a cable · click a pin of a terminal, then another, to move what it carries · names are what the PLC calls the signal'
})
</script>

<template>
  <div class="wiring">
    <p class="hint-bar">
      <span>{{ hint }}</span>
      <button v-if="picked && partner(picked.ref)" class="danger" :disabled="busy" @click="unplugPicked">Unplug</button>
      <button v-if="picked || pickedPin" @click="((picked = null), (pickedPin = null))">Cancel</button>
    </p>
    <div ref="canvas" class="canvas" @scroll="layout">
      <svg class="cables" :width="size.width" :height="size.height">
        <path v-for="p in paths" :key="p.key" :d="p.d" :stroke="p.color" :class="{ active: p.active }" />
      </svg>
      <p v-if="!stations.length" class="empty">Nothing to wire yet.</p>
      <section v-for="s in stations" :key="s.id" class="station">
        <h3 class="station-title">{{ s.name }}</h3>
        <div class="columns">
          <div v-for="col in s.columns" :key="col.title" class="column">
            <h4 class="column-title">{{ col.title }}</h4>
            <p v-if="!col.boxes.length" class="none">—</p>
            <article v-for="box in col.boxes" :key="box.id" class="box" :class="box.kind">
              <header>
                <strong>{{ box.name }}</strong>
                <small v-if="box.of">of {{ box.of }}</small>
              </header>
              <template v-for="c in box.connectors" :key="c.id">
                <div v-if="!c.pins.length" class="connector" :class="side(box, c)">
                  <button
                    class="dot"
                    :data-ref="c.ref"
                    :data-side="side(box, c)"
                    :data-type="c.type"
                    :style="{ '--c': color(c.type) }"
                    :class="{ picked: picked?.ref === c.ref, target: picked && picked.type === c.type && picked.ref !== c.ref, free: !partner(c.ref) }"
                    :title="c.label"
                    :disabled="busy"
                    @click="clickConnector(c)"
                  />
                  <span class="conn-name">
                    {{ c.id }}
                    <small
                      >{{ c.label.split(',')[0] }}<template v-if="c.byte !== undefined && c.byte !== null"> · byte {{ c.byte }}</template></small
                    >
                  </span>
                  <small class="to" :title="names.get(partner(c.ref)) ?? ''">{{ partner(c.ref) ? '↔ ' + (names.get(partner(c.ref)) ?? partner(c.ref)) : 'free' }}</small>
                </div>
                <table v-else class="pins">
                  <tr v-for="pin in c.pins" :key="pin.pin" :class="{ out: pin.output }">
                    <td>
                      <button
                        class="pin"
                        :class="{ picked: pickedPin?.terminal === `${box.id}.${c.id}` && pickedPin.pin.pin === pin.pin, target: pinTarget(box, c, pin) }"
                        :disabled="busy"
                        @click="clickPin(box, c, pin)"
                      >
                        {{ pin.pin }}
                      </button>
                    </td>
                    <td class="what" :title="pin.source ? `${pin.function} · ${pin.source}` : pin.function">
                      <template v-if="pin.source || pin.function">
                        {{ pin.function || 'signal' }}<small v-if="pin.source"> · {{ pin.source }}</small>
                      </template>
                      <span v-else class="spare">spare</span>
                    </td>
                    <td class="addr mono">{{ pin.address }}</td>
                    <td>
                      <input class="name mono" :value="pin.name" :disabled="!pin.address || busy" placeholder="—" title="What the PLC calls it" @change="rename(pin, $event)" />
                    </td>
                  </tr>
                </table>
              </template>
            </article>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.wiring {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
  min-height: 0;
  height: 100%;
}

.hint-bar {
  align-items: center;
  background: var(--panel);
  border-bottom: 1px solid var(--line);
  display: flex;
  font-size: 13px;
  gap: 8px;
  margin: 0;
  padding: 8px 12px;
}

.hint-bar span {
  color: var(--text-dim);
  flex: 1;
}

.canvas {
  overflow: auto;
  padding: 12px 16px 40px;
  position: relative;
}

.cables {
  left: 0;
  pointer-events: none;
  position: absolute;
  top: 0;
}

.cables path {
  fill: none;
  opacity: 0.8;
  stroke-width: 2.5;
}

.cables path.active {
  opacity: 1;
  stroke-width: 4.5;
}

.station {
  margin-bottom: 28px;
  position: relative;
}

.station-title {
  font-size: 15px;
  margin: 4px 0 10px;
}

.columns {
  display: grid;
  gap: 48px;
  grid-template-columns: minmax(170px, 230px) minmax(170px, 230px) minmax(300px, 1fr);
}

.column-title {
  color: var(--text-dim);
  font-size: 11px;
  letter-spacing: 0.06em;
  margin: 0 0 6px;
  text-transform: uppercase;
}

.none,
.empty {
  color: var(--text-dim);
  font-size: 12px;
}

.box {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 6px;
  margin-bottom: 14px;
  padding: 8px 10px;
  position: relative;
  z-index: 1;
}

.box.plc {
  border-top: 3px solid #a78bfa;
}

.box.interface {
  border-top: 3px solid #4ea8ff;
}

.box.terminal {
  border-top: 3px solid #3cc97a;
}

.box header {
  display: flex;
  gap: 6px;
  align-items: baseline;
  margin-bottom: 6px;
}

.box header small {
  color: var(--text-dim);
}

.connector {
  align-items: center;
  display: grid;
  gap: 8px;
  grid-template-columns: 18px auto 1fr;
  margin: 4px 0;
}

.connector.right {
  direction: rtl;
}

.connector.right > * {
  direction: ltr;
}

.conn-name small,
.to {
  color: var(--text-dim);
  font-size: 11px;
}

.to {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dot {
  background: var(--panel);
  border: 3px solid var(--c);
  border-radius: 50%;
  cursor: pointer;
  height: 18px;
  padding: 0;
  width: 18px;
}

.dot:not(.free) {
  background: var(--c);
}

.dot.target {
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--c) 35%, transparent);
}

.dot.picked {
  box-shadow: 0 0 0 4px var(--accent);
}

.pins {
  border-collapse: collapse;
  font-size: 12px;
  margin-top: 6px;
  table-layout: fixed;
  width: 100%;
}

.pins td:first-child {
  width: 46px;
}

.pins td.addr {
  width: 72px;
}

.pins td:last-child {
  width: 84px;
}

.what {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pins td {
  border-top: 1px solid var(--line);
  padding: 2px 4px;
}

.pins tr.out .pin {
  color: #ffb84d;
}

.pin {
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: 3px;
  font-family: var(--mono, monospace);
  min-width: 40px;
  padding: 2px 4px;
}

.pin.target {
  border-color: var(--accent);
  border-style: dashed;
}

.pin.picked {
  background: var(--accent);
  color: #000;
}

.what small,
.spare {
  color: var(--text-dim);
}

.addr {
  color: var(--text-dim);
  font-size: 11px;
  white-space: nowrap;
}

.name {
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: 2px;
  color: var(--text);
  font-size: 12px;
  padding: 2px 4px;
  width: 100%;
}

.danger {
  color: var(--danger);
}
</style>
