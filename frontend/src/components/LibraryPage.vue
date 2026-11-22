<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, provide, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { Entry, EntryRef } from '../../bindings/github.com/toblu96/taktwin/internal/model'
import { grouped } from '../collections'
import { detailsWidth, treeWidth } from '../layout'
import { current, enter, entries, lastEntry, leave, open, openError, preview, rootInstance } from '../library'
import { simKey } from '../sim'
import UiSash from '../ui/UiSash.vue'
import UiTree, { type TreeHeading, type TreeRow } from '../ui/UiTree.vue'
import StationViewer from '../viewer/StationViewer.vue'
import LibraryDetails from './LibraryDetails.vue'
import StationDock from './StationDock.vue'
import IconCollection from '~icons/fluent/library-16-regular'
import IconStation from '~icons/fluent/grid-16-regular'
import IconModule from '~icons/fluent/cube-16-regular'
import IconAccessory from '~icons/fluent/toolbox-16-regular'
import IconPart from '~icons/fluent/puzzle-piece-16-regular'
import IconSearch from '~icons/fluent/search-16-regular'
import IconWarning from '~icons/fluent/warning-16-regular'

// The library: the collections' stations, modules, accessories and parts in a tree on the left,
// the one chosen in a preview without a PLC in the middle, and what it is on the right, as an
// open project lays out its station. The address names the entry shown, such as
// #/library/module/pallet-conveyor; without one it shows the entry shown last.

// The components below show the preview here, not the running simulation.
provide(simKey, preview)
const { scene } = preview

const route = useRoute()
const router = useRouter()
const show = (e: EntryRef) => router.replace({ name: 'library', params: { level: e.level, id: e.id } })

onMounted(async () => {
  await enter()
  if (!route.params.id) {
    const first = lastEntry()
    if (first) await show(first)
  }
})
onBeforeUnmount(leave)
watch(
  () => [route.params.level, route.params.id, entries.value.length] as const,
  ([level, id]) => {
    // A part of the address that is left out comes as "".
    if (typeof level !== 'string' || typeof id !== 'string' || !level || !id || !entries.value.length) return
    if (current.value?.level !== level || current.value?.id !== id) void open({ level, id } as EntryRef)
  },
  { immediate: true },
)

const search = ref('')
const expanded = ref(new Set<string>())

const levels = [
  { level: 'station', title: 'Stations', tree: true },
  { level: 'module', title: 'Modules', tree: true },
  { level: 'accessory', title: 'Accessories', tree: false },
  { level: 'part', title: 'Parts', tree: false },
]

// Sensors and actuators first, then the rest.
const groupOrder = ['Sensors', 'Actuators', 'PLC and I/O', 'Mechanical', 'Pneumatics']

const key = (e: { level: string; id: string }) => `${e.level}/${e.id}`

// Stations and modules unfold into what they are built from. A search lists
// everything that matches, flat.
function rows(entry: Entry, depth: number, parents: string[]): { entry: Entry; depth: number; path: string }[] {
  const path = [...parents, key(entry)].join('>')
  const list = [{ entry, depth, path }]
  if (!expanded.value.has(path)) return list
  for (const ref of entry.contains ?? []) {
    const child = entries.value.find((e) => e.id === ref.id && e.level === ref.level)
    if (child && child.level !== 'part') list.push(...rows(child, depth + 1, [...parents, key(entry)]))
  }
  return list
}

const canUnfold = (entry: Entry) => (entry.contains ?? []).some((c) => c.level !== 'part')

