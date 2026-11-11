<script setup lang="ts">
import { computed } from 'vue'
import type { EntryRef, SceneTest } from '../../bindings/github.com/toblu96/taktwin/internal/model'
import { collectionName } from '../collections'
import { current, entries, preview, reset, rootInstance } from '../library'
import { loadStation, page, simulation } from '../sim'
import ConnectorsSection from './ConnectorsSection.vue'
import PlcSection from './PlcSection.vue'
import SettingsSection from './SettingsSection.vue'
import WidgetList from './WidgetList.vue'

defineEmits<{ open: [entry: EntryRef] }>()

const { actionMessage, runAction, scene, selected, selection } = preview

const LEVELS: { [level: string]: string } = { station: 'Station', module: 'Module', accessory: 'Accessory', part: 'Part' }

const entry = computed(() => entries.value.find((e) => e.id === current.value?.id && e.level === current.value?.level) ?? null)

// What the panel shows: a part clicked in the preview, or else the entry itself.
const shown = computed(() => selected.value ?? rootInstance.value)
const name = computed(() => shown.value?.name ?? entry.value?.name ?? '')
const description = computed(() => shown.value?.description || entry.value?.description || '')
const level = computed(() => shown.value?.level ?? current.value?.level ?? '')
const group = computed(() => {
  const id = selected.value?.module ?? current.value?.id
  return entries.value.find((e) => e.id === id)?.group ?? ''
})
const collection = computed(() => {
  const id = selected.value?.module ?? current.value?.id
  const e = entries.value.find((x) => x.id === id && (selected.value ? x.level !== 'station' : x.level === current.value?.level))
  return e ? collectionName(e.collection) : ''
})
const parent = computed(() => (scene.value?.instances ?? []).find((i) => i.id === selected.value?.parent) ?? null)

const inside = computed(() => {
  const id = shown.value?.id ?? ''
  return (scene.value?.instances ?? []).filter((i) => !i.selectAs && (i.parent ?? '') === id)
})

const parameters = computed(() => {
  const all = scene.value?.parameters ?? []
  return shown.value ? all.filter((p) => p.module === shown.value?.id) : all
})

// The entry a clicked part comes from, to open it on its own.
const shownEntry = computed(() => {
  const instance = selected.value
  if (!instance) return null
  return entries.value.find((e) => e.id === instance.module && e.level === instance.level) ?? null
})

const kinds = computed(() => scene.value?.workpieces.kinds ?? [])
const tests = computed(() => shown.value?.tests ?? [])
const controls = computed(() => shown.value?.controls ?? [])
const parts = computed(() => (selected.value ? [] : (entry.value?.contains ?? []).filter((c) => c.level === 'part')))

const hasConnectors = computed(() => (shown.value?.connectors ?? []).length > 0)

// A workpiece held at a sensor is swapped; on a transport they queue up.
async function tryWorkpiece(test: SceneTest, kind: string) {
  if (test.holds) await runAction(test.clear)
  await runAction(test.place, { kind })
}

async function openInSimulation() {
  if (current.value && simulation.scene.value?.id !== current.value.id) await loadStation(current.value.id)
  page.value = 'simulation'
}
</script>

<template>
  <aside class="details" aria-label="Details">
    <header v-if="current" class="head">
      <button v-if="selected" class="back" @click="selection = parent && parent.id !== rootInstance?.id ? parent.id : null">‹ {{ parent?.name ?? entry?.name }}</button>
      <p class="level">
        {{ LEVELS[level] ?? level }}<template v-if="group"> / {{ group }}</template
        ><template v-if="collection"> · {{ collection }}</template>
      </p>
      <h2>{{ name }}</h2>
      <p class="id mono">
        {{ shown?.module ?? current.id }}<template v-if="shown?.version"> · v{{ shown.version }}</template>
      </p>
      <p v-if="description" class="description">{{ description }}</p>
      <div class="actions">
        <button v-if="current.level === 'station'" @click="openInSimulation">Open in simulation</button>
        <button v-if="shownEntry" @click="$emit('open', shownEntry)">Open on its own</button>
        <button @click="reset()">Reset</button>
      </div>
    </header>

    <div v-if="current" class="body">
      <p v-if="entry?.error" class="section broken">{{ entry.error }}</p>
      <p v-if="actionMessage" class="section broken" role="status">{{ actionMessage }}</p>

      <section v-if="inside.length" class="section">
        <h3 class="section-title">Contains</h3>
        <button v-for="item in inside" :key="item.id" class="link" @click="selection = item.id">
          <span>{{ item.name }}</span>
          <small class="mono">{{ item.module }}</small>
        </button>
      </section>

      <SettingsSection :parameters="parameters" :faults="shown?.faults ?? []" :values="[]" :named="!shown" />

      <section v-if="tests.length || controls.length" class="section">
        <h3 class="section-title">Try it</h3>
        <div v-for="test in tests" :key="test.place" class="test">
          <span>{{ test.label }}</span>
          <div class="kinds">
            <button v-for="kind in kinds" :key="kind.id" :title="kind.label" @click="tryWorkpiece(test, kind.id)">
              <span class="swatch" :style="{ background: kind.material.color }" />{{ kind.label }}
            </button>
            <button @click="runAction(test.clear)">Remove</button>
          </div>
        </div>
        <WidgetList v-if="controls.length" :widgets="controls" />
      </section>

      <PlcSection v-if="shown?.isPlc" />
      <ConnectorsSection v-else-if="hasConnectors" :connectors="shown?.connectors ?? []" />

      <section v-if="shown?.behaviour?.length" class="section">
        <h3 class="section-title">How it behaves</h3>
        <ul class="behaviour">
          <li v-for="line in shown.behaviour" :key="line">{{ line }}</li>
        </ul>
      </section>

      <section v-if="parts.length" class="section">
        <h3 class="section-title">Built with</h3>
        <button v-for="part in parts" :key="part.id" class="link" @click="$emit('open', part)">
          <span>{{ part.name }}</span>
          <small class="mono">{{ part.id }}</small>
        </button>
      </section>
    </div>
  </aside>
</template>

<style scoped>
.details {
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

.head h2 {
  font-size: 15px;
  margin: 4px 0 0;
}

.level {
  color: var(--accent);
  font-size: 11px;
  letter-spacing: 0.08em;
  margin: 0;
  text-transform: uppercase;
}

.id {
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

.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 10px;
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

.broken {
  color: var(--danger);
  margin: 0;
}

.link {
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

.link:hover {
  background: var(--panel-raised);
}

.link small {
  color: var(--text-dim);
  font-size: 11px;
}

.test {
  display: grid;
  gap: 6px;
  margin-bottom: 10px;
}

.kinds {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.kinds button {
  align-items: center;
  display: flex;
  gap: 5px;
}

.swatch {
  border: 1px solid #0006;
  border-radius: 50%;
  height: 11px;
  width: 11px;
}

.behaviour {
  color: var(--text-dim);
  margin: 0;
  padding-left: 16px;
}

.behaviour li + li {
  margin-top: 4px;
}
</style>
