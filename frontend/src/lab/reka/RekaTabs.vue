<script setup lang="ts">
import { TabsContent, TabsIndicator, TabsList, TabsRoot, TabsTrigger } from 'reka-ui'

// The same connected tabs on Reka UI.
defineProps<{ tabs: { value: string; label: string; count?: number }[]; label: string }>()
const model = defineModel<string>({ required: true })
</script>

<template>
  <TabsRoot v-model="model" class="flex min-h-0 flex-col">
    <TabsList :aria-label="label" class="relative flex h-9 items-end gap-0.5 border-b border-line px-2">
      <TabsTrigger
        v-for="t in tabs"
        :key="t.value"
        :value="t.value"
        class="relative -mb-px flex h-8 items-center gap-1.5 rounded-t-control border border-transparent px-3 text-muted select-none hover:text-fg data-[state=active]:border-line data-[state=active]:border-b-s0 data-[state=active]:bg-s0 data-[state=active]:text-fg"
      >
        {{ t.label }}
        <span v-if="t.count" class="rounded-full bg-s3 px-1.5 text-[11px] leading-4 text-muted">{{ t.count }}</span>
      </TabsTrigger>
      <TabsIndicator
        class="pointer-events-none absolute top-0 left-0 h-0.5 w-(--reka-tabs-indicator-size) translate-x-(--reka-tabs-indicator-position) rounded-full transition-[width,translate] duration-150"
      >
        <div class="h-full w-full rounded-full bg-accent" />
      </TabsIndicator>
    </TabsList>
    <TabsContent v-for="t in tabs" :key="t.value" :value="t.value" class="min-h-0 flex-1 p-3 focus-visible:outline-none">
      <slot :name="t.value" />
    </TabsContent>
  </TabsRoot>
</template>
