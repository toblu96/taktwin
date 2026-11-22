<script setup lang="ts">
import { Label, SelectContent, SelectItem, SelectItemIndicator, SelectItemText, SelectPortal, SelectRoot, SelectTrigger, SelectValue, SelectViewport } from 'reka-ui'
import IconChevron from '~icons/fluent/chevron-down-16-regular'
import IconCheck from '~icons/fluent/checkmark-16-regular'

// The same select on Reka UI.
const props = defineProps<{ options: { value: string; label: string; hint?: string }[]; label: string; placeholder?: string }>()
const model = defineModel<string | null>({ required: true })
const id = `select-${Math.random().toString(36).slice(2, 8)}`
void props
</script>

<template>
  <div class="flex flex-col gap-1">
    <Label :for="id" class="t-label">{{ label }}</Label>
    <SelectRoot :model-value="model ?? undefined" @update:model-value="(v) => (model = v as string)">
      <SelectTrigger :id="id" class="t-input flex items-center justify-between gap-2 text-left">
        <SelectValue :placeholder="placeholder" class="truncate data-placeholder:text-faint" />
        <IconChevron class="size-4 shrink-0 text-muted" />
      </SelectTrigger>
      <SelectPortal>
        <SelectContent position="popper" :side-offset="4" class="t-popup max-h-72 w-(--reka-select-trigger-width)">
          <SelectViewport>
            <SelectItem v-for="o in options" :key="o.value" :value="o.value" class="t-item h-auto py-1.5 data-highlighted:bg-select">
              <span class="flex flex-1 flex-col">
                <SelectItemText>{{ o.label }}</SelectItemText>
                <span v-if="o.hint" class="text-xs text-muted">{{ o.hint }}</span>
              </span>
              <SelectItemIndicator><IconCheck class="size-4 text-focus" /></SelectItemIndicator>
            </SelectItem>
          </SelectViewport>
        </SelectContent>
      </SelectPortal>
    </SelectRoot>
  </div>
</template>
