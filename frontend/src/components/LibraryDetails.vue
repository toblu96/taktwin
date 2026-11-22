<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { EntryRef, SceneTest } from '../../bindings/github.com/toblu96/taktwin/internal/model'
import { collectionName } from '../collections'
import { current, entries, preview, reset, rootInstance } from '../library'
import { newProject } from '../sim'
import ConnectorsSection from './ConnectorsSection.vue'
import PlcSection from './PlcSection.vue'
import SettingsSection from './SettingsSection.vue'
import WidgetList from './WidgetList.vue'
import UiTree, { type TreeRow } from '../ui/UiTree.vue'
import IconModule from '~icons/fluent/cube-16-regular'
import IconAccessory from '~icons/fluent/toolbox-16-regular'
import IconPlc from '~icons/fluent/developer-board-16-regular'
import IconUp from '~icons/fluent/chevron-left-16-regular'
import IconAdd from '~icons/fluent/add-16-regular'
import IconOpen from '~icons/fluent/open-16-regular'
import IconReset from '~icons/fluent/arrow-counterclockwise-16-regular'
import IconWarning from '~icons/fluent/warning-16-regular'

// What the library shows: the entry chosen, or a part of it clicked in the preview, with how it
// behaves, what it holds as a tree, ways to try it, its settings and its signals.
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

// What it holds, as a tree: a module's parts, or a station's modules with theirs. A row opens
// to what it holds in turn; choosing one shows it here, with the way back above.
const listed = computed(() => (scene.value?.instances ?? []).filter((i) => !i.selectAs))
const open = ref(new Set<string>())
watch(current, () => (open.value = new Set()))
function toggle(id: string, to: boolean) {
  const next = new Set(open.value)
  if (to) next.add(id)
  else next.delete(id)
  open.value = next
}
function rows(parent: string, depth: number): TreeRow[] {
  return listed.value
    .filter((i) => (i.parent ?? '') === parent)
    .flatMap((i) => {
      const children = listed.value.some((c) => c.parent === i.id)
      const row: TreeRow = {
        key: i.id,
        label: i.name,
        depth,
        icon: i.isPlc ? IconPlc : i.level === 'module' ? IconModule : IconAccessory,
        children,
        open: open.value.has(i.id),
        parent: depth > 0 ? parent : undefined,
        title: i.module,
      }
      return [row, ...(children && row.open ? rows(i.id, depth + 1) : [])]
    })
}
const inside = computed(() => rows(shown.value?.id ?? '', 0))

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
// The kinds of workpiece a test takes, as the module says, or all of them.
const kindsFor = (test: SceneTest) => (test.kinds?.length ? kinds.value.filter((k) => test.kinds?.includes(k.id)) : kinds.value)

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
        <button v-if="current.level === 'station' && !selected" class="t-btn t-btn-primary" @click="startProject"><IconAdd class="size-4" />New project</button>
        <button v-if="shownEntry" class="t-btn" @click="$emit('open', shownEntry)"><IconOpen class="size-4" />Open on its own</button>
        <button v-if="!selected" class="t-btn" title="Put it back as it started" @click="reset()"><IconReset class="size-4" />Reset</button>
      </div>
    </header>

    <div v-if="current" class="details-body min-h-0 flex-1 overflow-y-auto">
      <p v-if="entry?.error" class="t-section m-0 flex items-start gap-2 text-danger"><IconWarning class="mt-0.5 size-4 shrink-0" />{{ entry.error }}</p>
      <p v-if="actionMessage" class="t-section m-0 text-danger" role="status">{{ actionMessage }}</p>

      <section v-if="shown?.behaviour?.length" class="t-section">
        <h3 class="t-section-title">How it behaves</h3>
        <ul class="m-0 flex flex-col gap-1.5 pl-4 text-muted">
          <li v-for="line in shown.behaviour" :key="line">{{ line }}</li>
        </ul>
      </section>

      <section v-if="inside.length" class="t-section">
        <h3 class="t-section-title">Contains</h3>
        <UiTree :items="inside" :label="`What ${name} holds`" class="-mx-1.5" @select="(id) => (selection = id)" @toggle="toggle" />
      </section>

      <section v-if="tests.length || controls.length" class="t-section">
        <h3 class="t-section-title">Try it</h3>
        <div v-for="test in tests" :key="test.place" class="mb-3 last:mb-0">
          <p class="m-0 mb-1.5 text-muted">{{ test.label }}</p>
          <div class="flex flex-wrap gap-1.5">
            <button v-for="kind in kindsFor(test)" :key="kind.id" class="t-btn h-6 px-2 text-xs" :title="kind.label" @click="tryWorkpiece(test, kind.id)">
              <span class="size-3 rounded-full shadow-[inset_0_0_0_1px_#ffffff2e]" :style="{ background: kind.material.color }" />{{ kind.label }}
            </button>
            <button class="t-btn t-btn-ghost h-6 px-2 text-xs" @click="runAction(test.clear)">Remove</button>
          </div>
        </div>
        <WidgetList v-if="controls.length" :widgets="controls" />
      </section>

      <SettingsSection :parameters="parameters" :faults="shown?.faults ?? []" :values="[]" :named="!shown" />

      <PlcSection v-if="shown?.isPlc" />
      <ConnectorsSection v-else-if="hasConnectors" :connectors="shown?.connectors ?? []" />
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
