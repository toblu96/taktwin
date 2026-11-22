<script setup lang="ts">
import { computed } from 'vue'
import { useSim } from '../sim'
import type { SceneWidget } from '../../bindings/github.com/toblu96/taktwin/internal/model'
import UiSegmented from '../ui/UiSegmented.vue'

// Renders the widgets a module or station file defines. It holds no knowledge of
// any specific module. The console's push buttons and lamps look like the hardware, in its own
// colours; the rest uses the app's controls. Inline, as in the strip below the 3D view, each
// group's widgets stand in a row.
const props = defineProps<{ widgets: SceneWidget[]; inline?: boolean }>()

const { operate, runAction, scene, snapshot, stateOn } = useSim()

const round = computed(() => props.widgets.filter((w) => w.type === 'push' || w.type === 'lamp'))
const others = computed(() => props.widgets.filter((w) => w.type !== 'push' && w.type !== 'lamp'))

const kind = (id: string) => scene.value?.workpieces.kinds?.find((k) => k.id === id)

function fill(widget: SceneWidget) {
  const pieces = snapshot.value?.bins?.[widget.fill] ?? []
  const wrong = widget.expects ? pieces.filter((k) => k !== widget.expects).length : 0
  return { pieces, wrong, free: Math.max(0, (widget.capacity ?? 0) - pieces.length) }
}

const press = (widget: SceneWidget, down: boolean) => operate(widget.control ?? '', down, false)
const setKey = (widget: SceneWidget, on: boolean) => {
  if (stateOn(widget.state) !== on) operate(widget.control ?? '', true, true, widget.state)
}
const keyOptions = (widget: SceneWidget) => (widget.labels ?? []).slice(0, 2).map((label, i) => ({ value: String(i), label }))
</script>

<template>
  <div class="widgets flex gap-2" :class="inline ? 'flex-row flex-wrap items-center gap-1.5' : 'flex-col'">
    <div v-if="round.length" class="grid grid-cols-5 gap-1">
      <div v-for="widget in round" :key="widget.control || widget.label" class="flex flex-col items-center gap-0.5 text-center">
        <button
          v-if="widget.type === 'push'"
          class="push"
          :class="[widget.tone, { lit: stateOn(widget.lamp) }]"
          :aria-label="widget.label"
          @pointerdown="press(widget, true)"
          @pointerup="press(widget, false)"
          @pointerleave="press(widget, false)"
          @pointercancel="press(widget, false)"
          @keydown.space.prevent="press(widget, true)"
          @keyup.space.prevent="press(widget, false)"
        />
        <span v-else class="lamp" :class="[widget.tone, { lit: stateOn(widget.state) }]" />
        <span class="text-xs">{{ widget.label }}</span>
        <span class="font-mono text-[10px] text-muted">{{ widget.tag }}</span>
      </div>
    </div>

    <template v-for="(widget, index) in others" :key="index">
      <div v-if="widget.type === 'switch'" class="flex items-center justify-between gap-3">
        <span>{{ widget.label }} <span class="font-mono text-[10px] text-muted">{{ widget.tag }}</span></span>
        <UiSegmented
          :label="widget.label ?? 'Switch'"
          :options="keyOptions(widget)"
          :model-value="stateOn(widget.state) ? '1' : '0'"
          @update:model-value="(v: string) => setKey(widget, v === '1')"
        />
      </div>

      <div v-else-if="widget.type === 'fill'" class="flex gap-1.5" :class="inline ? 'items-center' : 'flex-col'">
        <span v-if="widget.label" class="text-xs text-muted">{{ widget.label }}</span>
        <span class="flex gap-1" role="img" :aria-label="`${fill(widget).pieces.length} of ${widget.capacity}`">
          <span v-for="(k, i) in fill(widget).pieces" :key="i" class="size-3.5 rounded-full shadow-[inset_0_0_0_1px_#ffffff2e]" :style="{ background: kind(k)?.material.color }" />
          <span v-for="i in fill(widget).free" :key="`free-${i}`" class="size-3.5 rounded-full shadow-[inset_0_0_0_1px_var(--t-control)]" />
        </span>
        <span class="text-xs" :class="fill(widget).wrong ? 'font-semibold text-danger' : 'text-muted'">
          <template v-if="fill(widget).wrong">{{ fill(widget).wrong }} wrongly sorted</template>
          <template v-else>{{ fill(widget).pieces.length }} of {{ widget.capacity }}</template>
        </span>
      </div>

      <button v-else-if="widget.type === 'button'" class="t-btn self-start" :disabled="stateOn(widget.disabledWhen)" @click="runAction(widget.action ?? '', widget.args)">
        <span v-if="widget.swatch" class="size-3 rounded-full shadow-[inset_0_0_0_1px_#ffffff2e]" :style="{ background: widget.swatch }" />{{ widget.label }}
      </button>

      <section v-else-if="widget.type === 'group'" class="min-w-0">
        <h3 class="t-section-title mb-1.5">
          {{ widget.label }}<span v-if="widget.tag" class="ml-1 font-mono font-normal tracking-normal text-fg normal-case">{{ widget.tag }}</span>
        </h3>
        <WidgetList :widgets="widget.widgets ?? []" :inline="inline" />
      </section>
    </template>
  </div>
</template>

<style scoped>
/* The console's push buttons and lamps, in the colours of the hardware they stand for, the same
   in a light and a dark look, as the 3D view is. */
.push {
  border: 2px solid #0007;
  border-radius: 50%;
  box-shadow: inset 0 -3px 0 #0005;
  height: 40px;
  margin-bottom: 4px;
  padding: 0;
  touch-action: none;
  width: 40px;
}

.push:active {
  box-shadow: inset 0 2px 3px #0008;
  transform: translateY(1px);
}

.push.green {
  background: #1e6b3c;
}

.push.green.lit {
  background: #55ef8c;
  box-shadow: 0 0 14px #55ef8c;
}

.push.red {
  background: #a5271d;
}

.push.blue {
  background: #1e4f86;
}

.push.blue.lit {
  background: #62b3ff;
  box-shadow: 0 0 14px #62b3ff;
}

.lamp {
  background: #4a3a18;
  border: 2px solid #0007;
  border-radius: 50%;
  height: 26px;
  margin: 7px 0 11px;
  width: 26px;
}

.lamp.lit {
  background: #f5b83d;
  box-shadow: 0 0 12px #f5b83d;
}
</style>