// Each collection with its levels: stations, modules, accessories and parts.
const collapsed = ref(new Set<string>())
function toggleCollection(id: string) {
  const next = new Set(collapsed.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  collapsed.value = next
}

const visible = computed(() => {
  const term = search.value.trim().toLowerCase()
  return grouped(entries.value)
    .map((c) => ({ ...c, levels: levelsOf(c.items, term) }))
    .map((c) => ({ ...c, count: c.levels.reduce((n, l) => n + l.count, 0) }))
    .filter((c) => c.count > 0)
})

function levelsOf(all: Entry[], term: string) {
  return levels
    .map((level) => {
      const inLevel = all.filter((e) => e.level === level.level)
      const matching = term ? inLevel.filter((e) => `${e.name} ${e.id} ${e.group ?? ''} ${e.description ?? ''}`.toLowerCase().includes(term)) : inLevel
      const names = [...new Set(matching.map((e) => e.group ?? ''))].sort((a, b) => (groupOrder.indexOf(a) + 1 || 99) - (groupOrder.indexOf(b) + 1 || 99))
      return {
        ...level,
        count: matching.length,
        groups: names.map((name) => ({
          name,
          rows: matching.filter((e) => (e.group ?? '') === name).flatMap((e) => (level.tree && !term ? rows(e, 0, []) : [{ entry: e, depth: 0, path: key(e) }])),
        })),
      }
    })
    .filter((level) => level.count > 0)
}

function choose(row: { entry: Entry; path: string }) {
  if (canUnfold(row.entry)) {
    const next = new Set(expanded.value)
    // Choosing it again folds it, while choosing it the first time always shows it.
    if (current.value && key(current.value) === key(row.entry) && next.has(row.path)) next.delete(row.path)
    else next.add(row.path)
    expanded.value = next
  }
  void show(row.entry)
}

// The tree's rows: each collection, which opens and closes, with its levels and groups as
// headings and its entries below them.
const icons: Record<string, typeof IconStation> = { station: IconStation, module: IconModule, accessory: IconAccessory, part: IconPart }
const byKey = new Map<string, { entry: Entry; path: string }>()
const items = computed<(TreeRow | TreeHeading)[]>(() => {
  byKey.clear()
  const term = search.value.trim()
  return visible.value.flatMap((c) => {
    const open = !collapsed.value.has(c.id) || !!term
    const head: TreeRow = { key: `c:${c.id}`, label: c.name, icon: IconCollection, children: true, open, count: c.count, strong: true }
    if (!open) return [head]
    return [
      head,
      ...c.levels.flatMap((level) => [
        { key: `l:${c.id}:${level.level}`, heading: level.title, count: level.count } as TreeHeading,
        ...level.groups.flatMap((group) => [
          ...(group.name ? [{ key: `g:${c.id}:${level.level}:${group.name}`, heading: group.name, depth: 1 } as TreeHeading] : []),
          ...group.rows.map((row): TreeRow => {
            const k = `e:${c.id}:${row.path}`
            byKey.set(k, row)
            const parentPath = row.path.split('>').slice(0, -1).join('>')
            return {
              key: k,
              label: row.entry.name,
              depth: row.depth + (group.name ? 1 : 0),
              icon: row.entry.error ? IconWarning : icons[row.entry.level],
              children: level.tree && !term && canUnfold(row.entry),
              open: expanded.value.has(row.path),
              selected: !!current.value && key(current.value) === key(row.entry),
              parent: parentPath ? `e:${c.id}:${parentPath}` : `c:${c.id}`,
              title: row.entry.error || row.entry.description,
              danger: !!row.entry.error,
            }
          }),
        ]),
      ]),
    ]
  })
})

function select(k: string) {
  if (k.startsWith('c:')) toggleCollection(k.slice(2))
  else {
    const row = byKey.get(k)
    if (row) choose(row)
  }
}
function toggle(k: string, to: boolean) {
  if (k.startsWith('c:')) {
    if (collapsed.value.has(k.slice(2)) === to) toggleCollection(k.slice(2))
    return
  }
  const row = byKey.get(k)
  if (!row) return
  const next = new Set(expanded.value)
  if (to) next.add(row.path)
  else next.delete(row.path)
  expanded.value = next
}

const hint = computed(() =>
  current.value?.level === 'station'
    ? 'No PLC runs here: switch signals on the right or use the console · drag to orbit'
    : 'Click a part to inspect it · switch its signals on the right · drag to orbit',
)
</script>

<template>
  <main class="flex h-full min-h-0">
    <div class="relative shrink-0 border-r border-line" :style="{ width: `${treeWidth}px` }">
      <aside class="flex h-full min-w-0 flex-col bg-s1" aria-label="Catalog">
        <h2 class="m-0 flex h-9 shrink-0 items-center px-3 text-xs font-semibold tracking-wide text-muted uppercase">Library</h2>
        <div class="relative shrink-0 px-2 pb-2">
          <IconSearch class="pointer-events-none absolute top-1.5 left-4 size-4 text-faint" />
          <input v-model="search" class="t-input pl-8" type="search" placeholder="Search the library" aria-label="Search the library" @keydown.esc="search = ''" />
        </div>
        <UiTree :items="items" label="Library" class="min-h-0 flex-1 overflow-auto px-1.5 pb-2" @select="select" @toggle="toggle" />
        <p v-if="!visible.length" class="m-0 px-3 py-2 text-muted">Nothing matches “{{ search }}”.</p>
      </aside>
      <UiSash v-model="treeWidth" side="left" :min="180" :max="480" label="Width of the library's tree" />
    </div>

    <div class="relative flex min-w-0 flex-1 flex-col">
      <p v-if="openError" class="m-auto flex max-w-md items-start gap-2 p-6 text-danger" role="alert">
        <IconWarning class="mt-0.5 size-4 shrink-0" />Could not open this entry: {{ openError }}
      </p>
      <div v-else-if="scene" class="relative grid min-h-0 flex-1">
        <StationViewer preview :root="rootInstance?.id ?? null" :hint="hint" />
      </div>
      <StationDock v-if="scene && !openError" place="bottom" />
    </div>

    <div class="relative shrink-0 border-l border-line" :style="{ width: `${detailsWidth}px` }">
      <UiSash v-model="detailsWidth" side="right" :min="260" :max="560" label="Width of the details" />
      <LibraryDetails @open="show" />
    </div>
  </main>
</template>
