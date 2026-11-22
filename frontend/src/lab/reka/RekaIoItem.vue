<script setup lang="ts">
import { ref } from 'vue'
import { useSortable } from '@dnd-kit/vue/sortable'
import type { Signal } from '../sdk/SdkIoList.vue'
import IconGrip from '~icons/fluent/re-order-dots-vertical-20-regular'

// One signal of the I/O list, sortable with dnd-kit: by its handle with the pointer, or with
// the keyboard from the handle.
const props = defineProps<{ signal: Signal; index: number }>()
const element = ref<HTMLElement>()
const handle = ref<HTMLElement>()
const { isDragging } = useSortable({ id: () => props.signal.key, index: () => props.index, element, handle })
</script>

<template>
  <li
    ref="element"
    class="group flex h-9 items-center gap-2 rounded-control border border-line bg-s1 pr-2 pl-1 select-none"
    :class="isDragging ? 'z-10 border-focus bg-select shadow-pop' : ''"
  >
    <button ref="handle" class="cursor-grab rounded p-0.5 text-faint group-hover:text-muted" :aria-label="`Move ${signal.name}`">
      <IconGrip class="size-4" />
    </button>
    <span class="w-16 font-mono text-xs text-muted">%IX0.{{ index }}</span>
    <span class="w-28 font-medium">{{ signal.name }}</span>
    <span class="truncate text-muted">{{ signal.description }}</span>
  </li>
</template>
