<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import WidgetList from '../components/WidgetList.vue'
import IconClose from '~icons/fluent/dismiss-16-regular'
import { bit, thumbnails, useSim } from '../sim'
import UiLamp from '../ui/UiLamp.vue'
import type { SceneInstance } from '../../bindings/github.com/toblu96/taktwin/internal/model'
import type { GhostPose, Handle, Overlay, SiteLine } from './build'
import { StationScene, type CameraView } from './scene'

const props = withDefaults(
  defineProps<{
    active?: boolean
    preview?: boolean
    root?: string | null
    hint?: string
    // The builder: no controls, but handles, a ghost to place and what modules line up with.
    building?: boolean
    overlay?: Overlay | null
    placing?: SiteLine[] | null
    ghost?: SceneInstance[] | null
    ghostPose?: GhostPose | null
    handle?: Handle | null
  }>(),
  {
    active: true,
    preview: false,
    root: null,
    hint: 'Click a module · press buttons on the console · double-click to focus · drag to orbit',
    building: false,
    overlay: null,
    placing: null,
    ghost: null,
    ghostPose: null,
    handle: null,
  },
)
const emit = defineEmits<{
  hover: [key: string, along: number]
  place: [key: string, along: number, touch: boolean]
  handle: [key: string, along: number, done: boolean]
  nudge: [axis: 'along' | 'across', sign: number]
  addBeside: [side: number]
}>()

const { actionMessage, focus, highlight, operate, pickWorkpiece, scene: sceneData, selected, selection, service, snapshot, workpiece } = useSim()

const host = ref<HTMLDivElement>()
const canvas = ref<HTMLCanvasElement>()
const failed = ref(false)
const popup = ref<HTMLDivElement>()

// The builder shows no controls: they are for running the station.
const popupWidgets = computed(() => (props.building ? [] : (selected.value?.controls ?? []).filter((w) => w.show !== 'panel')))

let scene: StationScene | undefined
let observer: ResizeObserver | undefined
let view: CameraView | undefined
let builtFrom = ''
const canvasKey = ref(0)
// The hint sits on the view in a quiet label, light as the view, that stays readable over the
// station without drawing the eye.
const chip = 'pointer-events-none absolute top-2 m-0 rounded-sm bg-white/45 px-1.5 py-0.5 text-[11px] text-black/60'

// The module under the pointer, named with its signals and their states in a label beside it,
// which follows the pointer without a Vue update. Not while building or in a preview.
const hovered = ref<string | null>(null)
const tip = ref<HTMLDivElement>()
let tipAt = { x: 0, y: 0 }
function hover(id: string | null, x: number, y: number) {
  if (props.building || props.preview) return
  hovered.value = id
  tipAt = { x, y }
  placeTip()
}
function placeTip() {
  const element = tip.value
  if (!element || !host.value) return
  const room = host.value.clientWidth
  const x = Math.min(tipAt.x + 16, room - element.offsetWidth - 8)
  const y = Math.min(tipAt.y + 16, host.value.clientHeight - element.offsetHeight - 8)
  element.style.transform = `translate(${Math.max(8, x)}px, ${Math.max(8, y)}px)`
}
const hoveredModule = computed(() => sceneData.value?.instances?.find((i) => i.id === hovered.value) ?? null)
// The signals of the module under the pointer and the modules inside it, by the pins that carry
// them, with their state from the PLC's image.
const hoveredSignals = computed(() => {
  const id = hovered.value
  const s = sceneData.value
  if (!id || !s) return []
  const plcs = s.plcs?.length ? s.plcs : [s.plc]
  const seen = new Set<string>()
  const out: { key: string; name: string; description: string; on: boolean; output: boolean }[] = []
  for (const i of s.instances ?? [])
    for (const c of i.connectors ?? [])
      for (const p of c.pins ?? []) {
        if (!p.address || !p.owner || (p.owner !== id && !p.owner.startsWith(`${id}/`))) continue
        const k = Math.max(
          0,
          plcs.findIndex((x) => (x.module ?? '') === (p.plc ?? '')),
        )
        const key = `${k}|${p.address}`
        if (seen.has(key)) continue
        seen.add(key)
        const plc = plcs[k]
        const output = !!plc?.outputs?.some((x) => x.address === p.address)
        const signal = (output ? plc?.outputs : plc?.inputs)?.find((x) => x.address === p.address)
        if (!signal) continue
        const view = snapshot.value?.plcs?.[k]
        out.push({ key, name: signal.name, description: signal.description ?? '', on: bit(output ? view?.outputs : view?.inputs, signal.byte, signal.bit), output })
      }
  return out
})
watch([hoveredModule, () => hoveredSignals.value.length], () => void nextTick(placeTip))

