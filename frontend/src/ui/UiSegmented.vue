<script setup lang="ts">
import type { Component } from 'vue'
import { SegmentedControl, SegmentedControlIndicator, SegmentedControlItem } from '@ui-sdk/vue/segmented-control'

// A row of options where exactly one is on, such as All, Stations and Exercises, with an indicator
// that slides to it. The frame is drawn as an inset shadow rather than a border, so that the
// indicator UI SDK places inside it lines up with the items to the pixel.
defineProps<{ options: { value: string; label: string; icon?: Component; disabled?: boolean; title?: string }[]; label: string }>()
const model = defineModel<string>({ required: true })
</script>

<template>
  <SegmentedControl v-model="model" :aria-label="label" class="relative inline-flex h-7 items-stretch rounded-control bg-s0 p-0.5 shadow-[inset_0_0_0_1px_var(--t-control)]">
    <SegmentedControlIndicator
      class="absolute rounded-sm bg-s3 shadow-[0_0_0_1px_var(--t-line),0_1px_2px_#0000002e] transition-[left,width] duration-150"
      style="
        left: var(--segmented-control-indicator-left);
        top: var(--segmented-control-indicator-top);
        width: var(--segmented-control-indicator-width);
        height: var(--segmented-control-indicator-height);
      "
    />
    <SegmentedControlItem
      v-for="o in options"
      :key="o.value"
      :value="o.value"
      :disabled="o.disabled"
      :title="o.title"
      class="relative z-10 inline-flex h-full cursor-default items-center gap-1.5 rounded-sm border-0 bg-transparent px-2.5 py-0 text-[13px] leading-none text-muted transition-colors select-none hover:text-fg disabled:opacity-45 data-checked:font-medium data-checked:text-fg"
    >
      <component :is="o.icon" v-if="o.icon" class="size-4" />
      {{ o.label }}
    </SegmentedControlItem>
  </SegmentedControl>
</template>
