<script setup lang="ts">
import { computed } from 'vue'
import { useSim } from '../sim'
import type { SceneWidget } from '../../bindings/github.com/toblu96/taktwin/internal/model'

// Renders the widgets a module or station file defines. It holds no knowledge of
// any specific module.
const props = defineProps<{ widgets: SceneWidget[] }>()

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
</script>

<template>
  <div class="widgets">
    <div v-if="round.length" class="round">
      <div v-for="widget in round" :key="widget.control || widget.label" class="control">
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
        <span>{{ widget.label }}</span>
        <small class="mono">{{ widget.tag }}</small>
      </div>
    </div>

    <template v-for="(widget, index) in others" :key="index">
      <div v-if="widget.type === 'switch'" class="switch" role="group" :aria-label="widget.label">
        <span>{{ widget.label }} <small class="mono">{{ widget.tag }}</small></span>
        <div class="segmented">
          <button v-for="(label, i) in widget.labels ?? []" :key="label" :class="{ active: stateOn(widget.state) === (i === 1) }" @click="setKey(widget, i === 1)">
            {{ label }}
          </button>
        </div>
      </div>

      <div v-else-if="widget.type === 'fill'" class="fill" :class="{ bad: fill(widget).wrong }">
        <span v-if="widget.label" class="fill-label">{{ widget.label }}</span>
        <div class="pips" :aria-label="`${fill(widget).pieces.length} of ${widget.capacity}`">
          <span v-for="(k, i) in fill(widget).pieces" :key="i" class="swatch" :style="{ background: kind(k)?.material.color }" />
          <span v-for="i in fill(widget).free" :key="`free-${i}`" class="swatch free" />
        </div>
        <p class="note">
          <template v-if="fill(widget).wrong">{{ fill(widget).wrong }} wrongly sorted</template>
          <template v-else>{{ fill(widget).pieces.length }} of {{ widget.capacity }}</template>
        </p>
      </div>

      <button
        v-else-if="widget.type === 'button'"
        class="action"
        :disabled="stateOn(widget.disabledWhen)"
        @click="runAction(widget.action ?? '', widget.args)"
      >
        <span v-if="widget.swatch" class="swatch" :style="{ background: widget.swatch }" />{{ widget.label }}
      </button>

      <section v-else-if="widget.type === 'group'" class="group">
        <h3 class="section-title">
          {{ widget.label }} <span v-if="widget.tag" class="tag">{{ widget.tag }}</span>
        </h3>
        <WidgetList :widgets="widget.widgets ?? []" />
      </section>
    </template>
  </div>
</template>

<style scoped>
.widgets {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.round {
  display: grid;
  gap: 4px;
  grid-template-columns: repeat(5, 1fr);
}

.control {
  align-items: center;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

small {
  color: var(--text-dim);
  font-size: 10px;
}

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
  background: var(--accent);
  box-shadow: 0 0 12px var(--accent);
}

.switch {
  align-items: center;
  display: flex;
  justify-content: space-between;
}

.segmented {
  display: flex;
}

.segmented button {
  border-radius: 2px 0 0 6px;
  min-width: 58px;
}

.segmented button:last-child {
  border-radius: 0 2px 2px 0;
  margin-left: -1px;
}

.segmented .active {
  background: var(--accent);
  border-color: var(--accent);
  color: #1b1405;
  position: relative;
}

.fill-label {
  color: var(--text-dim);
  display: block;
  font-size: 12px;
  margin-bottom: 4px;
}

.pips {
  display: flex;
  gap: 4px;
}

.swatch {
  border-radius: 50%;
  box-shadow: inset 0 0 0 1px #ffffff2e;
  display: inline-block;
  flex: none;
  height: 14px;
  width: 14px;
}

.swatch.free {
  box-shadow: inset 0 0 0 1px var(--line);
}

.note {
  color: var(--text-dim);
  font-size: 12px;
  margin: 4px 0 0;
}

.bad .note {
  color: var(--danger);
  font-weight: 600;
}

.action {
  align-items: center;
  align-self: flex-start;
  display: inline-flex;
  gap: 6px;
}

.group .section-title {
  margin-bottom: 6px;
}

.tag {
  color: var(--text);
  font-weight: 500;
  letter-spacing: 0;
  margin-left: 4px;
  text-transform: none;
}
</style>
