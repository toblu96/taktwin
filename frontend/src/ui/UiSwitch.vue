<script setup lang="ts">
import { Switch, SwitchThumb } from '@ui-sdk/vue/switch'

// A setting that is on or off, with its label beside it. On, it is teal, the colour of what is
// chosen, and so is its focus ring; the thumb slides, the colours change at once.
defineProps<{ label: string; disabled?: boolean }>()
const model = defineModel<boolean>({ required: true })
</script>

<template>
  <label class="inline-flex cursor-default items-center gap-2.5 select-none" :class="disabled ? 'opacity-45' : ''">
    <Switch
      v-model="model"
      :disabled="disabled"
      class="relative inline-flex h-5 w-9 shrink-0 cursor-default items-center rounded-full border-0 bg-s3 p-0 shadow-[inset_0_0_0_1px_var(--t-control)] data-checked:bg-focus data-checked:shadow-none"
    >
      <SwitchThumb class="block size-3.5 translate-x-[3px] rounded-full bg-muted transition-transform duration-100 data-checked:translate-x-[19px] data-checked:bg-s0" />
    </Switch>
    <span>{{ label }}</span>
  </label>
</template>
