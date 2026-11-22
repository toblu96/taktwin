<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch, type Component } from 'vue'

// A row of options where exactly one is on, such as All, Stations and Exercises, with an indicator
// that slides to it. Each option is a button of its own, pressed or not, so Tab reaches every one
// of them, rather than the one stop with arrow keys of a radio group. The frame is drawn as an
// inset shadow rather than a border, so that the indicator lines up with the items to the pixel.
const props = // An option may count what it shows, as a badge after its label.
  defineProps<{ options: { value: string; label: string; icon?: Component; disabled?: boolean; title?: string; count?: number }[]; label: string }>()
const model = defineModel<string>({ required: true })

// Where the indicator sits: under the pressed button, measured whenever it or the sizes change.
const root = ref<HTMLElement | null>(null)
const box = ref<{ left: number; width: number } | null>(null)
function measure() {
  const on = root.value?.querySelector<HTMLElement>('[aria-pressed="true"]')
  box.value = on ? { left: on.offsetLeft, width: on.offsetWidth } : null
}
let sizes: ResizeObserver | undefined
onMounted(() => {
  measure()
  sizes = new ResizeObserver(measure)
  if (root.value) sizes.observe(root.value)
})
onBeforeUnmount(() => sizes?.disconnect())
watch([model, () => props.options], () => nextTick(measure), { deep: true })
</script>

<template>
  <div ref="root" role="group" :aria-label="label" class="relative inline-flex h-7 items-stretch gap-px rounded-control bg-s0 p-0.5 shadow-[inset_0_0_0_1px_var(--t-control)]">
    <span
      v-if="box"
      class="absolute top-0.5 bottom-0.5 rounded-sm bg-s3 shadow-[0_0_0_1px_var(--t-line),0_1px_2px_#0000002e] transition-[left,width] duration-150"
      :style="{ left: `${box.left}px`, width: `${box.width}px` }"
    />
    <button
      v-for="o in options"
      :key="o.value"
      type="button"
      :aria-pressed="model === o.value"
      :disabled="o.disabled"
      :title="o.title"
      class="relative z-10 inline-flex h-full cursor-default items-center gap-1.5 rounded-sm border-0 bg-transparent px-2.5 py-0 text-[13px] leading-none text-muted select-none hover:text-fg focus-visible:-outline-offset-2 disabled:opacity-45 disabled:hover:text-muted aria-pressed:text-fg"
      @click="model = o.value"
    >
      <component :is="o.icon" v-if="o.icon" class="size-4" />
      {{ o.label }}
      <span v-if="o.count !== undefined" class="rounded-full px-1.5 text-[11px] leading-4 font-medium" :class="model === o.value ? 'bg-s0 text-fg' : 'bg-s3 text-muted'">{{
        o.count
      }}</span>
    </button>
  </div>
</template>
