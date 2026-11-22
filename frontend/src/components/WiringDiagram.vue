<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { WiringBox, WiringConnector, WiringState } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { busy, plug, unplug } from '../builder'
import { KINDS, wiringIndex } from '../wiring'

// The wiring of the station as a diagram: for each stand, its PLC and console on the left, the
// interfaces in the middle and the I/O terminals on the right, each a box with its connectors,
// and the cables drawn between them. Clicking a box shows it beside the diagram. Edited, a
// connector picked and then another of its kind moves the cable there; where both have one,
// they swap.
const props = defineProps<{ wiring: WiringState | null; editable?: boolean; selected: string | null }>()
const emit = defineEmits<{ select: [id: string] }>()

const index = computed(() => wiringIndex(props.wiring))
const color = (type: string) => index.value.color.get(type) ?? 'var(--t-control)'
const partner = (ref: string) => index.value.partner.get(ref) ?? ''

const stations = computed(() =>
  (props.wiring?.stations ?? []).map((s) => {
    const boxes = s.boxes ?? []
    return {
      id: s.id,
      name: s.name,
      columns: [
        { title: 'PLC and console', boxes: boxes.filter((b) => b.kind === 'plc' || b.kind === 'console') },
        { title: 'Interfaces', boxes: boxes.filter((b) => b.kind === 'interface') },
        { title: 'I/O terminals', boxes: boxes.filter((b) => b.kind === 'terminal') },
      ].filter((c) => c.boxes.length),
    }
  }),
)

// Which side of its box a connector's cable leaves from: towards the column it goes to.
function side(box: WiringBox, c: WiringConnector) {
  if (box.kind === 'plc' || box.kind === 'console') return 'right'
  if (box.kind === 'terminal') return 'left'
  const other = index.value.boxOf.get(partner(c.ref))
  if (other) return other.kind === 'plc' || other.kind === 'console' ? 'left' : 'right'
  return index.value.boxes.some((b) => b.kind === 'plc' && (b.connectors ?? []).some((x) => x.type === c.type)) ? 'left' : 'right'
}

// A box's connectors by the side their cables leave from, each side a column of its own: an
// interface has both, side by side, the PLC and a terminal one.
function sides(box: WiringBox) {
  const all = box.connectors ?? []
  return (['left', 'right'] as const).map((which) => ({ side: which, connectors: all.filter((c) => side(box, c) === which) })).filter((s) => s.connectors.length)
}

const pinsOf = (c: WiringConnector) => {
  const pins = c.pins ?? []
  const inputs = pins.filter((p) => !p.output).length
  return pins.length ? `${inputs} in · ${pins.length - inputs} out` : ''
}

// --- Picking a connector, then where its cable goes. ---

const picked = ref<WiringConnector | null>(null)
watch(
  () => props.editable,
  () => (picked.value = null),
)
function clickConnector(c: WiringConnector) {
  if (!props.editable || busy.value) return
  const p = picked.value
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
const hint = computed(() => {
  const p = picked.value
  if (!p) return 'Click a connector, then another of its kind, to move its cable there.'
  const to = partner(p.ref)
  return `${index.value.name.get(p.ref)}${to ? ` is cabled to ${index.value.name.get(to) ?? to}` : ' is free'}. Click another ${p.label.split(',')[0]} connector to plug its cable there; where both have one, they swap.`
})

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
  paths.value = (props.wiring?.cables ?? []).flatMap((c) => {
    const a = dots.get(c.from)
    const b = dots.get(c.to)
    if (!a || !b) return []
    const pull = Math.max(40, Math.abs(b.x - a.x) / 2)
    const active = !!picked.value && (picked.value.ref === c.from || picked.value.ref === c.to)
    return [{ key: `${c.from}-${c.to}`, d: `M ${a.x} ${a.y} C ${a.x + pull * a.dir} ${a.y}, ${b.x + pull * b.dir} ${b.y}, ${b.x} ${b.y}`, color: color(a.type), active }]
  })
}
watch([() => props.wiring, picked, () => props.selected], () => nextTick(layout))
let sizes: ResizeObserver | undefined
onMounted(() => {
  sizes = new ResizeObserver(layout)
  if (canvas.value) sizes.observe(canvas.value)
  void nextTick(layout)
})
onBeforeUnmount(() => sizes?.disconnect())
</script>

