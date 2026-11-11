<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import WidgetList from '../components/WidgetList.vue'
import { useSim } from '../sim'
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

const { actionMessage, highlight, operate, scene: sceneData, selected, selection, snapshot } = useSim()

const host = ref<HTMLDivElement>()
const canvas = ref<HTMLCanvasElement>()
const failed = ref(false)
const load = ref('')
const popup = ref<HTMLDivElement>()

// The builder shows no controls: they are for running the station.
const popupWidgets = computed(() => (props.building ? [] : (selected.value?.controls ?? []).filter((w) => w.show !== 'panel')))

let scene: StationScene | undefined
let observer: ResizeObserver | undefined
let loadTimer = 0
let view: CameraView | undefined
let builtFrom = ''
const canvasKey = ref(0)

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
        control: (name, down, toggle) => operate(name, down, toggle),
        rendered: placePopup,
        build: {
          hover: (key, along) => emit('hover', key, along),
          place: (key, along, touch) => emit('place', key, along, touch),
          handle: (key, along, done) => emit('handle', key, along, done),
          nudge: (axis, sign) => emit('nudge', axis, sign),
          addBeside: (side) => emit('addBeside', side),
        },
      },
      { fit: props.preview, view, building: props.building },
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
  if (snapshot.value) scene.update(snapshot.value)
  scene.select(selection.value)
  window.clearInterval(loadTimer)
  loadTimer = window.setInterval(() => {
    const stats = scene?.stats()
    if (stats) load.value = `${stats.calls} draw calls · ${stats.triangles.toLocaleString('en')} triangles`
  }, 1000)
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
watch(selection, (id) => {
  scene?.select(id)
  void nextTick(placePopup)
})

onBeforeUnmount(() => {
  window.clearInterval(loadTimer)
  observer?.disconnect()
  scene?.dispose()
})
</script>

<template>
  <div ref="host" class="viewer">
    <canvas :key="canvasKey" ref="canvas" aria-label="3D view" />
    <p v-if="failed" class="failed">3D view unavailable: this machine could not start WebGL.</p>
    <p v-else class="hint">{{ hint }}</p>
    <p v-if="!building" class="load mono">{{ load }}</p>
    <div v-if="popupWidgets.length" ref="popup" class="popup" role="dialog" :aria-label="selected?.name">
      <header>
        <strong>{{ selected?.name }}</strong>
        <button class="close" aria-label="Close" @click="selection = null">×</button>
      </header>
      <WidgetList :widgets="popupWidgets" />
    </div>
    <p v-if="actionMessage" class="message" role="status">
      {{ actionMessage }}
    </p>
  </div>
</template>

<style scoped>
.popup {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 2px;
  box-shadow: 0 6px 20px #0005;
  left: 0;
  min-width: 170px;
  padding: 10px 12px 12px;
  position: absolute;
  top: 0;
}

.popup header {
  align-items: center;
  display: flex;
  gap: 12px;
  justify-content: space-between;
  margin-bottom: 8px;
}

.close {
  background: none;
  border: 0;
  color: var(--text-dim);
  font-size: 16px;
  line-height: 1;
  padding: 0 2px;
}

.message {
  background: var(--panel);
  border: 1px solid var(--danger);
  border-radius: 2px;
  bottom: 12px;
  color: var(--danger);
  left: 50%;
  margin: 0;
  padding: 6px 12px;
  position: absolute;
  transform: translateX(-50%);
}

.message::first-letter {
  text-transform: uppercase;
}

.viewer {
  background: #d5dcdc;
  min-height: 0;
  position: relative;
}

canvas {
  display: block;
  height: 100%;
  inset: 0;
  position: absolute;
  touch-action: none;
  width: 100%;
}

.hint,
.failed {
  color: #3f5356;
  font-size: 11px;
  left: 12px;
  margin: 0;
  pointer-events: none;
  position: absolute;
  top: 10px;
}

.load {
  color: #3f5356;
  font-size: 11px;
  margin: 0;
  pointer-events: none;
  position: absolute;
  right: 12px;
  top: 10px;
}

.failed {
  color: #8c2318;
  font-size: 13px;
}
</style>
