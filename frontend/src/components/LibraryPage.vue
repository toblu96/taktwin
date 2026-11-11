<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, provide, ref } from 'vue'
import type { Entry } from '../../bindings/github.com/toblu96/taktwin/internal/model'
import { grouped } from '../collections'
import { current, enter, entries, leave, open, openError, preview, rootInstance } from '../library'
import { simKey } from '../sim'
import StationViewer from '../viewer/StationViewer.vue'
import LibraryDetails from './LibraryDetails.vue'
import StationDock from './StationDock.vue'

// The components below show the preview here, not the running simulation.
provide(simKey, preview)
const { scene } = preview

onMounted(enter)
onBeforeUnmount(leave)

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
  void open(row.entry)
}

const hint = computed(() =>
  current.value?.level === 'station'
    ? 'No PLC runs here: switch signals on the right or use the console · drag to orbit'
    : 'Click a part to inspect it · switch its signals on the right · drag to orbit',
)
</script>

<template>
  <div class="library">
    <aside class="browser" aria-label="Catalog">
      <input v-model="search" class="search" type="search" placeholder="Search catalog" aria-label="Search catalog" />
      <div class="levels">
        <template v-for="c in visible" :key="c.id">
          <button class="collection" :aria-expanded="!collapsed.has(c.id)" @click="toggleCollection(c.id)">
            <span class="fold mono">{{ collapsed.has(c.id) && !search ? '+' : '−' }}</span
            >{{ c.name }}<span class="mono">{{ c.count }}</span>
          </button>
          <section v-for="level in collapsed.has(c.id) && !search ? [] : c.levels" :key="c.id + level.level">
            <h3 class="band">
              {{ level.title }}<span class="mono">{{ level.count }}</span>
            </h3>
            <template v-for="group in level.groups" :key="group.name">
              <h4 v-if="group.name" class="group">{{ group.name }}</h4>
              <button
                v-for="row in group.rows"
                :key="row.path"
                class="row"
                :class="{ active: current && key(current) === key(row.entry), broken: row.entry.error }"
                :style="{ paddingLeft: `${10 + row.depth * 14}px` }"
                :title="row.entry.error || row.entry.description"
                @click="choose(row)"
              >
                <span class="fold mono">{{ level.tree && !search && canUnfold(row.entry) ? (expanded.has(row.path) ? '−' : '+') : '' }}</span>
                <span class="name">{{ row.entry.name }}</span>
              </button>
            </template>
          </section>
        </template>
        <p v-if="!visible.length" class="empty">No match for “{{ search }}”.</p>
      </div>
    </aside>

    <main class="stage">
      <p v-if="openError" class="failed">Could not open this entry: {{ openError }}</p>
      <StationViewer v-else-if="scene" preview :root="rootInstance?.id ?? null" :hint="hint" />
      <StationDock v-if="scene && !openError" place="bottom" />
    </main>

    <LibraryDetails @open="open" />
  </div>
</template>

<style scoped>
.library {
  display: grid;
  grid-template-columns: 240px minmax(0, 1fr) 340px;
  min-height: 0;
}

.browser {
  background: var(--panel);
  border-right: 1px solid var(--line);
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.search {
  background: var(--bg);
  border: 0;
  border-bottom: 1px solid var(--line);
  border-radius: 0;
  color: var(--text);
  height: 34px;
  outline-offset: -2px;
  padding: 0 10px;
  width: 100%;
}

.levels {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.collection {
  align-items: center;
  background: var(--bg);
  border: 0;
  border-bottom: 1px solid var(--line);
  border-radius: 0;
  color: var(--accent);
  display: flex;
  font-size: 12px;
  font-weight: 700;
  gap: 4px;
  padding: 7px 10px;
  text-align: left;
  width: 100%;
}

.collection .mono {
  color: var(--text-dim);
  font-weight: 400;
  margin-left: auto;
}

.band {
  align-items: center;
  background: var(--panel-raised);
  border-bottom: 1px solid var(--line);
  color: var(--text);
  display: flex;
  font-size: 11px;
  font-weight: 600;
  height: 24px;
  justify-content: space-between;
  letter-spacing: 0.08em;
  margin: 0;
  padding: 0 10px;
  position: sticky;
  text-transform: uppercase;
  top: 0;
  z-index: 1;
}

.band .mono {
  color: var(--text-dim);
  font-weight: 400;
  letter-spacing: 0;
}

.group {
  color: var(--text-dim);
  font-size: 11px;
  font-weight: 400;
  letter-spacing: 0.04em;
  margin: 0;
  padding: 10px 10px 3px;
}

.row {
  align-items: center;
  background: none;
  border: 0;
  border-bottom: 1px solid #1a3a3f;
  border-radius: 0;
  display: flex;
  gap: 6px;
  height: 26px;
  padding: 0 10px;
  text-align: left;
  width: 100%;
}

.row:hover:not(.active) {
  background: var(--panel-raised);
}

.row.active {
  background: var(--accent);
  border-color: var(--accent);
  color: #1b1405;
  font-weight: 600;
}

.row.broken .name {
  color: var(--danger);
}

.fold {
  color: var(--text-dim);
  flex: none;
  text-align: center;
  width: 10px;
}

.active .fold {
  color: inherit;
}

.name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.empty {
  color: var(--text-dim);
  padding: 12px 10px;
}

.stage {
  display: grid;
  grid-template-rows: minmax(0, 1fr) auto;
  min-width: 0;
}

.failed {
  color: var(--danger);
  padding: 24px;
}
</style>
