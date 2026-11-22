<script setup lang="ts">
import { ref, watch } from 'vue'
import { signals } from './data'
import ProjectsLayout from './layouts/ProjectsLayout.vue'
import ProjectWindow from './layouts/ProjectWindow.vue'
import UiDialog from '../ui/UiDialog.vue'
import UiIoList from '../ui/UiIoList.vue'
import UiMenu from '../ui/UiMenu.vue'
import UiSegmented from '../ui/UiSegmented.vue'
import UiSelect from '../ui/UiSelect.vue'
import UiSwitch from '../ui/UiSwitch.vue'
import UiTabs from '../ui/UiTabs.vue'
import IconPlay from '~icons/fluent/play-20-regular'
import IconWrench from '~icons/fluent/wrench-20-regular'
import IconRename from '~icons/fluent/rename-20-regular'
import IconDelete from '~icons/fluent/delete-20-regular'

// The lab: the app's components from src/ui on their own, and layouts in the new look before they
// reach the app, in dark and light. The query string picks what shows, such as
// ?view=window&theme=light, for screenshots.
const params = new URLSearchParams(location.search)
const view = ref(params.get('view') ?? 'components')
const theme = ref(params.get('theme') ?? 'system')
watch(
  theme,
  (t) => {
    if (t === 'system') delete document.documentElement.dataset.theme
    else document.documentElement.dataset.theme = t
  },
  { immediate: true },
)

const seg = ref('simulate')
const tab = ref('check')
const station = ref<string | null>(null)
const open = ref(false)
const on = ref(true)
const order = ref(signals.map((s) => s.key))
const menu = [{ label: 'Rename', icon: IconRename, shortcut: 'F2' }, {}, { label: 'Delete…', icon: IconDelete, danger: true }]
</script>

<template>
  <div class="flex h-full flex-col">
    <div class="flex h-9 shrink-0 items-center gap-3 border-b border-line bg-s2 px-3 text-xs">
      <strong class="text-[13px]">UI lab</strong>
      <select v-model="view" class="t-input h-6 w-auto py-0 text-xs" aria-label="View">
        <option value="components">Components</option>
        <option value="projects">Projects page</option>
        <option value="window">Project window</option>
      </select>
      <select v-model="theme" class="t-input h-6 w-auto py-0 text-xs" aria-label="Theme">
        <option value="system">System</option>
        <option value="dark">Dark</option>
        <option value="light">Light</option>
      </select>
    </div>

    <div class="min-h-0 flex-1">
      <ProjectsLayout v-if="view === 'projects'" />
      <ProjectWindow v-else-if="view === 'window'" />
      <div v-else class="flex h-full flex-col gap-7 overflow-auto p-6">
        <section class="flex flex-wrap items-center gap-3">
          <UiSegmented
            v-model="seg"
            label="View"
            :options="[
              { value: 'simulate', label: 'Simulate', icon: IconPlay },
              { value: 'build', label: 'Build', icon: IconWrench },
            ]"
          />
          <UiMenu :items="menu" label="More" />
          <button class="t-btn" @click="open = true">Open dialog</button>
          <button class="t-btn t-btn-primary">Primary</button>
          <button class="t-btn t-btn-ghost">Ghost</button>
          <input class="t-input w-56" placeholder="An input" aria-label="An input" />
          <UiSwitch v-model="on" label="A switch" />
        </section>
        <section class="w-72">
          <UiSelect
            v-model="station"
            label="Station"
            placeholder="Choose a station"
            :options="[
              { value: 'a', label: 'Pallet loading', hint: 'Taktwin' },
              { value: 'b', label: 'Two magazines', hint: 'Your own' },
            ]"
          />
        </section>
        <section class="h-48 overflow-hidden rounded-card border border-line">
          <UiTabs
            v-model="tab"
            label="Panel"
            :tabs="[
              { value: 'check', label: 'Check', count: 5 },
              { value: 'io', label: 'Inputs' },
              { value: 'output', label: 'Output' },
            ]"
          >
            <template #check><p class="text-muted">The check's lines.</p></template>
            <template #io><p class="text-muted">The inputs.</p></template>
            <template #output><p class="text-muted">The output.</p></template>
          </UiTabs>
        </section>
        <section class="max-w-2xl">
          <h3 class="t-label mb-2">I/O list: drag a row, or focus it and press Space, the arrows, Space</h3>
          <UiIoList v-model="order" :signals="signals" />
        </section>
        <UiDialog v-model:open="open" title="A dialog" description="With a title, a description and buttons.">
          <p>Esc closes it, and focus returns to the button that opened it.</p>
          <template #footer>
            <button class="t-btn" @click="open = false">Cancel</button>
            <button class="t-btn t-btn-primary" @click="open = false">OK</button>
          </template>
        </UiDialog>
      </div>
    </div>
  </div>
</template>

<style>
html,
body,
#lab {
  height: 100%;
  margin: 0;
}
</style>
