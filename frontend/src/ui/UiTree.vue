<script setup lang="ts">
import { nextTick, ref, type Component } from 'vue'
import IconChevron from '~icons/fluent/chevron-right-12-regular'

// A tree as VS Code's explorer draws one: rows indented by depth, a chevron on those that open,
// an icon and a name, the selected row in the highlight, with headings between them. Each row is
// a button of its own; Up and Down go through the rows, Right opens one and Left closes it or
// goes to its parent. Clicking the chevron only opens or closes; clicking the row selects it.
export type TreeRow = {
  key: string
  label: string
  depth?: number
  icon?: Component
  // Whether the row opens, and whether it is open now.
  children?: boolean
  open?: boolean
  selected?: boolean
  // The row above it, which Left goes to.
  parent?: string
  title?: string
  count?: number
  danger?: boolean
  strong?: boolean
}
export type TreeHeading = { key: string; heading: string; count?: number; depth?: number }

defineProps<{ items: (TreeRow | TreeHeading)[]; label: string }>()
const emit = defineEmits<{ select: [key: string]; toggle: [key: string, open: boolean] }>()

const root = ref<HTMLElement | null>(null)
const rows = () => Array.from(root.value?.querySelectorAll<HTMLElement>('[data-row]') ?? [])

function key(e: KeyboardEvent, row: TreeRow) {
  const all = rows()
  const here = all.indexOf(e.currentTarget as HTMLElement)
  const focus = (i: number) => all[Math.max(0, Math.min(all.length - 1, i))]?.focus()
  if (e.key === 'ArrowDown') focus(here + 1)
  else if (e.key === 'ArrowUp') focus(here - 1)
  else if (e.key === 'Home') focus(0)
  else if (e.key === 'End') focus(all.length - 1)
  else if (e.key === 'ArrowRight' && row.children && !row.open) emit('toggle', row.key, true)
  else if (e.key === 'ArrowLeft' && row.children && row.open) emit('toggle', row.key, false)
  else if (e.key === 'ArrowLeft' && row.parent !== undefined) {
    const parent = row.parent
    void nextTick(() => all.find((el) => el.dataset.row === parent)?.focus())
  } else return
  e.preventDefault()
}

const isRow = (item: TreeRow | TreeHeading): item is TreeRow => 'label' in item
const indent = (depth = 0) => ({ paddingLeft: `${4 + depth * 16}px` })
</script>

<template>
  <nav ref="root" :aria-label="label">
    <template v-for="item in items" :key="item.key">
      <button
        v-if="isRow(item)"
        :data-row="item.key"
        class="flex h-7 w-full min-w-0 cursor-default items-center gap-1.5 rounded-[5px] border-0 pr-2 text-left focus-visible:-outline-offset-2"
        :class="[item.selected ? 'bg-highlight' : 'bg-transparent hover:bg-hover', item.danger ? 'text-danger' : 'text-fg']"
        :style="indent(item.depth)"
        :title="item.title"
        :aria-current="item.selected ? 'true' : undefined"
        :aria-expanded="item.children ? !!item.open : undefined"
        @click="emit('select', item.key)"
        @keydown="key($event, item)"
      >
        <span
          class="-my-1 grid size-4 shrink-0 place-items-center rounded-sm text-faint hover:text-fg"
          :class="item.children ? '' : 'invisible'"
          aria-hidden="true"
          @click.stop="emit('toggle', item.key, !item.open)"
        >
          <IconChevron class="size-3" :class="item.open ? 'rotate-90' : ''" />
        </span>
        <component :is="item.icon" v-if="item.icon" class="size-4 shrink-0" :class="item.danger ? '' : 'text-muted'" />
        <span class="min-w-0 flex-1 truncate" :class="item.strong ? 'font-medium' : ''">{{ item.label }}</span>
        <span v-if="item.count !== undefined" class="shrink-0 text-xs text-faint">{{ item.count }}</span>
      </button>
      <h3 v-else class="m-0 mt-3 mb-1 flex items-center pr-2 text-[11px] font-medium text-faint first:mt-1" :style="{ paddingLeft: `${8 + (item.depth ?? 0) * 16}px` }">
        <span class="flex-1">{{ item.heading }}</span>
        <span v-if="item.count !== undefined">{{ item.count }}</span>
      </h3>
    </template>
  </nav>
</template>