<template>
  <div class="flex min-h-0 flex-col bg-s0">
    <div v-if="editable" class="flex min-h-9 shrink-0 items-center gap-2 border-0 border-b border-solid border-line bg-s1 px-3 py-1.5 text-xs text-muted">
      <span class="flex-1">{{ hint }}</span>
      <button v-if="picked && partner(picked.ref)" class="t-btn h-6 px-2 text-xs text-danger" :disabled="busy" @click="unplugPicked">Unplug</button>
      <button v-if="picked" class="t-btn h-6 px-2 text-xs" @click="picked = null">Cancel</button>
    </div>
    <div ref="canvas" class="relative min-h-0 flex-1 overflow-auto" @scroll="layout">
      <svg class="pointer-events-none absolute top-0 left-0 z-10" :width="size.width" :height="size.height" aria-hidden="true">
        <path v-for="p in paths" :key="p.key" :d="p.d" fill="none" :stroke="p.color" :stroke-width="p.active ? 3.5 : 2" :stroke-opacity="p.active ? 1 : 0.8" />
      </svg>
      <p v-if="!stations.length" class="m-0 p-6 text-muted">Nothing is wired yet.</p>
      <section v-for="s in stations" :key="s.id" class="relative px-6 pt-5 pb-6">
        <h3 class="t-section-title">{{ s.name }}</h3>
        <div class="flex items-start gap-20">
          <div v-for="col in s.columns" :key="col.title" class="flex w-64 shrink-0 flex-col gap-3">
            <h4 class="m-0 text-[11px] font-medium text-faint">{{ col.title }}</h4>
            <article
              v-for="box in col.boxes"
              :key="box.id"
              class="cursor-default rounded-card border bg-s1"
              :class="selected === box.id ? 'border-focus shadow-[0_0_0_1px_var(--t-focus)]' : 'border-line hover:border-control'"
              @click="emit('select', box.id)"
            >
              <button
                class="flex w-full cursor-default flex-col items-start rounded-t-card border-0 bg-transparent px-3 pt-2 pb-1.5 text-left text-fg hover:bg-hover focus-visible:-outline-offset-2"
                :aria-pressed="selected === box.id"
                @click.stop="emit('select', box.id)"
              >
                <span class="font-semibold">{{ box.name }}</span>
                <span class="text-xs text-muted">{{ KINDS[box.kind] ?? box.kind }}<template v-if="box.of"> of {{ box.of }}</template></span>
              </button>
              <div class="grid gap-x-2 px-2 pt-0.5 pb-2" :class="sides(box).length > 1 ? 'grid-cols-2' : 'grid-cols-1'">
                <ul v-for="list in sides(box)" :key="list.side" class="m-0 flex min-w-0 list-none flex-col gap-0.5 p-0">
                  <li
                    v-for="c in list.connectors"
                    :key="c.ref"
                    class="flex items-center gap-2 rounded-[5px] px-1 py-0.5"
                    :class="side(box, c) === 'right' ? 'flex-row-reverse text-right' : ''"
                  >
                    <button
                      v-if="editable"
                      class="grid size-6 shrink-0 cursor-default place-items-center rounded-full border-0 bg-transparent p-0 hover:bg-hover disabled:opacity-45"
                      :data-ref="c.ref"
                      :data-side="side(box, c)"
                      :data-type="c.type"
                      :aria-label="`${c.id}, ${c.label}${partner(c.ref) ? `, cabled to ${index.name.get(partner(c.ref))}` : ', free'}`"
                      :disabled="busy"
                      @click="clickConnector(c)"
                    >
                      <span
                        class="block size-4 rounded-full border-[3px] border-solid"
                        :style="{ borderColor: color(c.type), background: partner(c.ref) ? color(c.type) : 'var(--t-s1)' }"
                        :class="
                          picked?.ref === c.ref
                            ? 'shadow-[0_0_0_2px_var(--t-s1),0_0_0_4px_var(--t-accent)]'
                            : picked && picked.type === c.type
                              ? 'shadow-[0_0_0_2px_var(--t-s1),0_0_0_4px_var(--t-focus)]'
                              : ''
                        "
                      />
                    </button>
                    <span
                      v-else
                      class="size-4 shrink-0 rounded-full border-[3px] border-solid"
                      :style="{ borderColor: color(c.type), background: partner(c.ref) ? color(c.type) : 'var(--t-s1)' }"
                      :data-ref="c.ref"
                      :data-side="side(box, c)"
                      :data-type="c.type"
                    />
                    <span class="min-w-0 flex-1 leading-tight">
                      <span class="font-mono text-xs font-semibold">{{ c.id }}</span>
                      <span class="ml-1.5 text-[11px] text-muted"
                        >{{ c.label.split(',')[0] }}<template v-if="c.byte !== undefined && c.byte !== null"> · byte {{ c.byte }}</template></span
                      >
                      <span v-if="pinsOf(c)" class="block text-[11px] text-faint">{{ pinsOf(c) }}</span>
                      <span v-else-if="!partner(c.ref)" class="block text-[11px] text-faint">free</span>
                    </span>
                  </li>
                </ul>
              </div>
            </article>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
