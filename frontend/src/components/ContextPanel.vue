<script setup lang="ts">
import { computed } from 'vue'
import { useSim } from '../sim'
import ConnectorsSection from './ConnectorsSection.vue'
import PlcSection from './PlcSection.vue'
import WidgetList from './WidgetList.vue'
import IconUp from '~icons/fluent/chevron-left-16-regular'
import IconCursor from '~icons/fluent/cursor-click-20-regular'

// The side bar on the right of a running station: the module clicked in the 3D view, with what
// can be done with it while it runs: its controls, its pins with their live signals and the
// I/O test, or a PLC's program and connection. How it is built is for editing.
const { scene, selected, selection } = useSim()

const parent = computed(() => (scene.value?.instances ?? []).find((i) => i.id === selected.value?.parent) ?? null)
const panelWidgets = computed(() => (selected.value?.controls ?? []).filter((w) => w.show === 'panel'))
</script>

<template>
  <aside class="flex h-full min-h-0 flex-col bg-s1" aria-label="Selected">
    <template v-if="selected">
      <header class="shrink-0 border-b border-line px-3 pt-2.5 pb-3">
        <button
          class="mb-1 -ml-1 flex cursor-default items-center gap-0.5 rounded-sm border-0 bg-transparent px-1 py-0 text-xs text-muted hover:text-fg"
          @click="selection = parent?.id ?? null"
        >
          <IconUp class="size-3.5" />{{ parent?.name ?? scene?.name }}
        </button>
        <h2 class="m-0 text-[15px] leading-snug font-semibold">{{ selected.name }}</h2>
        <p v-if="selected.description" class="m-0 mt-1.5 text-muted">{{ selected.description }}</p>
      </header>
      <div class="min-h-0 flex-1 overflow-y-auto">
        <section v-if="panelWidgets.length" class="t-section">
          <h3 class="t-section-title">Controls</h3>
          <WidgetList :widgets="panelWidgets" />
        </section>
        <PlcSection v-if="selected.isPlc" :signals="false" />
        <ConnectorsSection v-else :connectors="selected.connectors ?? []" />
      </div>
    </template>
    <div v-else class="flex flex-1 flex-col items-center justify-center gap-2 p-6 text-center text-muted">
      <IconCursor class="size-6 text-faint" />
      <p class="m-0 max-w-56">Click a module in the 3D view to see its controls and signals, or a PLC on the left for its connection.</p>
    </div>
  </aside>
</template>
