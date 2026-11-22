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
import IconExport from '~icons/fluent/arrow-export-20-regular'
import IconShare from '~icons/fluent/share-20-regular'
import IconDelete from '~icons/fluent/delete-20-regular'

// A project on the projects page: a picture of its station, its name, what it is made from, when
// it last changed and how its last check went. Clicking it opens it; its menu has the rest.
const props = defineProps<{ project: ProjectInfo; busy?: boolean }>()
const emit = defineEmits<{
  open: []
  rename: [to: string]
  duplicate: []
  reveal: []
  export: []
  handout: []
  remove: []
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

const menu = computed<MenuEntry[]>(() => [
  { label: 'Rename', icon: IconRename, run: startRename },
  { label: 'Duplicate', icon: IconCopy, run: () => emit('duplicate') },
  { label: 'Show in folder', icon: IconFolder, run: () => emit('reveal') },
  {},
  { label: 'Export…', icon: IconExport, run: () => emit('export') },
  ...(p.value.exercise ? [{ label: 'Hand out as exercise…', icon: IconShare, run: () => emit('handout') }] : []),
  {},
  { label: 'Delete…', icon: IconDelete, danger: true, run: () => emit('remove') },
])

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
  <article
    class="group flex flex-col overflow-hidden rounded-card border border-line bg-s1 transition-[border-color,translate] hover:-translate-y-px hover:border-control"
    :class="p.problem ? 'opacity-70' : ''"
  >
    <button
      class="relative block aspect-16/10 w-full cursor-default overflow-hidden border-0 bg-[#cfd7d8] p-0"
      :disabled="busy || !!p.problem"
      :aria-label="`Open ${p.name}`"
      :title="p.problem || `Open ${p.name}`"
      @click="emit('open')"
    >
      <img v-if="p.thumbnail" :src="p.thumbnail" alt="" class="block size-full object-cover" :class="p.problem ? 'grayscale' : ''" />
      <img v-else src="/logo.svg" alt="" class="absolute top-1/2 left-1/2 size-14 -translate-1/2 opacity-60" />
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
          class="block w-full cursor-default truncate border-0 bg-transparent p-0 text-left font-semibold text-fg"
          :disabled="busy || !!p.problem"
          @click="emit('open')"
        >
          {{ p.name }}
        </button>
        <p class="m-0 mt-0.5 flex items-center gap-1.5 text-xs text-muted">
          <component :is="p.exercise ? IconTask : IconCube" class="size-3.5 shrink-0" />
          <span class="truncate">{{ p.exerciseName ? `${p.exerciseName} · ${p.stationName}` : p.stationName }}</span>
        </p>
        <p v-if="p.problem" class="m-0 mt-2 flex items-start gap-1.5 text-xs text-danger"><IconWarning class="mt-px size-3.5 shrink-0" />{{ p.problem }}</p>
        <p v-else class="m-0 mt-2 flex items-center justify-between gap-2 text-xs text-faint">
          <span :title="exact(p.modified)">{{ ago(p.modified) }}</span>
          <span v-if="p.lastCheck" class="flex items-center gap-1 font-medium" :class="p.lastCheck.passed ? 'text-ok' : 'text-danger'" :title="`Checked ${exact(p.lastCheck.at)}`">
            <component :is="p.lastCheck.passed ? IconPassed : IconFailed" class="size-3.5" />{{ p.lastCheck.passed ? 'Check passed' : 'Check failed' }}
          </span>
        </p>
      </div>
      <UiMenu :items="menu" :label="`More for ${p.name}`" class="-mr-1" />
    </div>
  </article>
</template>