// The colour of focus, as the theme has it, for the box round a signal's device.
const focusColor = () => getComputedStyle(document.documentElement).getPropertyValue('--t-focus').trim() || 'teal'

function start() {
  if (scene || !sceneData.value || !canvas.value) return
  try {
    scene = new StationScene(
      canvas.value,
      sceneData.value,
      {
        select: (id) => {
          selection.value = id === props.root ? null : id
        },
        pickWorkpiece: (id) => pickWorkpiece(id),
        control: (name, down, toggle) => operate(name, down, toggle),
        rendered: placePopup,
        hover,
        build: {
          hover: (key, along) => emit('hover', key, along),
          place: (key, along, touch) => emit('place', key, along, touch),
          handle: (key, along, done) => emit('handle', key, along, done),
          nudge: (axis, sign) => emit('nudge', axis, sign),
          addBeside: (side) => emit('addBeside', side),
        },
      },
      // Workpieces are picked where they can be taken off, as in a running station.
      { fit: props.preview, view, building: props.building, workpieces: !props.building && !!service.TakeWorkpiece },
    )
  } catch (error) {
    console.error(error)
    failed.value = true
    return
  }
  if (import.meta.env.MODE === 'development') (window as any)[props.preview ? '__preview' : '__station'] = scene
  observer?.disconnect()
  observer = new ResizeObserver(([entry]) => {
    const { width, height } = entry.contentRect
    if (width > 0 && height > 0) scene?.resize(width, height)
  })
  observer.observe(host.value!)
  scene.setActive(props.active)
  if (scene.build) {
    if (props.overlay) scene.build.setOverlay(props.overlay)
    scene.build.setPlacing(props.placing)
    scene.build.setGhost(props.ghost)
    scene.build.poseGhost(props.ghostPose)
    scene.build.setHandle(props.handle)
  }
  scene.highlight(highlight.value?.instances ?? [], highlight.value?.at ?? null)
  scene.focus(focus.value, focusColor())
  if (snapshot.value) scene.update(snapshot.value)
  scene.select(selection.value)
  scene.pickWorkpiece(workpiece.value)
}

// Keeps the popup on its module as the camera moves, without a Vue update.
function placePopup() {
  const element = popup.value
  const id = selection.value
  if (!element || !id || !host.value) return
  const at = scene?.anchor(id)
  if (!at) return
  const margin = 8
  const gap = 40
  const width = element.offsetWidth
  const height = element.offsetHeight
  const room = host.value.clientWidth
  let x = at.x - width / 2
  let y = at.y - height - 12
  // With no room above its module, sit beside it instead of covering it.
  if (y < margin) {
    x = at.x < room / 2 ? at.x - width - gap : at.x + gap
    y = at.y
  }
  x = Math.min(Math.max(x, margin), room - width - margin)
  y = Math.min(Math.max(y, margin), host.value.clientHeight - height - margin)
  element.style.transform = `translate(${x}px, ${y}px)`
}

onMounted(start)

// The simulation's own view takes the picture a project keeps of its station.
const takeThumbnail = () => scene?.thumbnail() ?? null
if (!props.preview && !props.building) thumbnails.take = takeThumbnail

