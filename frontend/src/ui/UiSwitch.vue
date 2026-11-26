<script setup lang="ts">
import type { Component } from 'vue'
import { Switch, SwitchThumb } from '@ui-sdk/vue/switch'

// A setting that is on or off, with its label beside it, or before it at the start of a row that
// it ends (trailing), as in a list of settings. On, it is teal, the colour of what is chosen, and
// so is its focus ring; the thumb slides, the colours change at once.
//
// Given an icon, it is a switch for a toolbar, such as the I/O test's: as tall as a button, with
// a control's corners, and the icon in its thumb says what is switched. hideLabel leaves the label
// to screen readers and the tooltip.
const props = defineProps<{ label: string; disabled?: boolean; trailing?: boolean; icon?: Component; hideLabel?: boolean; title?: string }>()
const model = defineModel<boolean>({ required: true })
</script>

<template>
  <label
    class="cursor-default items-center gap-2.5 select-none"
    :class="[disabled ? 'opacity-45' : '', trailing ? 'flex flex-row-reverse justify-between' : 'inline-flex']"
    :title="title ?? (hideLabel ? label : undefined)"
  >
    <Switch
      v-if="props.icon"
      v-model="model"
      :disabled="disabled"
      class="relative inline-flex h-7 w-12 shrink-0 cursor-default items-center rounded-control border-0 bg-s0 p-0 shadow-[inset_0_0_0_1px_var(--t-control)] data-checked:bg-focus data-checked:shadow-none"
    >
      <SwitchThumb
        class="grid size-5.5 translate-x-0.75 place-items-center rounded-[4px] bg-muted text-s0 transition-transform duration-100 data-checked:translate-x-5.75 data-checked:bg-s0 data-checked:text-focus"
      >
        <component :is="props.icon" class="size-3.5" />
      </SwitchThumb>
    </Switch>
    <Switch
      v-else
      v-model="model"
      :disabled="disabled"
      class="relative inline-flex h-5 w-9 shrink-0 cursor-default items-center rounded-full border-0 bg-s3 p-0 shadow-[inset_0_0_0_1px_var(--t-control)] data-checked:bg-focus data-checked:shadow-none"
    >
      <SwitchThumb class="block size-3.5 translate-x-0.75 rounded-full bg-muted transition-transform duration-100 data-checked:translate-x-4.75 data-checked:bg-s0" />
    </Switch>
    <span :class="{ 'sr-only': hideLabel }">{{ label }}</span>
  </label>
</template>
