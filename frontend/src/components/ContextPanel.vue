<script setup lang="ts">
import { computed } from 'vue'
import { useSim } from '../sim'
import ConnectorsSection from './ConnectorsSection.vue'
import PlcSection from './PlcSection.vue'
import SettingsSection from './SettingsSection.vue'
import StationDock from './StationDock.vue'
import WidgetList from './WidgetList.vue'
import IconUp from '~icons/fluent/chevron-left-16-regular'

// The side bar on the right of an open project: what is selected, in the tree or the 3D view.
// A module shows its controls, its connections or its PLC, and its settings and faults; with
// nothing selected, the station shows what it is and the settings of all its modules.
const { scene, selected, selection } = useSim()

const parent = computed(() => (scene.value?.instances ?? []).find((i) => i.id === selected.value?.parent) ?? null)
const panelWidgets = computed(() => (selected.value?.controls ?? []).filter((w) => w.show === 'panel'))
const allFaults = computed(() => (scene.value?.instances ?? []).flatMap((i) => i.faults ?? []))
const parameters = computed(() => {
  const all = scene.value?.parameters ?? []
  return selected.value ? all.filter((p) => p.module === selected.value?.id) : all
})
</script>

<template>
  <aside class="flex h-full min-h-0 flex-col bg-s1" aria-label="Selected">
    <StationDock place="side" />
    <header class="shrink-0 border-b border-line px-3 pt-2.5 pb-3">
      <button
        v-if="selected"
        class="mb-1 -ml-1 flex cursor-default items-center gap-0.5 rounded-sm border-0 bg-transparent px-1 py-0 text-xs text-muted hover:text-fg"
        @click="selection = parent?.id ?? null"
      >
        <IconUp class="size-3.5" />{{ parent?.name ?? scene?.name }}
      </button>
      <h2 class="m-0 text-[15px] leading-snug font-semibold">{{ selected?.name ?? scene?.name }}</h2>
      <p v-if="selected" class="m-0 mt-0.5 font-mono text-[11px] text-muted">
        {{ selected.level === 'module' ? 'Module' : 'Accessory' }} · {{ selected.module }} · v{{ selected.version }}
      </p>
      <p v-if="(selected ?? scene)?.description" class="m-0 mt-1.5 text-muted">{{ (selected ?? scene)?.description }}</p>
    </header>
    <div class="context-body min-h-0 flex-1 overflow-y-auto">
      <template v-if="selected">
        <section v-if="panelWidgets.length" class="section">
          <h3 class="section-title">Controls</h3>
          <WidgetList :widgets="panelWidgets" />
        </section>
        <PlcSection v-if="selected.isPlc" :signals="false" />
        <ConnectorsSection v-else :connectors="selected.connectors ?? []" />
        <SettingsSection :parameters="parameters" :faults="selected.faults ?? []" :values="selected.values ?? []" />
      </template>
      <SettingsSection v-else-if="scene" :parameters="parameters" :faults="allFaults" :values="[]" named />
    </div>
  </aside>
</template>

<style scoped>
.context-body :deep(.section) {
  border-bottom: 1px solid var(--t-line);
  padding: 12px;
}

.context-body :deep(.section:last-child) {
  border-bottom: 0;
}
</style>
