<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import type { SceneInstance } from '../../bindings/github.com/toblu96/taktwin/internal/model'
import { programming, simulation } from '../sim'
import IconChevron from '~icons/fluent/chevron-right-12-regular'
import IconStation from '~icons/fluent/grid-16-regular'
import IconModule from '~icons/fluent/cube-16-regular'
import IconAccessory from '~icons/fluent/toolbox-16-regular'
import IconPlc from '~icons/fluent/developer-board-16-regular'
import IconCode from '~icons/fluent/code-16-regular'

// The side bar of an open project, as VS Code's explorer: the station and its modules as a tree,
// with what a module holds below it, and its PLCs at the foot with how they run. Clicking a row
// selects it, as clicking it in the 3D view does; the panel on the right shows what is selected.
// Up and Down go through the rows, Right opens a row and Left closes it or goes to its parent.
const { scene, snapshot, selection } = simulation

// Guide rails and the like belong to another module, so they are not listed.
const listed = computed(() => (scene.value?.instances ?? []).filter((i) => !i.selectAs))
const inside = (id: string) => listed.value.filter((i) => i.parent === id)

const open = ref(new Set<string>())
function toggle(id: string, to = !open.value.has(id)) {
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

type Row = { item: SceneInstance; depth: number; parent?: string; children: number }
function rows(items: SceneInstance[], depth: number): Row[] {
  return items.flatMap((item) => {
    const children = inside(item.id)
    const row: Row = { item, depth, parent: item.parent, children: children.length }
    return [row, ...(open.value.has(item.id) ? rows(children, depth + 1) : [])]
  })
}
const groups = computed(() => {
  const top = listed.value.filter((i) => !i.parent)
  return [
    {
      title: 'Modules',
      rows: rows(
        top.filter((i) => i.level === 'module'),
        0,
      ),
    },
    {
      title: 'Accessories',
      rows: rows(
        top.filter((i) => i.level !== 'module'),
        0,
      ),
    },
  ].filter((g) => g.rows.length)
})

// The keyboard moves through the rows as they show.
const tree = ref<HTMLElement | null>(null)
function move(e: KeyboardEvent, row?: Row) {
  const all = Array.from(tree.value?.querySelectorAll<HTMLElement>('[data-row]') ?? [])
  const here = all.indexOf(e.currentTarget as HTMLElement)
  const focus = (i: number) => all[Math.max(0, Math.min(all.length - 1, i))]?.focus()
  if (e.key === 'ArrowDown') focus(here + 1)
  else if (e.key === 'ArrowUp') focus(here - 1)
  else if (e.key === 'ArrowRight' && row?.children && !open.value.has(row.item.id)) toggle(row.item.id, true)
  else if (e.key === 'ArrowLeft' && row && open.value.has(row.item.id)) toggle(row.item.id, false)
  else if (e.key === 'ArrowLeft' && row?.parent) {
    const parent = row.parent
    void nextTick(() => tree.value?.querySelector<HTMLElement>(`[data-row="${CSS.escape(parent)}"]`)?.focus())
  } else return
  e.preventDefault()
}

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

const row = 'flex h-7 w-full min-w-0 cursor-default items-center gap-1.5 rounded-[5px] border-0 pr-2 text-left text-fg focus-visible:-outline-offset-2'
const chosen = (on: boolean) => (on ? 'bg-highlight' : 'bg-transparent hover:bg-hover')
</script>

<template>
  <aside class="flex h-full min-w-0 flex-col bg-s1" aria-label="Station">
    <h2 class="m-0 flex h-9 shrink-0 items-center px-3 text-xs font-semibold tracking-wide text-muted uppercase">Station</h2>
    <nav ref="tree" class="min-h-0 flex-1 overflow-auto px-1.5 pb-2" aria-label="Modules of the station">
      <button data-row="" :class="[row, chosen(selection === null)]" class="pl-2" :aria-current="selection === null ? 'true' : undefined" @click="selection = null" @keydown="move">
        <IconStation class="size-4 shrink-0 text-muted" />
        <span class="truncate font-medium">{{ scene?.name }}</span>
      </button>
      <template v-for="group in groups" :key="group.title">
        <h3 class="m-0 mt-3 mb-1 px-2 text-[11px] font-medium text-faint">{{ group.title }}</h3>
        <button
          v-for="r in group.rows"
          :key="r.item.id"
          :data-row="r.item.id"
          :class="[row, chosen(selection === r.item.id)]"
          :style="{ paddingLeft: `${4 + r.depth * 16}px` }"
          :title="r.item.module"
          :aria-current="selection === r.item.id ? 'true' : undefined"
          :aria-expanded="r.children ? open.has(r.item.id) : undefined"
          @click="selection = r.item.id"
          @keydown="move($event, r)"
        >
          <span
            class="-my-1 grid size-4 shrink-0 place-items-center rounded-sm text-faint hover:text-fg"
            :class="r.children ? '' : 'invisible'"
            aria-hidden="true"
            @click.stop="toggle(r.item.id)"
          >
            <IconChevron class="size-3" :class="open.has(r.item.id) ? 'rotate-90' : ''" />
          </span>
          <component :is="r.item.isPlc ? IconPlc : r.item.level === 'module' ? IconModule : IconAccessory" class="size-4 shrink-0 text-muted" />
          <span class="truncate">{{ r.item.name }}</span>
        </button>
      </template>
    </nav>

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
