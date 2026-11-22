<script setup lang="ts">
import { createParts } from '@ui-sdk/vue/primitive'
import { ioList } from './io-list'
import IconGrip from '~icons/fluent/re-order-dots-vertical-20-regular'

// The PLC's inputs in the order of its I/O list, which people rearrange to match a program
// written for another wiring: dragged, or moved with Space, the arrows and Space. Built from UI
// SDK's reorder behaviour in a component of our own (io-list.ts).
export type Signal = { key: string; address: string; name: string; description: string; locked?: boolean }
const props = defineProps<{ signals: Signal[] }>()
const order = defineModel<string[]>({ required: true })
const { Root: IoListRoot, Item: IoListItem } = createParts(ioList, { root: 'ul', item: 'li' })
const byKey = (key: string) => props.signals.find((s) => s.key === key)!
</script>

<template>
  <IoListRoot v-slot="{ state }" v-model:value="order" class="m-0 flex list-none flex-col gap-1 p-0" aria-label="Inputs of the PLC">
    <IoListItem
      v-for="(key, i) in state.order"
      :key="key"
      :value="key"
      class="group flex h-9 items-center gap-2 rounded-control border border-line bg-s1 pr-2 pl-1 select-none data-dragging:z-10 data-dragging:translate-x-(--drag-x) data-dragging:translate-y-(--drag-y) data-dragging:shadow-pop data-moving:border-focus data-moving:bg-select"
    >
      <IconGrip class="size-4 cursor-grab text-faint group-hover:text-muted" />
      <span class="w-16 font-mono text-xs text-muted">%IX0.{{ i }}</span>
      <span class="w-28 font-medium">{{ byKey(key).name }}</span>
      <span class="truncate text-muted">{{ byKey(key).description }}</span>
    </IoListItem>
  </IoListRoot>
</template>
