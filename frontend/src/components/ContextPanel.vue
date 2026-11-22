<script setup lang="ts">
import { computed } from 'vue'
import { useSim } from '../sim'
import ConnectorsSection from './ConnectorsSection.vue'
import PlcSection from './PlcSection.vue'
import WidgetList from './WidgetList.vue'
import IoPanel from './IoPanel.vue'

// The side bar on the right of a running station: the station's inputs and outputs with the I/O
// test, or the module clicked in the 3D view, with what can be done with it while it runs: its
// controls, and its I/O connectors with their live pins, or a PLC's program. Clicking beside it
// in the 3D view goes back to the inputs and outputs; how it is built and how a PLC is connected
// are for the other views.
const { selected } = useSim()
const panelWidgets = computed(() => (selected.value?.controls ?? []).filter((w) => w.show === 'panel'))
</script>

<template>
  <aside class="flex h-full min-h-0 flex-col bg-s1" aria-label="Selected">
    <template v-if="selected">
      <header class="flex h-11 shrink-0 items-center border-b border-line px-3">
        <h2 class="m-0 truncate text-[15px] font-semibold" :title="selected.description">{{ selected.name }}</h2>
      </header>
      <div class="min-h-0 flex-1 overflow-y-auto">
        <section v-if="panelWidgets.length" class="t-section">
          <h3 class="t-section-title">Controls</h3>
          <WidgetList :widgets="panelWidgets" />
        </section>
        <PlcSection v-if="selected.isPlc" :signals="false" :connection="false" />
        <ConnectorsSection v-else :connectors="(selected.connectors ?? []).filter((c) => c.type !== 'wires')" />
      </div>
    </template>
    <template v-else>
      <header class="flex h-11 shrink-0 items-center border-b border-line px-3">
        <h2 class="m-0 truncate text-[15px] font-semibold">Inputs and outputs</h2>
      </header>
      <IoPanel class="min-h-0 flex-1 overflow-y-auto" />
    </template>
  </aside>
</template>
