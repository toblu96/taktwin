<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { SceneInstance } from '../../bindings/github.com/toblu96/taktwin/internal/model'
import { programming, simulation } from '../sim'
import UiTree, { type TreeHeading, type TreeRow } from '../ui/UiTree.vue'
import IconStation from '~icons/fluent/grid-16-regular'
import IconModule from '~icons/fluent/cube-16-regular'
import IconAccessory from '~icons/fluent/toolbox-16-regular'
import IconPlc from '~icons/fluent/developer-board-16-regular'
import IconCode from '~icons/fluent/code-16-regular'

// The side bar of an open project, as VS Code's explorer: the station and its modules as a tree,
// with what a module holds below it, and its PLCs at the foot with how they run. Clicking a row
// selects it, as clicking it in the 3D view does; the panel on the right shows what is selected.
const { scene, snapshot, selection } = simulation

// Guide rails and the like belong to another module, so they are not listed.
const listed = computed(() => (scene.value?.instances ?? []).filter((i) => !i.selectAs))
const inside = (id: string) => listed.value.filter((i) => i.parent === id)

const open = ref(new Set<string>())
function toggle(id: string, to: boolean) {
  const next = new Set(open.value)
  if (to) next.add(id)
  else next.delete(id)
  open.value = next
}

// Whatever gets selected, in the tree or in the 3D view, shows with the rows above it opened.
watch(selection, (id) => {
  let at = listed.value.find((i) => i.id === id)
  const next = new Set(open.value)
  while (at?.parent) {
    next.add(at.parent)
    at = listed.value.find((i) => i.id === at?.parent)
  }
  open.value = next
})

// The tree's rows: the station, then its modules and its accessories, each with what it holds
// below it while open. The station's row has the key "", for nothing selected.
function rows(items: SceneInstance[], depth: number): TreeRow[] {
  return items.flatMap((item) => {
    const children = inside(item.id)
    const row: TreeRow = {
      key: item.id,
      label: item.name,
      depth,
      icon: item.isPlc ? IconPlc : item.level === 'module' ? IconModule : IconAccessory,
      children: children.length > 0,
      open: open.value.has(item.id),
      selected: selection.value === item.id,
      parent: item.parent ?? '',
      title: item.module,
    }
    return [row, ...(row.open ? rows(children, depth + 1) : [])]
  })
}
const items = computed<(TreeRow | TreeHeading)[]>(() => {
  const top = listed.value.filter((i) => !i.parent)
  const modules = rows(
    top.filter((i) => i.level === 'module'),
    0,
  )
  const accessories = rows(
    top.filter((i) => i.level !== 'module'),
    0,
  )
  return [
    { key: '', label: scene.value?.name ?? '', icon: IconStation, selected: selection.value === null, strong: true },
    ...(modules.length ? [{ key: 'h:modules', heading: 'Modules' }, ...modules] : []),
    ...(accessories.length ? [{ key: 'h:accessories', heading: 'Accessories' }, ...accessories] : []),
  ]
})

// The PLCs, each with a lamp: green while a program runs or the connection stands, amber while
// it connects, red when it failed. The engine calls a PLC with nothing to run "No PLC".
const idle = 'No PLC'

const plcs = computed(() => {
  const list = scene.value?.plcs?.length ? scene.value.plcs : scene.value?.plc ? [scene.value.plc] : []
  return list.map((p, i) => {
    const view = snapshot.value?.plcs?.[i] ?? null
    const link = view?.link?.state ?? ''
    return {
      id: i > 0 ? (p.module ?? '') : '',
      module: p.module ?? null,
      name: listed.value.find((m) => m.id === p.module)?.name ?? 'PLC',
      runs: view?.plc || idle,
      step: view?.step ? `S${view.step.number} ${view.step.label}` : '',
      lamp: link ? ({ connected: 'bg-ok', failed: 'bg-danger', connecting: 'bg-accent' }[link] ?? 'bg-faint') : view?.plc && view.plc !== idle ? 'bg-ok' : 'bg-faint',
      testing: !!view?.ioTest,
    }
  })
})
</script>

<template>
  <aside class="flex h-full min-w-0 flex-col bg-s1" aria-label="Station">
    <h2 class="m-0 flex h-9 shrink-0 items-center px-3 text-xs font-semibold tracking-wide text-muted uppercase">Station</h2>
    <UiTree :items="items" label="Modules of the station" class="min-h-0 flex-1 overflow-auto px-1.5 pb-2" @select="(key) => (selection = key || null)" @toggle="toggle" />

    <section v-if="plcs.length" class="shrink-0 border-t border-line p-2" aria-label="PLCs">
      <h3 class="m-0 mb-1.5 px-1 text-[11px] font-medium text-faint">{{ plcs.length > 1 ? 'PLCs' : 'PLC' }}</h3>
      <div
        v-for="p in plcs"
        :key="p.id"
        class="mb-1 flex items-center gap-1 rounded-control border border-line bg-s0 last:mb-0"
        :class="p.module && selection === p.module ? 'border-focus' : ''"
      >
        <button
          class="flex min-w-0 flex-1 cursor-default items-center gap-2 rounded-l-control border-0 bg-transparent py-1.5 pl-2.5 text-left text-fg hover:bg-hover focus-visible:-outline-offset-2"
          :disabled="!p.module"
          :title="p.module ? `Show ${p.name}: its connection and settings` : undefined"
          @click="selection = p.module"
        >
          <span class="size-2 shrink-0 rounded-full" :class="p.lamp" />
          <span class="min-w-0">
            <span class="block truncate">{{ p.name }}</span>
            <span class="block truncate text-xs text-muted">{{ p.testing ? 'I/O test' : p.runs }}<template v-if="p.step"> · {{ p.step }}</template></span>
          </span>
        </button>
        <button class="t-btn mr-1.5 h-6 px-2 text-xs" :title="`Write the program of ${p.name}`" @click="programming = p.id"><IconCode class="size-3.5" />Program</button>
      </div>
    </section>
  </aside>
</template>
