<script setup lang="ts">
import { computed } from 'vue'

// The edge between two areas, dragged to resize one of them, as VS Code's sashes are. It lies
// over the border line, wider than it so that it is easy to catch, and turns teal while hovered
// or dragged. The keyboard reaches it too: arrows resize in steps, Home and End go to the limits.
// The model is the size in pixels of the area on the side named, which the sash grows by moving
// away from it.
const props = defineProps<{ side: 'left' | 'right' | 'bottom'; min: number; max: number; label: string }>()
const size = defineModel<number>({ required: true })

const across = computed(() => props.side === 'bottom')
const clamp = (v: number) => Math.round(Math.min(props.max, Math.max(props.min, v)))

function start(e: PointerEvent) {
  if (e.button !== 0) return
  e.preventDefault()
  const el = e.currentTarget as HTMLElement
  el.setPointerCapture(e.pointerId)
  const from = across.value ? e.clientY : e.clientX
  const was = size.value
  const sign = props.side === 'left' ? 1 : -1
  const move = (m: PointerEvent) => (size.value = clamp(was + sign * ((across.value ? m.clientY : m.clientX) - from)))
  const end = () => {
    el.removeEventListener('pointermove', move)
    el.removeEventListener('pointerup', end)
    el.removeEventListener('pointercancel', end)
  }
  el.addEventListener('pointermove', move)
  el.addEventListener('pointerup', end)
  el.addEventListener('pointercancel', end)
}

function key(e: KeyboardEvent) {
  const grow = props.side === 'left' ? 'ArrowRight' : props.side === 'right' ? 'ArrowLeft' : 'ArrowUp'
  const shrink = props.side === 'left' ? 'ArrowLeft' : props.side === 'right' ? 'ArrowRight' : 'ArrowDown'
  const step = e.shiftKey ? 64 : 16
  const to = e.key === grow ? size.value + step : e.key === shrink ? size.value - step : e.key === 'Home' ? props.min : e.key === 'End' ? props.max : null
  if (to === null) return
  e.preventDefault()
  size.value = clamp(to)
}
</script>

<template>
  <div
    role="separator"
    tabindex="0"
    :aria-label="label"
    :aria-orientation="across ? 'horizontal' : 'vertical'"
    :aria-valuenow="size"
    :aria-valuemin="min"
    :aria-valuemax="max"
    class="absolute z-20 touch-none outline-none select-none after:absolute after:bg-transparent hover:after:bg-focus focus-visible:after:bg-focus active:after:bg-focus"
    :class="
      across
        ? 'inset-x-0 -top-0.5 h-1 cursor-row-resize after:inset-x-0 after:top-px after:h-0.5'
        : [side === 'left' ? '-right-0.5' : '-left-0.5', 'inset-y-0 w-1 cursor-col-resize after:inset-y-0 after:left-px after:w-0.5']
    "
    @pointerdown="start"
    @keydown="key"
  />
</template>
