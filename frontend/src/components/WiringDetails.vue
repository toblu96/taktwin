<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { WiringBox, WiringConnector, WiringPin, WiringState } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { busy, movePin, setSignal, unplug } from '../builder'
import { simulation } from '../sim'
import { KINDS, pinClass, wiringIndex } from '../wiring'
import IoTable from './IoTable.vue'
import { useRouter } from 'vue-router'
import IconConnect from '~icons/fluent/plug-connected-16-regular'
import IconDrag from '~icons/fluent/re-order-dots-vertical-16-regular'
import IconUnplug from '~icons/fluent/plug-disconnected-16-regular'
import UiField from '../ui/UiField.vue'

// What a box of the wiring is, beside the diagram. A PLC: its signals, and the way to its
// connection, which is set up in the simulation's panel with the PLC selected. An I/O terminal: its pins, with what each
// carries and the name the PLC gives it. An interface or a console: where its connectors are
// cabled to. Edited, a terminal's pins swap what they carry, dragged onto another of their
// class or picked and then the other clicked, and the PLC's names change in place; a cable
// is unplugged here.
const props = defineProps<{ box: WiringBox | null; wiring: WiringState | null; editable?: boolean }>()

const index = computed(() => wiringIndex(props.wiring))
const partner = (ref: string) => index.value.partner.get(ref) ?? ''
const color = (type: string) => index.value.color.get(type) ?? 'var(--t-control)'

// The PLC a PLC box is, by its index among the station's PLCs.
const plcIndex = computed(() => {
  const plcs = simulation.scene.value?.plcs ?? []
  return Math.max(
    0,
    plcs.findIndex((p) => p.module === props.box?.id),
  )
})

// Shows the PLC in the simulation, where its connection is, running rather than being built.
const router = useRouter()
function showConnection() {
  simulation.selection.value = props.box?.id ?? null
  void router.push({ name: 'simulate' })
}

// --- Pins: swapping what two pins of a terminal carry. ---

const picked = ref<{ terminal: string; pin: string } | null>(null)
watch(
  () => [props.box?.id, props.editable],
  () => (picked.value = null),
)
const terminalOf = (c: WiringConnector) => `${props.box?.id}.${c.id}`
const fits = (c: WiringConnector, pin: WiringPin, from: { terminal: string; pin: string } | null) =>
  !!from && from.terminal === terminalOf(c) && from.pin !== pin.pin && pinClass(from.pin) === pinClass(pin.pin)

function clickPin(c: WiringConnector, pin: WiringPin) {
  if (!props.editable || busy.value) return
  const from = picked.value
  if (fits(c, pin, from)) {
    picked.value = null
    void movePin(terminalOf(c), from!.pin, pin.pin)
    return
  }
  picked.value = from?.pin === pin.pin && from.terminal === terminalOf(c) ? null : { terminal: terminalOf(c), pin: pin.pin }
}

const dragged = ref<{ terminal: string; pin: string } | null>(null)
const over = ref('')
function dragStart(c: WiringConnector, pin: WiringPin, e: DragEvent) {
  dragged.value = { terminal: terminalOf(c), pin: pin.pin }
  e.dataTransfer?.setData('text/plain', pin.pin)
  if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move'
}
function dragOver(c: WiringConnector, pin: WiringPin, e: DragEvent) {
  if (!fits(c, pin, dragged.value)) return
  e.preventDefault()
  over.value = `${terminalOf(c)}.${pin.pin}`
}
function drop(c: WiringConnector, pin: WiringPin) {
  const from = dragged.value
  dragged.value = null
  over.value = ''
  if (fits(c, pin, from)) void movePin(terminalOf(c), from!.pin, pin.pin)
}

function rename(pin: WiringPin, e: Event) {
  const value = (e.target as HTMLInputElement).value.trim()
  if (pin.address && value !== pin.name) void setSignal(pin.plc, pin.address, value)
}
</script>

