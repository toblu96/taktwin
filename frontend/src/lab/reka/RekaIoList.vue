<script setup lang="ts">
import { DragDropProvider } from '@dnd-kit/vue'
import { isSortable } from '@dnd-kit/vue/sortable'
import type { Signal } from '../sdk/SdkIoList.vue'
import RekaIoItem from './RekaIoItem.vue'

// The same I/O list for the Reka UI side, which has no drag and drop: dnd-kit's Vue adapter.
const props = defineProps<{ signals: Signal[] }>()
const order = defineModel<string[]>({ required: true })
const byKey = (key: string) => props.signals.find((s) => s.key === key)!

function dragEnd(event: { operation: { source: unknown }; canceled: boolean }) {
  const source = event.operation.source
  if (event.canceled || !isSortable(source as never)) return
  const { initialIndex, index } = source as { initialIndex: number; index: number }
  if (initialIndex === index) return
  const next = [...order.value]
  const [moved] = next.splice(initialIndex, 1)
  next.splice(index, 0, moved)
  order.value = next
}
</script>

<template>
  <DragDropProvider @drag-end="dragEnd">
    <ul class="flex flex-col gap-1" aria-label="Inputs of the PLC">
      <RekaIoItem v-for="(key, i) in order" :key="key" :signal="byKey(key)" :index="i" />
    </ul>
  </DragDropProvider>
</template>
