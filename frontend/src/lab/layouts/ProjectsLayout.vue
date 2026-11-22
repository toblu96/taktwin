<script setup lang="ts">
import UiSegmented from '../../ui/UiSegmented.vue'
import UiMenu from '../../ui/UiMenu.vue'
import UiDialog from '../../ui/UiDialog.vue'
import UiSelect from '../../ui/UiSelect.vue'
import { computed, ref } from 'vue'
import { projects } from '../data'
import ActivityBar from './ActivityBar.vue'
import StatusBar from './StatusBar.vue'
import IconHome from '~icons/fluent/home-20-regular'
import IconLibrary from '~icons/fluent/library-20-regular'
import IconSearch from '~icons/fluent/search-20-regular'
import IconAdd from '~icons/fluent/add-20-regular'
import IconOpen from '~icons/fluent/folder-open-20-regular'
import IconCube from '~icons/fluent/cube-16-regular'
import IconTask from '~icons/fluent/clipboard-task-20-regular'
import IconPassed from '~icons/fluent/checkmark-circle-16-filled'
import IconFailed from '~icons/fluent/dismiss-circle-16-filled'
import IconRename from '~icons/fluent/rename-20-regular'
import IconCopy from '~icons/fluent/copy-20-regular'
import IconFolder from '~icons/fluent/folder-20-regular'
import IconExport from '~icons/fluent/arrow-export-20-regular'
import IconShare from '~icons/fluent/share-20-regular'
import IconDelete from '~icons/fluent/delete-20-regular'
import IconWrench from '~icons/fluent/wrench-20-regular'

// The start page in the new look: projects as cards, the way to start one, and the examples.
const view = ref('home')
const filter = ref('all')
const query = ref('')
const creating = ref(false)
const from = ref('station')
const station = ref<string | null>('pallet-loading')

const shown = computed(() =>
  projects.filter((p) => (filter.value === 'all' || (filter.value === 'exercises') === p.exercise) && p.name.toLowerCase().includes(query.value.toLowerCase())),
)
const menu = (exercise: boolean) => [
  { label: 'Rename', icon: IconRename, shortcut: 'F2' },
  { label: 'Duplicate', icon: IconCopy },
  { label: 'Show in folder', icon: IconFolder },
  {},
  { label: 'Export…', icon: IconExport },
  ...(exercise ? [{ label: 'Hand out as exercise…', icon: IconShare }] : []),
  {},
  { label: 'Delete…', icon: IconDelete, danger: true },
]
</script>