// A new scene in a preview: a parameter can change the shape, and the camera stays
// where it is unless it is another entry.
async function rebuild() {
  const sameEntry = scene && sceneData.value && builtFrom === sceneData.value.id
  view = sameEntry ? scene?.view() : undefined
  scene?.dispose()
  scene = undefined
  canvasKey.value++
  await nextTick()
  builtFrom = sceneData.value?.id ?? ''
  start()
}
watch(sceneData, rebuild)
watch(highlight, (h) => scene?.highlight(h?.instances ?? [], h?.at ?? null))
watch(focus, (f) => scene?.focus(f, focusColor()))
watch(
  () => props.overlay,
  (overlay) => overlay && scene?.build?.setOverlay(overlay),
)
watch(
  () => props.placing,
  (placing) => scene?.build?.setPlacing(placing),
)
watch(
  () => props.ghost,
  (ghost) => scene?.build?.setGhost(ghost),
)
watch(
  () => props.ghostPose,
  (pose) => scene?.build?.poseGhost(pose),
)
watch(
  () => props.handle,
  (handle) => scene?.build?.setHandle(handle),
)
watch(
  () => props.active,
  (active) => scene?.setActive(active),
)
watch(snapshot, (value) => {
  if (value) scene?.update(value)
})
watch(workpiece, (id) => scene?.pickWorkpiece(id))
watch(selection, (id) => {
  scene?.select(id)
  void nextTick(placePopup)
})

onBeforeUnmount(() => {
  if (thumbnails.take === takeThumbnail) thumbnails.take = null
  observer?.disconnect()
  scene?.dispose()
})
</script>

<template>
  <!-- The 3D view keeps its own colours in a light and a dark look, as the station does. -->
  <div ref="host" class="relative min-h-0 bg-[#d5dcdc]">
    <canvas :key="canvasKey" ref="canvas" class="absolute inset-0 block size-full touch-none" aria-label="3D view" />
    <p v-if="failed" class="absolute top-1/2 left-1/2 m-0 -translate-1/2 rounded-card bg-s1 px-4 py-3 text-danger shadow-pop" role="alert">
      3D view unavailable: this machine could not start WebGL.
    </p>
    <p v-else-if="hint" :class="chip" class="left-2 max-w-[calc(100%-1rem)]">{{ hint }}</p>
    <div
      v-if="hoveredModule && hoveredModule.id !== selection"
      ref="tip"
      class="t-popup pointer-events-none absolute top-0 left-0 max-w-72 min-w-0 px-2.5 py-2 text-xs"
      role="tooltip"
    >
      <p class="m-0 font-semibold">{{ hoveredModule.name }}</p>
      <ul v-if="hoveredSignals.length" class="m-0 mt-1.5 flex list-none flex-col gap-1 p-0">
        <li v-for="s in hoveredSignals" :key="s.key" class="flex min-w-0 items-center gap-2">
          <UiLamp :on="s.on" :out="s.output" />
          <span class="shrink-0 font-semibold">{{ s.name }}</span>
          <span class="truncate text-muted">{{ s.description }}</span>
        </li>
      </ul>
    </div>
    <div v-if="popupWidgets.length" ref="popup" class="t-popup absolute top-0 left-0 min-w-44 p-3" role="dialog" :aria-label="selected?.name">
      <header class="mb-2 flex items-center justify-between gap-3">
        <strong class="font-semibold">{{ selected?.name }}</strong>
        <button class="t-icon-btn size-6" aria-label="Close" @click="selection = null"><IconClose class="size-4" /></button>
      </header>
      <WidgetList :widgets="popupWidgets" />
    </div>
    <p
      v-if="actionMessage"
      class="absolute bottom-3 left-1/2 m-0 max-w-[calc(100%-2rem)] -translate-x-1/2 rounded-card border border-danger/40 bg-s2 px-3 py-1.5 text-danger shadow-pop first-letter:uppercase"
      role="status"
    >
      {{ actionMessage }}
    </p>
  </div>
</template>
