<script setup lang="ts">
import { computed } from 'vue'
import {
  busy,
  cancelPlacing,
  check,
  confirming,
  fit,
  hovered,
  move,
  pending,
  problem,
  putBack,
  putDown,
  remove,
  rotate,
  selected,
  selectedMarks,
  setParam,
  state,
} from '../builder'
import UiSegmented from '../ui/UiSegmented.vue'
import BuilderSetting from './BuilderSetting.vue'
import IconDelete from '~icons/fluent/delete-16-regular'
import IconFit from '~icons/fluent/arrow-fit-16-regular'
import IconTurn from '~icons/fluent/arrow-rotate-clockwise-16-regular'
import IconCursor from '~icons/fluent/cursor-click-20-regular'
import IconChevron from '~icons/fluent/chevron-right-12-regular'
import UiField from '../ui/UiField.vue'

// The builder's side bar on the right: the module being added, with where it would go and
// whether it fits there, or the module selected, with where it is, how it is put together, the
// parts taken off it and, folded away, how it moves and senses. A module's handle in the 3D view
// moves it too; while it is dragged, its place here follows (draggedAt).
const props = defineProps<{ draggedAt: number | null }>()

const onFloor = computed(() => selected.value?.place?.on === 'floor.area')
const shownAt = computed(() => props.draggedAt ?? selected.value?.place?.at ?? 0)
const slots = computed(() => {
  const place = selected.value?.place
  return place ? (state.value?.sites ?? []).filter((s) => `${s.owner}.${s.id}` === place.on).length : 0
})
const parentName = computed(() => {
  const parent = selected.value?.parent
  return (state.value?.modules ?? []).find((m) => m.id === parent)?.name ?? parent ?? ''
})
const removable = computed(() => {
  const m = selected.value
  return !!m && (m.parent ? m.optional : onFloor.value ? m.optional : !!m.place)
})

// How a module is put together comes first; how it behaves folds away. Settings without a
// label are the module's own business, and not offered.
const buildSettings = computed(() => (selected.value?.params ?? []).filter((p) => p.label && p.kind === 'build'))
const physicsSettings = computed(() => (selected.value?.params ?? []).filter((p) => p.label && p.kind !== 'build'))
function change(param: string, value: string) {
  if (selected.value) void setParam(selected.value.id, param, value)
}

function setAt(event: Event) {
  const m = selected.value
  const value = Number((event.target as HTMLInputElement).value)
  if (m?.place && Number.isFinite(value)) void move(m, { ...m.place, at: value })
}
function setSlot(event: Event) {
  const m = selected.value
  const value = Number((event.target as HTMLInputElement).value)
  if (m?.place && Number.isInteger(value)) void move(m, { ...m.place, slot: value })
}
function nudge(d: number) {
  const m = selected.value
  if (m?.place) void move(m, { ...m.place, at: Math.round(m.place.at + d) })
}
const turn = computed({
  get: () => String(selected.value?.place?.turn ?? 0),
  set: (v: string) => {
    const m = selected.value
    if (m?.place) void move(m, { ...m.place, turn: Number(v) })
  },
})
const field = 't-input w-24 text-right font-mono text-xs select-text'
</script>

