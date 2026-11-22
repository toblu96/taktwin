<script setup lang="ts">
import { Tabs, TabsIndicator, TabsList, TabsPanel, TabsTab } from '@ui-sdk/vue/tabs'

// Tabs whose active one joins the panel below, as VS Code's modern UI draws them, with an amber
// line on it. Each tab's content goes into the slot of its value.
defineProps<{ tabs: { value: string; label: string; count?: number }[]; label: string }>()
const model = defineModel<string>({ required: true })
</script>

<template>
  <Tabs v-model:value="model" class="flex min-h-0 flex-col">
    <TabsList :aria-label="label" class="relative flex h-9 items-end gap-0.5 border-b border-line px-2">
      <TabsTab
        v-for="t in tabs"
        :key="t.value"
        :value="t.value"
        class="relative -mb-px flex h-8 items-center gap-1.5 rounded-t-control border border-transparent px-3 text-muted select-none hover:text-fg data-selected:border-line data-selected:border-b-s0 data-selected:bg-s0 data-selected:text-fg"
      >
        {{ t.label }}
        <span v-if="t.count" class="rounded-full bg-s3 px-1.5 text-[11px] leading-4 text-muted">{{ t.count }}</span>
      </TabsTab>
      <TabsIndicator
        class="pointer-events-none absolute h-0.5 rounded-full bg-accent transition-[left,width] duration-150"
        style="left: var(--active-tab-left); width: var(--active-tab-width); top: calc(var(--active-tab-top) + 1px)"
      />
    </TabsList>
    <TabsPanel v-for="t in tabs" :key="t.value" :value="t.value" class="min-h-0 flex-1 p-3 focus-visible:outline-none">
      <slot :name="t.value" />
    </TabsPanel>
  </Tabs>
</template>