<template>
  <aside class="flex h-full min-h-0 flex-col bg-s1" aria-label="Details">
    <template v-if="box">
      <header class="shrink-0 border-0 border-b border-solid border-line px-3 py-2.5">
        <h2 class="m-0 truncate text-[15px] font-semibold">{{ box.name }}</h2>
        <p class="m-0 text-xs text-muted">{{ KINDS[box.kind] ?? box.kind }}<template v-if="box.of"> of {{ box.of }}</template></p>
      </header>
      <div class="min-h-0 flex-1 overflow-y-auto">
        <template v-if="box.kind === 'plc'">
          <section class="t-section">
            <h3 class="t-section-title">Connection</h3>
            <p class="m-0 mb-2.5 text-xs text-muted">How the PLC is connected, to the soft PLC or a controller outside the app, is set up in the simulation.</p>
            <button class="t-btn" @click="showConnection"><IconConnect class="size-4" />Show the connection</button>
          </section>
          <section class="t-section">
            <h3 class="t-section-title">Signals</h3>
            <IoTable :index="plcIndex" />
          </section>
        </template>

        <template v-for="c in box.connectors ?? []" :key="c.ref">
          <section v-if="(c.pins ?? []).length" class="t-section">
            <h3 class="t-section-title">{{ c.id }}<span class="ml-1 font-normal tracking-normal normal-case">{{ c.label }}</span></h3>
            <p v-if="editable" class="m-0 mb-2 text-xs text-muted">Drag a pin onto another of its kind, or click one and then the other, to swap what they carry.</p>
            <ul class="m-0 list-none p-0">
              <li
                v-for="pin in c.pins ?? []"
                :key="pin.pin"
                class="flex min-h-9 items-center gap-2 rounded-[5px] border-0 border-b border-solid border-line px-1 py-1 last:border-b-0"
                :class="over === `${terminalOf(c)}.${pin.pin}` || fits(c, pin, picked) ? 'bg-highlight' : ''"
                :draggable="editable && !busy"
                @dragstart="dragStart(c, pin, $event)"
                @dragend="((dragged = null), (over = ''))"
                @dragover="dragOver(c, pin, $event)"
                @dragleave="over = ''"
                @drop.prevent="drop(c, pin)"
              >
                <IconDrag v-if="editable" class="size-4 shrink-0 cursor-grab text-faint" />
                <button
                  v-if="editable"
                  class="w-9 shrink-0 cursor-default rounded-sm border border-solid px-1 py-0.5 font-mono text-xs font-semibold"
                  :class="picked?.terminal === terminalOf(c) && picked.pin === pin.pin ? 'border-focus bg-highlight text-fg' : 'border-control bg-s2 text-fg hover:bg-s3'"
                  :aria-pressed="picked?.terminal === terminalOf(c) && picked.pin === pin.pin"
                  :aria-label="`Pin ${pin.pin}: pick it, then another ${pinClass(pin.pin)}, to swap them`"
                  :disabled="busy"
                  @click="clickPin(c, pin)"
                >
                  {{ pin.pin }}
                </button>
                <span v-else class="w-9 shrink-0 font-mono text-xs font-semibold">{{ pin.pin }}</span>
                <span class="min-w-0 flex-1 leading-tight">
                  <span v-if="pin.function || pin.source" class="block truncate">{{ pin.function || 'Signal' }}</span>
                  <span v-else class="block text-faint">Spare</span>
                  <span v-if="pin.source" class="block truncate text-xs text-muted">{{ pin.source }}</span>
                </span>
                <span class="w-28 shrink-0 text-right">
                  <UiField
                    v-if="editable && pin.address"
                    class="t-input h-6 px-1.5 text-right font-mono text-xs select-text"
                    :value="pin.name"
                    placeholder="—"
                    :aria-label="`What the PLC calls ${pin.pin}`"
                    :disabled="busy"
                    @change="rename(pin, $event)"
                  />
                  <span v-else class="block truncate font-mono text-xs">{{ pin.name || '' }}</span>
                  <span class="block font-mono text-[11px] text-muted" :title="pin.address">{{ pin.shown || pin.address }}</span>
                </span>
              </li>
            </ul>
          </section>
        </template>

        <section v-if="box.kind !== 'terminal' && box.kind !== 'plc'" class="t-section">
          <h3 class="t-section-title">Connectors</h3>
          <ul class="m-0 flex list-none flex-col p-0">
            <li v-for="c in box.connectors ?? []" :key="c.ref" class="flex min-h-9 items-center gap-2.5 border-0 border-b border-solid border-line py-1 last:border-b-0">
              <span
                class="size-3.5 shrink-0 rounded-full border-[3px] border-solid"
                :style="{ borderColor: color(c.type), background: partner(c.ref) ? color(c.type) : 'transparent' }"
              />
              <span class="min-w-0 flex-1 leading-tight">
                <span class="font-mono text-xs font-semibold">{{ c.id }}</span>
                <span class="ml-1.5 text-xs text-muted">{{ c.label.split(',')[0] }}</span>
                <span class="block truncate text-xs text-muted">{{ partner(c.ref) ? `to ${index.name.get(partner(c.ref))}` : 'free' }}</span>
              </span>
              <button v-if="editable && partner(c.ref)" class="t-btn h-6 px-2 text-xs" :disabled="busy" @click="unplug(c.ref)"><IconUnplug class="size-3.5" />Unplug</button>
            </li>
          </ul>
        </section>
      </div>
    </template>
    <p v-else class="m-auto max-w-56 p-6 text-center text-muted">Click a PLC or an I/O terminal in the diagram to see it here.</p>
  </aside>
</template>
