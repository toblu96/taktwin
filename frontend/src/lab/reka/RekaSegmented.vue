<script setup lang="ts">
import type { Component } from 'vue'
import { ToggleGroupItem, ToggleGroupRoot } from 'reka-ui'

// The same segmented control on Reka UI's toggle group. Reka has no sliding indicator, so the
// checked item draws its own background.
defineProps<{ options: { value: string; label: string; icon?: Component }[]; label: string }>()
const model = defineModel<string>({ required: true })
</script>

<template>
  <ToggleGroupRoot
    :model-value="model"
    type="single"
    :aria-label="label"
    class="inline-flex h-7 items-stretch rounded-control border border-line bg-s1 p-0.5"
    @update:model-value="(v) => v && (model = v as string)"
  >
    <ToggleGroupItem
      v-for="o in options"
      :key="o.value"
      :value="o.value"
      class="inline-flex items-center gap-1.5 rounded-[5px] px-2.5 text-[13px] text-muted transition-colors select-none hover:text-fg data-[state=on]:bg-s3 data-[state=on]:text-fg data-[state=on]:shadow-[0_1px_2px_#0003]"
    >
      <component :is="o.icon" v-if="o.icon" class="size-4" />
      {{ o.label }}
    </ToggleGroupItem>
  </ToggleGroupRoot>
</template>
