<script setup lang="ts">
import { computed } from 'vue'
import { useSim } from '../sim'
import ConnectorsSection from './ConnectorsSection.vue'
import PlcSection from './PlcSection.vue'
import WidgetList from './WidgetList.vue'
import IoPanel from './IoPanel.vue'
import IconDelete from '~icons/fluent/delete-16-regular'

// The side bar on the right of a running station: the station's inputs and outputs, or what was
// clicked in the 3D view or in the side bar on the left: a workpiece, to take it off the station,
// or a module, with what can be done with it while it runs: its controls, and its I/O connectors
// with their live pins, or for a PLC how it is connected and its program. Clicking beside it in
// the 3D view goes back to the inputs and outputs; how the station is built and wired is for the
// other views.
const { pickedWorkpiece, scene, selected, takeWorkpiece } = useSim()
const kindName = (kind: string) => scene.value?.workpieces.kinds?.find((k) => k.id === kind)?.label ?? kind
const panelWidgets = computed(() => (selected.value?.controls ?? []).filter((w) => w.show === 'panel'))
</script>

<template>
  <aside class="flex h-full min-h-0 flex-col bg-s1" aria-label="Selected">
    <template v-if="pickedWorkpiece">
      <header class="flex h-11 shrink-0 items-center border-b border-line px-3">
        <h2 class="m-0 truncate text-[15px] font-semibold">{{ kindName(pickedWorkpiece.kind) }}</h2>
      </header>
      <section class="t-section flex flex-col gap-2">
        <p v-if="pickedWorkpiece.loads?.length" class="m-0 text-muted">
          With {{ pickedWorkpiece.loads.length === 1 ? kindName(pickedWorkpiece.loads[0].kind).toLowerCase() : `${pickedWorkpiece.loads.length} workpieces` }} on it, which go with
          it.
        </p>
        <button class="t-btn self-start" @click="takeWorkpiece"><IconDelete class="size-4" />Take it off the station</button>
        <p class="m-0 text-xs text-muted">Delete or Backspace does the same. Clicking beside it lets go of it.</p>
      </section>
    </template>
    <template v-else-if="selected">
      <header class="flex h-11 shrink-0 items-center border-b border-line px-3">
        <h2 class="m-0 truncate text-[15px] font-semibold" :title="selected.description">{{ selected.name }}</h2>
      </header>
      <div class="min-h-0 flex-1 overflow-y-auto">
        <section v-if="panelWidgets.length" class="t-section">
          <h3 class="t-section-title">Controls</h3>
          <WidgetList :widgets="panelWidgets" />
        </section>
        <PlcSection v-if="selected.isPlc" :signals="false" />
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
