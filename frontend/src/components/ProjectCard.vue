<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import type { ProjectInfo } from '../../bindings/github.com/toblu96/taktwin/internal/simapp/models'
import UiMenu, { type MenuEntry } from '../ui/UiMenu.vue'
import IconCube from '~icons/fluent/cube-16-regular'
import IconTask from '~icons/fluent/clipboard-task-20-regular'
import IconPassed from '~icons/fluent/checkmark-circle-16-filled'
import IconFailed from '~icons/fluent/dismiss-circle-16-filled'
import IconWarning from '~icons/fluent/warning-16-filled'
import IconRename from '~icons/fluent/rename-20-regular'
import IconCopy from '~icons/fluent/copy-20-regular'
import IconFolder from '~icons/fluent/folder-20-regular'
import IconShare from '~icons/fluent/share-20-regular'
import IconDelete from '~icons/fluent/delete-20-regular'
import IconRemove from '~icons/fluent/dismiss-20-regular'
import IconSearch from '~icons/fluent/search-20-regular'

// A project file on the projects page: a picture of its station, its name, what it is made from,
// when it last changed, where it is if not with the others, and how its last check went.
// Clicking it opens it; its menu has the rest. A file opened from elsewhere is taken off the
// list rather than deleted, and one that was moved can be found again.
const props = defineProps<{ project: ProjectInfo; busy?: boolean }>()
const emit = defineEmits<{
  open: []
  rename: [to: string]
  copy: []
  reveal: []
  handout: []
  remove: []
  forget: []
  locate: []
}>()

const p = computed(() => props.project)

const renaming = ref(false)
const newName = ref('')
const input = ref<HTMLInputElement | null>(null)
async function startRename() {
  newName.value = p.value.name
  renaming.value = true
  await nextTick()
  input.value?.select()
}
function finishRename() {
  if (!renaming.value) return
  renaming.value = false
  const to = newName.value.trim()
  if (to && to !== p.value.name) emit('rename', to)
}

const menu = computed<MenuEntry[]>(() =>
  p.value.missing
    ? [
        { label: 'Locate…', icon: IconSearch, run: () => emit('locate') },
        { label: 'Remove from list', icon: IconRemove, run: () => emit('forget') },
      ]
    : [
        { label: 'Rename', icon: IconRename, run: startRename },
        { label: 'Save a copy…', icon: IconCopy, run: () => emit('copy') },
        { label: 'Show in folder', icon: IconFolder, run: () => emit('reveal') },
        {},
        { label: 'Hand out…', icon: IconShare, run: () => emit('handout') },
        {},
        p.value.inFolder
          ? { label: 'Delete…', icon: IconDelete, danger: true, run: () => emit('remove') }
          : { label: 'Remove from list', icon: IconRemove, run: () => emit('forget') },
      ],
)

// The folder a file opened from elsewhere is in, by its name.
const folder = computed(() => (p.value.inFolder ? '' : (p.value.path.split(/[\\/]/).slice(-2, -1)[0] ?? '')))

const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })
const units: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 31536000],
  ['month', 2592000],
  ['week', 604800],
  ['day', 86400],
  ['hour', 3600],
  ['minute', 60],
]
function ago(iso?: string) {
  if (!iso) return ''
  const seconds = (Date.parse(iso) - Date.now()) / 1000
  for (const [unit, size] of units) if (Math.abs(seconds) >= size) return rtf.format(Math.round(seconds / size), unit)
  return 'just now'
}
const exact = (iso?: string) => (iso ? new Date(iso).toLocaleString() : '')
</script>

<template>
  <article class="group flex flex-col overflow-hidden rounded-card border border-line bg-s1 hover:border-control">
    <button
      class="relative block aspect-16/10 w-full cursor-default overflow-hidden border-0 bg-[#cfd7d8] p-0 focus-visible:outline-offset-[-3px]"
      :class="p.problem ? 'opacity-60' : ''"
      :disabled="busy || !!p.problem"
      :aria-label="`Open ${p.name}${p.exercise ? ', Exercise' : ''}`"
      :title="p.problem || `Open ${p.name}`"
      @click="emit('open')"
    >
      <img v-if="p.thumbnail" :src="p.thumbnail" alt="" class="block size-full object-cover" :class="p.problem ? 'grayscale' : ''" />
      <img v-else src="/logo.svg" alt="" class="absolute top-1/2 left-1/2 size-14 -translate-1/2 opacity-60" />
      <span v-if="p.exercise" class="absolute top-2 left-2 rounded-sm bg-accent px-1.5 text-[11px] leading-4.5 font-semibold text-on-accent">Exercise</span>
    </button>
    <div class="flex items-start gap-2 px-3 pt-2.5 pb-3">
      <div class="min-w-0 flex-1">
        <input
          v-if="renaming"
          ref="input"
          v-model="newName"
          class="t-input h-6 px-1.5 font-semibold"
          type="text"
          maxlength="80"
          aria-label="New name"
          @keydown.enter="finishRename"
          @keydown.esc="renaming = false"
          @blur="finishRename"
        />
        <button
          v-else
          tabindex="-1"
          class="block w-full cursor-default truncate border-0 bg-transparent p-0 text-left font-semibold text-fg"
          :title="p.path"
          :disabled="busy || !!p.problem"
          @click="emit('open')"
        >
          {{ p.name }}
        </button>
        <p v-if="!p.missing" class="m-0 mt-0.5 flex items-center gap-1.5 text-xs text-muted">
          <component :is="p.exercise ? IconTask : IconCube" class="size-3.5 shrink-0" />
          <span class="truncate">{{ p.exerciseName ? `${p.exerciseName} · ${p.stationName}` : p.stationName }}</span>
        </p>
        <p v-if="p.problem" class="m-0 mt-2 flex items-start gap-1.5 text-xs text-danger"><IconWarning class="mt-px size-3.5 shrink-0" />{{ p.problem }}</p>
        <p v-else class="m-0 mt-2 flex items-center justify-between gap-2 text-xs text-faint">
          <span class="truncate" :title="exact(p.modified)">{{ ago(p.modified) }}<span v-if="folder" :title="p.path"> · in {{ folder }}</span></span>
          <span v-if="p.lastCheck" class="flex items-center gap-1 font-medium" :class="p.lastCheck.passed ? 'text-ok' : 'text-danger'" :title="`Checked ${exact(p.lastCheck.at)}`">
            <component :is="p.lastCheck.passed ? IconPassed : IconFailed" class="size-3.5" />{{ p.lastCheck.passed ? 'Check passed' : 'Check failed' }}
          </span>
        </p>
      </div>
      <UiMenu :items="menu" :label="`More for ${p.name}`" class="-mr-1" />
    </div>
  </article>
</template>
