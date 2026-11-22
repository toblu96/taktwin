<script setup lang="ts">
import type { Component } from 'vue'

// A switch between two modes, drawn as a pair of icons in one square-cornered frame, with the
// mode that is on filled: one button and one Tab stop, which a click or Space flips, as a toggle
// does. Off is the first icon, on the second; the title names the mode that shows.
const props = defineProps<{ label: string; off: { icon: Component; label: string }; on: { icon: Component; label: string }; disabled?: boolean; title?: string }>()
const model = defineModel<boolean>({ required: true })
const half = 'grid w-8 place-items-center'
</script>

<template>
  <button
    type="button"
    role="switch"
    :aria-checked="model"
    :aria-label="label"
    :title="title ?? `${model ? props.on.label : props.off.label}: click for ${model ? props.off.label : props.on.label}`"
    :disabled="disabled"
    class="inline-flex h-7 shrink-0 cursor-default items-stretch overflow-hidden rounded-control border border-control bg-s0 p-0 disabled:opacity-45"
    @click="model = !model"
  >
    <span :class="[half, model ? 'text-muted' : 'bg-highlight text-fg']"><component :is="off.icon" class="size-4" /></span>
    <span class="w-px bg-control" />
    <span :class="[half, model ? 'bg-highlight text-fg' : 'text-muted']"><component :is="on.icon" class="size-4" /></span>
  </button>
</template>