<template>
  <div class="flex h-full flex-col">
    <div class="flex min-h-0 flex-1">
      <ActivityBar
        v-model="view"
        :items="[
          { id: 'home', label: 'Projects', icon: IconHome },
          { id: 'library', label: 'Library', icon: IconLibrary },
        ]"
      />
      <main class="min-w-0 flex-1 overflow-auto">
        <div class="mx-auto max-w-[1180px] px-8 pt-7 pb-10">
          <header class="mb-5 flex items-end gap-4">
            <div>
              <h1 class="text-xl font-semibold tracking-tight">Projects</h1>
              <p class="mt-0.5 text-muted">Your work on stations and exercises, saved as you go.</p>
            </div>
            <span class="flex-1" />
            <button class="t-btn t-btn-ghost"><IconOpen class="size-4" />Open file…</button>
            <button class="t-btn t-btn-primary" @click="creating = true"><IconAdd class="size-4" />New project</button>
          </header>

          <div class="mb-4 flex items-center gap-3">
            <label class="relative w-64">
              <IconSearch class="pointer-events-none absolute top-1.5 left-2 size-4 text-faint" />
              <input v-model="query" class="t-input pl-8" placeholder="Search projects" aria-label="Search projects" />
            </label>
            <UiSegmented
              v-model="filter"
              label="Show"
              :options="[
                { value: 'all', label: 'All' },
                { value: 'stations', label: 'Stations' },
                { value: 'exercises', label: 'Exercises' },
              ]"
            />
          </div>

          <div class="grid grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-4">
            <article
              v-for="p in shown"
              :key="p.name"
              class="group hover:border-line-strong overflow-hidden rounded-card border border-line bg-s1 transition-[border-color,translate] hover:-translate-y-px"
            >
              <button class="block aspect-[16/10] w-full overflow-hidden bg-[#cfd7d8]" :aria-label="`Open ${p.name}`">
                <img :src="p.picture" alt="" class="size-full object-cover" />
              </button>
              <div class="flex items-start gap-2 px-3 pt-2.5 pb-3">
                <div class="min-w-0 flex-1">
                  <h3 class="truncate font-semibold">{{ p.name }}</h3>
                  <p class="mt-0.5 flex items-center gap-1.5 text-xs text-muted">
                    <component :is="p.exercise ? IconTask : IconCube" class="size-3.5 shrink-0" />
                    <span class="truncate">{{ p.what }}</span>
                  </p>
                  <p class="mt-2 flex items-center justify-between text-xs text-faint">
                    {{ p.when }}
                    <span v-if="p.check !== undefined" class="flex items-center gap-1 font-medium" :class="p.check ? 'text-ok' : 'text-danger'">
                      <component :is="p.check ? IconPassed : IconFailed" class="size-3.5" />{{ p.check ? 'Check passed' : 'Check failed' }}
                    </span>
                  </p>
                </div>
                <UiMenu :items="menu(p.exercise)" :label="`More for ${p.name}`" class="-mr-1" />
              </div>
            </article>
          </div>

          <h2 class="mt-9 mb-3 text-[13px] font-semibold text-muted">Start from</h2>
          <div class="grid grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-4">
            <button class="border-line-strong flex items-center gap-3 rounded-card border border-dashed p-3 text-left hover:bg-hover">
              <img src="/lab/station.jpg" alt="" class="h-12 w-[76px] rounded-[5px] object-cover" />
              <span>
                <span class="block font-semibold">Pallet loading</span>
                <span class="text-xs text-muted">Example · Taktwin</span>
              </span>
            </button>
            <button class="border-line-strong flex items-center gap-3 rounded-card border border-dashed p-3 text-left hover:bg-hover" @click="creating = true">
              <span class="flex h-12 w-[76px] items-center justify-center rounded-[5px] bg-s3 text-muted"><IconWrench class="size-5" /></span>
              <span>
                <span class="block font-semibold">Build your own</span>
                <span class="text-xs text-muted">An empty station to put together</span>
              </span>
            </button>
          </div>
        </div>
      </main>
    </div>
    <StatusBar><span class="flex-1" /><span class="px-1.5">4 projects</span></StatusBar>

    <UiDialog v-model:open="creating" title="New project" description="Start from a station, an exercise, or an empty station to build.">
      <div class="flex flex-col gap-4">
        <UiSegmented
          v-model="from"
          label="Start from"
          :options="[
            { value: 'station', label: 'Station', icon: IconCube },
            { value: 'exercise', label: 'Exercise', icon: IconTask },
            { value: 'empty', label: 'Empty', icon: IconWrench },
          ]"
        />
        <UiSelect
          v-model="station"
          label="Station"
          placeholder="Choose a station"
          :options="[
            { value: 'pallet-loading', label: 'Pallet loading', hint: 'Taktwin · a pallet line with euro pallets and cartons' },
            { value: 'line-2', label: 'Two magazines', hint: 'Your own' },
          ]"
        />
        <label class="flex flex-col gap-1">
          <span class="t-label">Name</span>
          <input class="t-input" placeholder="Pallet loading" />
        </label>
      </div>
      <template #footer>
        <button class="t-btn" @click="creating = false">Cancel</button>
        <button class="t-btn t-btn-primary" @click="creating = false">Create</button>
      </template>
    </UiDialog>
  </div>
</template>