<template>
  <aside class="flex h-full min-h-0 flex-col bg-s1" aria-label="Details">
    <p v-if="problem" class="m-0 shrink-0 border-0 border-b border-solid border-line px-3 py-2 text-danger" role="alert">{{ problem }}</p>

    <template v-if="pending">
      <header class="shrink-0 border-0 border-b border-solid border-line px-3 py-2.5">
        <p class="m-0 text-xs text-muted">Adding</p>
        <h2 class="m-0 truncate text-[15px] font-semibold">{{ pending.name }}</h2>
      </header>
      <section class="t-section flex flex-col gap-2">
        <p v-if="!hovered" class="m-0 text-muted">Point at one of the lines in the 3D view: that is where it can be screwed on.</p>
        <template v-else>
          <p class="m-0 font-mono text-xs text-muted">{{ hovered.key.split('#')[0] }} · {{ hovered.along }} mm</p>
          <p v-if="!check" class="m-0 text-muted">Checking…</p>
          <p v-else-if="check.ok" class="m-0 font-medium text-ok">Fits here.</p>
          <template v-else>
            <p class="m-0 text-danger">{{ check.reason }}</p>
            <button v-if="check.fix" class="t-btn self-start" :disabled="busy" @click="putDown(true)">{{ check.fix }} and put it here</button>
          </template>
        </template>
        <div class="flex flex-wrap gap-2">
          <button v-if="confirming || check?.ok" class="t-btn t-btn-primary" :disabled="busy || !check?.ok" @click="putDown(false)">Put it here</button>
          <button class="t-btn" :disabled="busy" title="R" @click="rotate"><IconTurn class="size-4" />Turn 90°</button>
          <button class="t-btn" title="Esc" @click="cancelPlacing">Cancel</button>
        </div>
      </section>
    </template>

    <template v-else-if="selected">
      <header class="shrink-0 border-0 border-b border-solid border-line px-3 py-2.5">
        <h2 class="m-0 truncate text-[15px] font-semibold">{{ selected.name }}</h2>
        <p class="m-0 truncate font-mono text-[11px] text-muted">{{ selected.module }}<template v-if="selected.parent"> · part of {{ parentName }}</template></p>
      </header>
      <div class="min-h-0 flex-1 overflow-y-auto">
        <section class="t-section flex flex-col gap-2.5">
          <h3 class="t-section-title mb-0">Place</h3>
          <template v-if="selected.place">
            <p class="m-0 text-muted">On <span class="font-mono text-xs text-fg">{{ onFloor ? 'the floor' : selected.place.on }}</span></p>
            <label v-if="slots > 1" class="flex items-center gap-3">
              <span class="flex-1">{{ onFloor ? 'Line across, 50 mm apart' : 'Slot' }}</span>
              <UiField type="number" :class="field" min="0" :max="slots - 1" :value="selected.place.slot" :disabled="busy" @change="setSlot" />
              <span class="w-9" />
            </label>
            <label class="flex items-center gap-3">
              <span class="flex-1">Along</span>
              <UiField type="number" :class="field" step="1" :value="shownAt" :disabled="busy" @change="setAt" />
              <span class="w-9 text-xs text-muted">mm</span>
            </label>
            <div class="flex items-center gap-1.5">
              <span class="flex-1 text-xs text-muted">Nudge, or the arrow keys</span>
              <button v-for="d in [-10, -1, 1, 10]" :key="d" class="t-btn h-6 w-10 justify-center px-0 font-mono text-xs" :disabled="busy" @click="nudge(d)">
                {{ d > 0 ? '+' : '' }}{{ d }}
              </button>
            </div>
            <div v-if="!onFloor" class="flex items-center gap-3">
              <span class="flex-1">Turned</span>
              <UiSegmented v-model="turn" label="Turned" :options="[0, 90, 180, 270].map((t) => ({ value: String(t), label: `${t}°` }))" />
            </div>
            <div class="flex flex-wrap gap-2">
              <button v-if="!onFloor" class="t-btn" :disabled="busy" title="Moves it along its line to where it fits best with the rest" @click="fit(selected.id)">
                <IconFit class="size-4" />Fit
              </button>
              <button v-if="removable" class="t-btn text-danger" :disabled="busy" title="Backspace or Delete" @click="remove(selected)"><IconDelete class="size-4" />Remove</button>
            </div>
          </template>
          <template v-else>
            <p class="m-0 text-muted">{{ selected.parent ? `Part of ${parentName}, where it stays.` : 'It stands at a fixed place.' }}</p>
            <button v-if="removable" class="t-btn self-start text-danger" :disabled="busy" title="Backspace or Delete" @click="remove(selected)">
              <IconDelete class="size-4" />Remove
            </button>
          </template>
        </section>

        <section v-if="buildSettings.length" class="t-section flex flex-col gap-3">
          <h3 class="t-section-title mb-0">Build settings</h3>
          <BuilderSetting v-for="p in buildSettings" :key="p.id" :param="p" :busy="busy" @set="(value) => change(p.id, value)" />
        </section>

        <section v-if="selected.removed?.length" class="t-section">
          <h3 class="t-section-title">Taken off</h3>
          <ul class="m-0 flex list-none flex-col gap-1.5 p-0">
            <li v-for="part in selected.removed" :key="part.id" class="flex items-center gap-2">
              <span class="flex-1 text-muted">{{ part.name }}</span>
              <button class="t-btn h-6 px-2 text-xs" :disabled="busy" @click="putBack(part.id)">Put back</button>
            </li>
          </ul>
        </section>

        <section v-if="selectedMarks.length" class="t-section">
          <h3 class="t-section-title">Lines up with</h3>
          <p v-for="(m, i) in selectedMarks" :key="i" class="m-0 text-muted">
            {{ m.label }} <span class="font-mono text-xs">({{ m.at.x.toFixed(1) }}, {{ m.at.y.toFixed(1) }})</span>
          </p>
        </section>

        <details v-if="physicsSettings.length" class="t-section group">
          <summary class="t-section-title mb-0 cursor-default list-none select-none [&::-webkit-details-marker]:hidden">
            <IconChevron class="mr-1 inline size-3 align-[-1px] group-open:rotate-90" />Physics
          </summary>
          <p class="m-0 mt-2 mb-3 text-xs text-muted">How it moves and senses. The simulation can change some of these while it runs.</p>
          <div class="flex flex-col gap-3">
            <BuilderSetting v-for="p in physicsSettings" :key="p.id" :param="p" :busy="busy" @set="(value) => change(p.id, value)" />
          </div>
        </details>
      </div>
    </template>

    <div v-else class="flex flex-1 flex-col items-center justify-center gap-2 p-6 text-center text-muted">
      <IconCursor class="size-6 text-faint" />
      <p class="m-0 max-w-60">Click a module in the 3D view or in the list on the left to change it, or pick one under Add to put it in.</p>
      <p class="m-0 max-w-60 text-xs">The project keeps every change. Switching editing off runs the station as it is built.</p>
    </div>
  </aside>
</template>
