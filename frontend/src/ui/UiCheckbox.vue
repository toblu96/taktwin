<script setup lang="ts">
import { Checkbox, CheckboxIndicator } from '@ui-sdk/vue/checkbox'
import IconCheck from '~icons/fluent/checkmark-12-filled'

// A box to tick, with its label beside it, or with the label only for screen readers where the
// row around it says what it is (hideLabel), as in a table. Ticked, it is teal, the colour of
// what is chosen; the change is at once.
defineProps<{ label: string; hideLabel?: boolean; disabled?: boolean }>()
const model = defineModel<boolean>({ required: true })
</script>

<template>
  <label class="inline-flex cursor-default items-center gap-2 select-none" :class="disabled ? 'opacity-45' : ''">
    <Checkbox
      v-model="model"
      :disabled="disabled"
      :aria-label="hideLabel ? label : undefined"
      class="grid size-4 shrink-0 cursor-default place-items-center rounded-sm border border-solid border-control bg-s0 p-0 data-checked:border-focus data-checked:bg-focus"
    >
      <CheckboxIndicator><IconCheck class="size-3 text-s0" /></CheckboxIndicator>
    </Checkbox>
    <span v-if="!hideLabel">{{ label }}</span>
  </label>
</template>
