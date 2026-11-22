<script setup lang="ts">
import { Tabs, TabsList, TabsPanel, TabsTab } from '@ui-sdk/vue/tabs'

// Tabs as VS Code's modern UI draws them: they sit on the chrome, and the active one, with
// rounded top corners and an amber line along its top that follows them, joins the content
// below. Each tab's content goes into the slot of its value; the slot actions sits at the end of
// the row, outside the list of tabs, for buttons such as closing a panel.
defineProps<{ tabs: { value: string; label: string; count?: number }[]; label: string }>()
const model = defineModel<string>({ required: true })
</script>

<template>
  <Tabs v-model:value="model" class="flex min-h-0 flex-col">
    <div class="flex h-9 shrink-0 items-end border-b border-line bg-s1 px-2">
      <TabsList :aria-label="label" class="flex h-full min-w-0 flex-1 items-end gap-0.5">
        <TabsTab
          v-for="t in tabs"
          :key="t.value"
          :value="t.value"
          class="relative -mb-px flex h-8 cursor-default items-center gap-1.5 overflow-hidden rounded-t-[7px] border border-b-0 border-transparent bg-transparent px-3 text-muted select-none hover:bg-hover hover:text-fg data-selected:border-line data-selected:bg-s0 data-selected:text-fg data-selected:before:absolute data-selected:before:inset-x-0 data-selected:before:top-0 data-selected:before:h-0.5 data-selected:before:bg-accent"
        >
          {{ t.label }}
          <span v-if="t.count" class="rounded-full bg-s3 px-1.5 text-[11px] leading-4 text-muted">{{ t.count }}</span>
        </TabsTab>
      </TabsList>
      <div v-if="$slots.actions" class="flex h-full items-center gap-0.5 pl-2"><slot name="actions" /></div>
    </div>
    <TabsPanel v-for="t in tabs" :key="t.value" :value="t.value" class="min-h-0 flex-1 overflow-auto bg-s0 p-3 focus-visible:outline-offset-[-2px]">
      <slot :name="t.value" />
    </TabsPanel>
  </Tabs>
</template>
