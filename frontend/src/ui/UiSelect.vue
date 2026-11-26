<script setup lang="ts">
import { Select, SelectContent, SelectItem, SelectItemIndicator, SelectItemText, SelectLabel, SelectTrigger, SelectValue } from '@ui-sdk/vue/select'
import IconChevron from '~icons/fluent/chevron-down-16-regular'
import IconCheck from '~icons/fluent/checkmark-16-regular'

// A choice of one from a list, such as the station of a new project, with UI SDK's select.
// With hideLabel the label is only for screen readers, where the row around it says what it is.
// An option's hint says where it comes from, its description what it is, both in the list only.
defineProps<{ options: { value: string; label: string; hint?: string; description?: string }[]; label: string; placeholder?: string; hideLabel?: boolean }>()
const model = defineModel<string | null>({ required: true })
</script>

<template>
  <div class="flex flex-col gap-1">
    <Select :value="model" @value-change="(v: string | null) => (model = v)">
      <SelectLabel class="t-label" :class="hideLabel ? 'sr-only' : ''">{{ label }}</SelectLabel>
      <SelectTrigger class="t-input flex items-center justify-between gap-2 text-left">
        <SelectValue :placeholder="placeholder" class="truncate data-placeholder:text-faint" />
        <IconChevron class="size-4 shrink-0 text-muted" />
      </SelectTrigger>
      <SelectContent class="t-popup max-h-72 min-w-(--anchor-width) overflow-auto">
        <SelectItem v-for="o in options" :key="o.value" :value="o.value" :label="o.label" class="t-item group h-auto py-1.5">
          <span class="flex flex-1 flex-col">
            <SelectItemText>{{ o.label }}</SelectItemText>
            <span v-if="o.hint" class="text-xs text-muted group-data-highlighted:text-fg/80">{{ o.hint }}</span>
            <span v-if="o.description" class="max-w-80 text-xs whitespace-normal text-muted group-data-highlighted:text-fg/80">{{ o.description }}</span>
          </span>
          <SelectItemIndicator><IconCheck class="size-4 text-focus" /></SelectItemIndicator>
        </SelectItem>
      </SelectContent>
    </Select>
  </div>
</template>
