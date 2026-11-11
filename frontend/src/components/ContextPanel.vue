<script setup lang="ts">
import { computed } from 'vue'
import type { SceneCheck } from '../../bindings/github.com/toblu96/taktwin/internal/model'
import { useSim } from '../sim'
import ConnectorsSection from './ConnectorsSection.vue'
import ExerciseSection from './ExerciseSection.vue'
import PlcSection from './PlcSection.vue'
import SettingsSection from './SettingsSection.vue'
import StationDock from './StationDock.vue'
import WidgetList from './WidgetList.vue'

const { scene, selected, selection, highlight } = useSim()

// Shows what a check is about in the 3D view, or nothing on a second click.
function show(check: SceneCheck) {
  const instances = check.instances ?? []
  highlight.value = highlight.value?.instances.join() === instances.join() ? null : { instances, at: check.at ?? null }
}

// Guide rails and the like belong to another module, so they are not listed.
const modules = computed(() => (scene.value?.instances ?? []).filter((i) => !i.selectAs))

// The modules with what they contain, then the station's accessories.
const groups = computed(() => {
  const inside = (id: string) => modules.value.filter((i) => i.parent === id)
  const top = modules.value.filter((i) => !i.parent)
  return [
    { title: 'Modules', modules: top.filter((i) => i.level === 'module').flatMap((i) => [i, ...inside(i.id)]) },
    { title: 'Accessories', modules: top.filter((i) => i.level !== 'module') },
  ].filter((g) => g.modules.length)
})

const contained = computed(() => (scene.value?.instances ?? []).filter((i) => i.parent === selected.value?.id && !i.selectAs))
const parent = computed(() => (scene.value?.instances ?? []).find((i) => i.id === selected.value?.parent) ?? null)

const panelWidgets = computed(() => (selected.value?.controls ?? []).filter((w) => w.show === 'panel'))

const allFaults = computed(() => (scene.value?.instances ?? []).flatMap((i) => i.faults ?? []))
const parameters = computed(() => {
  const all = scene.value?.parameters ?? []
  return selected.value ? all.filter((p) => p.module === selected.value?.id) : all
})
</script>

<template>
  <aside class="context">
    <StationDock place="side" />
    <template v-if="selected">
      <header class="head">
        <button class="back" @click="selection = parent?.id ?? null">‹ {{ parent?.name ?? scene?.name }}</button>
        <h2>{{ selected.name }}</h2>
        <p class="kind mono">{{ selected.level === 'module' ? 'Module' : 'Accessory' }} · {{ selected.module }} · v{{ selected.version }}</p>
        <p class="description">{{ selected.description }}</p>
      </header>
      <div class="body">
        <section v-if="contained.length" class="section">
          <h3 class="section-title">Contains</h3>
          <button v-for="module in contained" :key="module.id" class="module" @click="selection = module.id">
            <span>{{ module.name }}</span>
            <small class="mono">{{ module.module }}</small>
          </button>
        </section>
        <section v-if="panelWidgets.length" class="section">
          <h3 class="section-title">Controls</h3>
          <WidgetList :widgets="panelWidgets" />
        </section>
        <PlcSection v-if="selected.isPlc" />
        <ConnectorsSection v-else :connectors="selected.connectors ?? []" />
        <SettingsSection :parameters="parameters" :faults="selected.faults ?? []" :values="selected.values ?? []" />
      </div>
    </template>

    <template v-else-if="scene">
      <header class="head">
        <h2>{{ scene.name }}</h2>
        <p class="description">{{ scene.description }}</p>
      </header>
      <div class="body">
        <section v-if="scene.checks?.length" class="section">
          <h3 class="section-title">Station checks</h3>
          <button v-for="(check, index) in scene.checks" :key="index" class="check" title="Show it in the 3D view" @click="show(check)">
            {{ check.text }}
          </button>
        </section>
        <ExerciseSection />
        <section v-for="group in groups" :key="group.title" class="section">
          <h3 class="section-title">{{ group.title }}</h3>
          <button v-for="module in group.modules" :key="module.id" class="module" :class="{ nested: module.parent }" @click="selection = module.id">
            <span>{{ module.name }}</span>
            <small class="mono">{{ module.module }}</small>
          </button>
        </section>
        <SettingsSection :parameters="parameters" :faults="allFaults" :values="[]" named />
      </div>
    </template>
  </aside>
</template>

<style scoped>
.context {
  background: var(--panel);
  border-left: 1px solid var(--line);
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.head {
  border-bottom: 1px solid var(--line);
  padding: 12px;
}

h2 {
  font-size: 15px;
  margin: 0;
}

.kind {
  color: var(--text-dim);
  font-size: 11px;
  margin: 2px 0 0;
}

.description {
  color: var(--text-dim);
  margin: 6px 0 0;
}

.back {
  background: none;
  border: 0;
  color: var(--text-dim);
  margin: 0 0 6px -2px;
  padding: 0 2px;
}

.back:hover {
  color: var(--text);
}

.body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.body :deep(.section) {
  border-bottom: 1px solid var(--line);
  padding: 12px;
}

.body :deep(.section:last-child) {
  border-bottom: 0;
}

.module {
  align-items: baseline;
  background: none;
  border: 0;
  border-bottom: 1px solid #1a3a3f;
  border-radius: 0;
  display: flex;
  gap: 8px;
  justify-content: space-between;
  padding: 5px 2px;
  text-align: left;
  width: 100%;
}

.check {
  background: none;
  border: 0;
  border-left: 2px solid var(--danger);
  border-radius: 0;
  display: block;
  text-align: left;
  width: 100%;
  font-size: 13px;
  line-height: 1.4;
  margin: 0 0 6px;
  padding: 2px 0 2px 8px;
}

.module.nested {
  padding-left: 14px;
}

.module:hover {
  background: var(--panel-raised);
}

.module small {
  color: var(--text-dim);
  font-size: 11px;
}
</style>
