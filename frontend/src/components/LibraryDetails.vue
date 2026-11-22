<script setup lang="ts">
import { computed } from 'vue'
import type { EntryRef, SceneTest } from '../../bindings/github.com/toblu96/taktwin/internal/model'
import { collectionName } from '../collections'
import { current, entries, preview, reset, rootInstance } from '../library'
import { newProject } from '../sim'
import ConnectorsSection from './ConnectorsSection.vue'
import PlcSection from './PlcSection.vue'
import SettingsSection from './SettingsSection.vue'
import WidgetList from './WidgetList.vue'
import IconUp from '~icons/fluent/chevron-left-16-regular'
import IconAdd from '~icons/fluent/add-16-regular'
import IconOpen from '~icons/fluent/open-16-regular'
import IconReset from '~icons/fluent/arrow-counterclockwise-16-regular'
import IconWarning from '~icons/fluent/warning-16-regular'

// What the library shows: the entry chosen, or a part of it clicked in the preview, with what it
// contains, its settings, ways to try it and its signals, how it behaves and what it is built with.
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

// Makes a project on the station, and opens it.
const startProject = () => current.value && newProject({ station: current.value.id, exercise: '' })
</script>

<template>
  <aside class="flex h-full min-h-0 flex-col bg-s1" aria-label="Details">
    <header v-if="current" class="shrink-0 border-b border-line px-3 pt-2.5 pb-3">
      <button
        v-if="selected"
        class="mb-1 -ml-1 flex cursor-default items-center gap-0.5 rounded-sm border-0 bg-transparent px-1 py-0 text-xs text-muted hover:text-fg"
        @click="selection = parent && parent.id !== rootInstance?.id ? parent.id : null"
      >
        <IconUp class="size-3.5" />{{ parent?.name ?? entry?.name }}
      </button>
      <p class="m-0 text-xs text-muted">{{ LEVELS[level] ?? level }}<template v-if="group"> · {{ group }}</template><template v-if="collection"> · {{ collection }}</template></p>
      <h2 class="m-0 mt-0.5 text-[15px] leading-snug font-semibold">{{ name }}</h2>
      <p class="m-0 mt-0.5 font-mono text-[11px] text-muted">{{ shown?.module ?? current.id }}<template v-if="shown?.version"> · v{{ shown.version }}</template></p>
      <p v-if="description" class="m-0 mt-1.5 text-muted">{{ description }}</p>
      <div class="mt-3 flex flex-wrap gap-2">
        <button v-if="current.level === 'station'" class="t-btn t-btn-primary" @click="startProject"><IconAdd class="size-4" />New project</button>
        <button v-if="shownEntry" class="t-btn" @click="$emit('open', shownEntry)"><IconOpen class="size-4" />Open on its own</button>
        <button class="t-btn" title="Put it back as it started" @click="reset()"><IconReset class="size-4" />Reset</button>
      </div>
    </header>

    <div v-if="current" class="details-body min-h-0 flex-1 overflow-y-auto">
      <p v-if="entry?.error" class="section m-0 flex items-start gap-2 text-danger"><IconWarning class="mt-0.5 size-4 shrink-0" />{{ entry.error }}</p>
      <p v-if="actionMessage" class="section m-0 text-danger" role="status">{{ actionMessage }}</p>

      <section v-if="inside.length" class="section">
        <h3 class="section-title">Contains</h3>
        <ul class="m-0 flex list-none flex-col p-0">
          <li v-for="item in inside" :key="item.id">
            <button
              class="-mx-1.5 flex h-7 w-[calc(100%+0.75rem)] cursor-default items-center gap-2 rounded-[5px] border-0 bg-transparent px-1.5 text-left text-fg hover:bg-hover focus-visible:-outline-offset-2"
              @click="selection = item.id"
            >
              <span class="min-w-0 flex-1 truncate">{{ item.name }}</span>
              <span class="shrink-0 font-mono text-[11px] text-muted">{{ item.module }}</span>
            </button>
          </li>
        </ul>
      </section>

      <SettingsSection :parameters="parameters" :faults="shown?.faults ?? []" :values="[]" :named="!shown" />

      <section v-if="tests.length || controls.length" class="section">
        <h3 class="section-title">Try it</h3>
        <div v-for="test in tests" :key="test.place" class="mb-3 last:mb-0">
          <p class="m-0 mb-1.5 text-muted">{{ test.label }}</p>
          <div class="flex flex-wrap gap-1.5">
            <button v-for="kind in kinds" :key="kind.id" class="t-btn h-6 px-2 text-xs" :title="kind.label" @click="tryWorkpiece(test, kind.id)">
              <span class="size-3 rounded-full shadow-[inset_0_0_0_1px_#ffffff2e]" :style="{ background: kind.material.color }" />{{ kind.label }}
            </button>
            <button class="t-btn t-btn-ghost h-6 px-2 text-xs" @click="runAction(test.clear)">Remove</button>
          </div>
        </div>
        <WidgetList v-if="controls.length" :widgets="controls" />
      </section>

      <PlcSection v-if="shown?.isPlc" />
      <ConnectorsSection v-else-if="hasConnectors" :connectors="shown?.connectors ?? []" />

      <section v-if="shown?.behaviour?.length" class="section">
        <h3 class="section-title">How it behaves</h3>
        <ul class="m-0 flex flex-col gap-1.5 pl-4 text-muted">
          <li v-for="line in shown.behaviour" :key="line">{{ line }}</li>
        </ul>
      </section>

      <section v-if="parts.length" class="section">
        <h3 class="section-title">Built with</h3>
        <ul class="m-0 flex list-none flex-col p-0">
          <li v-for="part in parts" :key="part.id">
            <button
              class="-mx-1.5 flex h-7 w-[calc(100%+0.75rem)] cursor-default items-center gap-2 rounded-[5px] border-0 bg-transparent px-1.5 text-left text-fg hover:bg-hover focus-visible:-outline-offset-2"
              @click="$emit('open', part)"
            >
              <span class="min-w-0 flex-1 truncate">{{ part.name }}</span>
              <span class="shrink-0 font-mono text-[11px] text-muted">{{ part.id }}</span>
            </button>
          </li>
        </ul>
      </section>
    </div>
  </aside>
</template>

<style scoped>
.details-body :deep(.section) {
  border-bottom: 1px solid var(--t-line);
  padding: 12px;
}

.details-body :deep(.section:last-child) {
  border-bottom: 0;
}
</style>
