<script setup lang="ts">
import { computed } from 'vue'
import type { WiringState } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import UiTree, { type TreeHeading, type TreeRow } from '../ui/UiTree.vue'
import IconPlc from '~icons/fluent/developer-board-16-regular'
import IconInterface from '~icons/fluent/connector-16-regular'
import IconTerminal from '~icons/fluent/plug-connected-16-regular'
import IconConsole from '~icons/fluent/keyboard-16-regular'

// The side bar of the wiring: its boxes by stand, the PLC and the console first, then the
// interfaces and the I/O terminals, to jump to one.
const props = defineProps<{ wiring: WiringState | null; selected: string | null }>()
const emit = defineEmits<{ select: [id: string] }>()

const ORDER = ['plc', 'console', 'interface', 'terminal']
const ICONS = { plc: IconPlc, console: IconConsole, interface: IconInterface, terminal: IconTerminal } as const

const items = computed<(TreeRow | TreeHeading)[]>(() => {
  const stations = props.wiring?.stations ?? []
  return stations.flatMap((s) => [
    ...(stations.length > 1 || s.name ? [{ key: `h:${s.id}`, heading: s.name } as TreeHeading] : []),
    ...[...(s.boxes ?? [])]
      .sort((a, b) => ORDER.indexOf(a.kind) - ORDER.indexOf(b.kind))
      .map((b): TreeRow => ({
        key: b.id,
        label: b.of ? `${b.of} · ${b.name}` : b.name,
        icon: ICONS[b.kind as keyof typeof ICONS],
        selected: props.selected === b.id,
        title: b.of ? `${b.name} of ${b.of}` : b.name,
      })),
  ])
})
</script>

<template>
  <aside class="flex h-full min-w-0 flex-col bg-s1" aria-label="Wiring">
    <h2 class="m-0 flex h-9 shrink-0 items-center px-3 text-xs font-semibold tracking-wide text-muted uppercase">Wiring</h2>
    <UiTree :items="items" label="PLCs and I/O" class="min-h-0 flex-1 overflow-auto px-1.5 pb-2" @select="(id) => emit('select', id)" />
  </aside>
</template>
