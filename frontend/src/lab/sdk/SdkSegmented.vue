<script setup lang="ts">
import type { Component } from 'vue'
import { SegmentedControl, SegmentedControlIndicator, SegmentedControlItem } from '@ui-sdk/vue/segmented-control'

// A row of options where exactly one is on, such as Simulate and Build, with an indicator that
// slides to it.
defineProps<{ options: { value: string; label: string; icon?: Component }[]; label: string }>()
const model = defineModel<string>({ required: true })
</script>

<template>
  <SegmentedControl v-model="model" :aria-label="label" class="relative inline-flex h-7 items-stretch rounded-control border border-line bg-s1 p-0.5">
    <SegmentedControlIndicator
      class="absolute rounded-[5px] bg-s3 shadow-[0_1px_2px_#0003] transition-[left,width] duration-150"
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
      class="relative z-10 inline-flex items-center gap-1.5 rounded-[5px] px-2.5 text-[13px] text-muted transition-colors select-none hover:text-fg data-checked:text-fg"
    >
      <component :is="o.icon" v-if="o.icon" class="size-4" />
      {{ o.label }}
    </SegmentedControlItem>
  </SegmentedControl>
</template>
