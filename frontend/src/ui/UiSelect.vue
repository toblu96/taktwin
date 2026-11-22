<script setup lang="ts">
import { Select, SelectContent, SelectItem, SelectItemIndicator, SelectItemText, SelectLabel, SelectTrigger, SelectValue } from '@ui-sdk/vue/select'
import IconChevron from '~icons/fluent/chevron-down-16-regular'
import IconCheck from '~icons/fluent/checkmark-16-regular'

// A choice of one from a list, such as the station of a new project, with UI SDK's select.
defineProps<{ options: { value: string; label: string; hint?: string }[]; label: string; placeholder?: string }>()
const model = defineModel<string | null>({ required: true })
</script>

<template>
  <div class="flex flex-col gap-1">
    <Select :value="model" @value-change="(v: string | null) => (model = v)">
      <SelectLabel class="t-label">{{ label }}</SelectLabel>
      <SelectTrigger class="t-input flex items-center justify-between gap-2 text-left">
        <SelectValue :placeholder="placeholder" class="truncate data-placeholder:text-faint" />
        <IconChevron class="size-4 shrink-0 text-muted" />
      </SelectTrigger>
      <SelectContent class="t-popup max-h-72 min-w-(--anchor-width) overflow-auto">
        <SelectItem v-for="o in options" :key="o.value" :value="o.value" class="t-item group h-auto py-1.5">
          <span class="flex flex-1 flex-col">
            <SelectItemText>{{ o.label }}</SelectItemText>
            <span v-if="o.hint" class="text-xs text-muted group-data-highlighted:text-fg/80">{{ o.hint }}</span>
          </span>
          <SelectItemIndicator><IconCheck class="size-4 text-focus" /></SelectItemIndicator>
        </SelectItem>
      </SelectContent>
    </Select>
  </div>
</template>
