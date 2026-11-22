<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { BuilderModule } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { Entry } from '../../bindings/github.com/toblu96/taktwin/internal/model'
import { built, busy, pending, pick, selected, setStation, state } from '../builder'
import { grouped } from '../collections'
import UiTabs from '../ui/UiTabs.vue'
import UiSelect from '../ui/UiSelect.vue'
import UiTree, { type TreeHeading, type TreeRow } from '../ui/UiTree.vue'
import IconStand from '~icons/fluent/grid-16-regular'
import IconModule from '~icons/fluent/cube-16-regular'
import IconAccessory from '~icons/fluent/toolbox-16-regular'
import IconSearch from '~icons/fluent/search-16-regular'

// The side bar of the builder: the station's name and what it is about, then either the modules
// in the station as a tree, each stand with what stands on it and each module with what is
// screwed onto it, or what can be added, from one collection at a time.
const props = defineProps<{ entries: Entry[] }>()

// --- The station's name and description, kept as they are left. ---

const name = ref('')
const description = ref('')
watch(
  () => state.value?.station,
  (station) => {
    name.value = station?.name ?? ''
    description.value = station?.description ?? ''
  },
  { immediate: true },
)
function rename() {
  const station = state.value?.station
  if (station && (name.value !== station.name || description.value !== station.description)) void setStation(name.value, description.value)
}

const tab = ref('station')
watch(pending, (p) => p && (tab.value = 'add'))
const levelOf = (m: BuilderModule) => props.entries.find((e) => e.id === m.module && e.level !== 'station')?.level

// --- The modules in the station, as a tree. ---

const closed = ref(new Set<string>())
const tree = computed<TreeRow[]>(() => {
  const modules = state.value?.modules ?? []
  const ids = new Set(modules.map((m) => m.id))
  const parentOf = (m: BuilderModule) => {
    if (m.parent) return m.parent
    const on = m.place?.on
    if (!on || on === 'floor.area') return ''
    const owner = on.split('.')[0].split('/')[0]
    return ids.has(owner) ? owner : ''
  }
  const out: TreeRow[] = []
  const seen = new Set<string>()
  const visit = (parent: string, depth: number) => {
    for (const m of modules) {
      if (parentOf(m) !== parent || seen.has(m.id)) continue
      seen.add(m.id)
      const children = modules.some((c) => parentOf(c) === m.id)
      const stand = m.place?.on === 'floor.area'
      out.push({
        key: m.id,
        label: m.name,
        depth,
        icon: stand ? IconStand : levelOf(m) === 'module' ? IconModule : IconAccessory,
        children,
        open: !closed.value.has(m.id),
        selected: selected.value?.id === m.id,
        parent: parent || undefined,
        strong: stand,
        title: stand ? 'A stand' : m.place ? `On ${m.place.on.split('.').slice(-1)[0]}, ${m.place.at} mm along` : m.parent ? 'A part of it' : 'At a fixed place',
      })
      if (children && !closed.value.has(m.id)) visit(m.id, depth + 1)
    }
  }
  visit('', 0)
  return out
})
function toggle(id: string, open: boolean) {
  const next = new Set(closed.value)
  if (open) next.delete(id)
  else next.add(id)
  closed.value = next
}

// --- What can be added: what can be screwed on somewhere, from a collection. ---

const ALL = '*'
const search = ref('')
const chosenCollection = ref<string | null>(null)
const placeable = computed(() => props.entries.filter((e) => e.mounts?.length && !e.error && e.level !== 'station'))
const collections = computed(() => grouped(placeable.value).map((g) => ({ value: g.id, label: g.name })))
// By default the collection most of the station is from.
const builtFrom = computed(() => {
  const counts = new Map<string, number>()
  for (const m of state.value?.modules ?? []) {
    const c = props.entries.find((e) => e.id === m.module && e.level !== 'station')?.collection
    if (c) counts.set(c, (counts.get(c) ?? 0) + 1)
  }
  return [...counts].sort((a, b) => b[1] - a[1])[0]?.[0] ?? collections.value[0]?.value ?? ALL
})
const collection = computed({ get: () => chosenCollection.value ?? builtFrom.value, set: (v: string | null) => (chosenCollection.value = v ?? ALL) })
const palette = computed<(TreeRow | TreeHeading)[]>(() => {
  const term = search.value.trim().toLowerCase()
  const list = placeable.value.filter(
    (e) => (collection.value === ALL || (e.collection ?? '') === collection.value) && (!term || `${e.name} ${e.id} ${e.group ?? ''}`.toLowerCase().includes(term)),
  )
  const groups = new Map<string, Entry[]>()
  for (const e of list) {
    const title = e.level === 'module' ? 'Modules' : e.group || 'Accessories'
    groups.set(title, [...(groups.get(title) ?? []), e])
  }
  return [...groups].flatMap(([title, entries]) => [
    { key: `h:${title}`, heading: title, count: entries.length } as TreeHeading,
    ...entries
      .sort((a, b) => a.name.localeCompare(b.name))
      .map((e): TreeRow => ({
        key: e.id,
        label: e.name,
        icon: e.level === 'module' ? IconModule : IconAccessory,
        selected: pending.value?.id === e.id,
        title: `${e.description ?? e.name} · screws onto ${(e.mounts ?? []).join(', ')}`,
      })),
  ])
})
function add(id: string) {
  const e = placeable.value.find((x) => x.id === id)
  if (e && !busy.value) void pick(e)
}
</script>

<template>
  <aside class="flex h-full min-w-0 flex-col bg-s1" aria-label="Station">
    <section class="t-section flex shrink-0 flex-col gap-2">
      <label class="flex flex-col gap-1">
        <span class="t-label">Station</span>
        <input v-model="name" class="t-input font-semibold" aria-label="Name of the station" @change="rename" />
      </label>
      <label class="flex flex-col gap-1">
        <span class="t-label">What it is about</span>
        <textarea v-model="description" class="t-input h-auto min-h-16 resize-y py-1.5 font-[inherit] leading-snug" rows="3" @change="rename" />
      </label>
    </section>

    <UiTabs
      v-model="tab"
      label="Modules"
      flush
      class="min-h-0 flex-1"
      :tabs="[
        { value: 'station', label: 'In the station', count: state?.modules?.length ?? 0 },
        { value: 'add', label: 'Add' },
      ]"
    >
      <template #station>
        <UiTree :items="tree" label="Modules in the station" class="px-1.5 py-2" @select="(id) => (built.selection.value = id)" @toggle="toggle" />
      </template>
      <template #add>
        <div class="flex flex-col gap-2 px-3 pt-3 pb-2">
          <UiSelect v-if="collections.length > 1" v-model="collection" label="Collection" hide-label :options="[...collections, { value: ALL, label: 'All collections' }]" />
          <div class="relative">
            <IconSearch class="pointer-events-none absolute top-1.5 left-2 size-4 text-faint" />
            <input v-model="search" class="t-input pl-8" type="search" placeholder="Search what to add" aria-label="Search what to add" @keydown.esc="search = ''" />
          </div>
          <p class="m-0 text-xs text-muted">Pick one, then point at a line in the 3D view and click to put it down.</p>
        </div>
        <UiTree :items="palette" label="What can be added" class="px-1.5 pb-2" @select="add" />
      </template>
    </UiTabs>
  </aside>
</template>
