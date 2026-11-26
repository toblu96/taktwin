<script setup lang="ts">
import type { Component } from 'vue'
import { Switch, SwitchThumb } from '@ui-sdk/vue/switch'

// A setting that is on or off, with its label beside it, or before it at the start of a row that
// it ends (trailing), as in a list of settings. On, it is teal, the colour of what is chosen, and
// so is its focus ring; the thumb slides, the colours change at once. Given an icon, the switch is
// a size larger and the thumb carries it, to say what is switched, as the I/O test's does.
defineProps<{ label: string; disabled?: boolean; trailing?: boolean; icon?: Component }>()
const model = defineModel<boolean>({ required: true })
</script>

<template>
  <label class="cursor-default items-center gap-2.5 select-none" :class="[disabled ? 'opacity-45' : '', trailing ? 'flex flex-row-reverse justify-between' : 'inline-flex']">
    <Switch
      v-model="model"
      :disabled="disabled"
      class="relative inline-flex shrink-0 cursor-default items-center rounded-full border-0 bg-s3 p-0 shadow-[inset_0_0_0_1px_var(--t-control)] data-checked:bg-focus data-checked:shadow-none"
      :class="icon ? 'h-6 w-11' : 'h-5 w-9'"
    >
      <SwitchThumb
        v-if="icon"
        class="grid size-4.5 translate-x-0.75 place-items-center rounded-full bg-muted text-s1 transition-transform duration-100 data-checked:translate-x-5.75 data-checked:bg-s0 data-checked:text-focus"
      >
        <component :is="icon" class="size-3" />
      </SwitchThumb>
      <SwitchThumb v-else class="block size-3.5 translate-x-0.75 rounded-full bg-muted transition-transform duration-100 data-checked:translate-x-4.75 data-checked:bg-s0" />
    </Switch>
    <span>{{ label }}</span>
  </label>
</template>
