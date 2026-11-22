<script setup lang="ts">
import { provide, ref, watch } from 'vue'
import { kit, kitKey, kits, library } from './kit'
import { signals } from './data'
import ProjectsLayout from './layouts/ProjectsLayout.vue'
import ProjectWindow from './layouts/ProjectWindow.vue'
import IconPlay from '~icons/fluent/play-20-regular'
import IconWrench from '~icons/fluent/wrench-20-regular'
import IconRename from '~icons/fluent/rename-20-regular'
import IconDelete from '~icons/fluent/delete-20-regular'

// The lab: the layouts in the new look, and the components on both libraries side by side.
// The query string picks what shows, such as ?view=window&lib=reka&theme=light, for screenshots.
provide(kitKey, kit)
const params = new URLSearchParams(location.search)
const view = ref(params.get('view') ?? 'projects')
const theme = ref(params.get('theme') ?? 'system')
if (params.get('lib') === 'reka') library.value = 'reka'
watch(
  theme,
  (t) => {
    if (t === 'system') delete document.documentElement.dataset.theme
    else document.documentElement.dataset.theme = t
  },
  { immediate: true },
)

const states = {
  sdk: { seg: ref('simulate'), tab: ref('check'), sel: ref<string | null>(null), open: ref(false), order: ref(signals.map((s) => s.key)) },
  reka: { seg: ref('simulate'), tab: ref('check'), sel: ref<string | null>(null), open: ref(false), order: ref(signals.map((s) => s.key)) },
}
const menu = [{ label: 'Rename', icon: IconRename, shortcut: 'F2' }, {}, { label: 'Delete…', icon: IconDelete, danger: true }]
</script>

<template>
  <div class="flex h-full flex-col">
    <div class="flex h-9 shrink-0 items-center gap-3 border-b border-line bg-s2 px-3 text-xs">
      <strong class="text-[13px]">UI lab</strong>
      <select v-model="view" class="t-input h-6 w-auto py-0 text-xs" aria-label="View">
        <option value="projects">Projects page</option>
        <option value="window">Project window</option>
        <option value="components">Components side by side</option>
      </select>
      <select v-model="library" class="t-input h-6 w-auto py-0 text-xs" aria-label="Library">
        <option value="sdk">UI SDK</option>
        <option value="reka">Reka UI</option>
      </select>
      <select v-model="theme" class="t-input h-6 w-auto py-0 text-xs" aria-label="Theme">
        <option value="system">System</option>
        <option value="dark">Dark</option>
        <option value="light">Light</option>
      </select>
      <span class="text-muted">The app's components are drawn with {{ kit.name }}.</span>
    </div>

    <div class="min-h-0 flex-1">
      <ProjectsLayout v-if="view === 'projects'" />
      <ProjectWindow v-else-if="view === 'window'" />
      <div v-else class="grid h-full grid-cols-2 divide-x divide-line overflow-auto">
        <section v-for="(k, id) in kits" :key="id" class="flex flex-col gap-6 p-6">
          <h2 class="text-base font-semibold">{{ k.name }}</h2>
          <div class="flex flex-wrap items-center gap-3">
            <component
              :is="k.Segmented"
              v-model="states[id].seg.value"
              label="View"
              :options="[
                { value: 'simulate', label: 'Simulate', icon: IconPlay },
                { value: 'build', label: 'Build', icon: IconWrench },
              ]"
            />
            <component :is="k.Menu" :items="menu" label="More" />
            <button class="t-btn" @click="states[id].open.value = true">Open dialog</button>
            <button class="t-btn t-btn-primary">Primary</button>
          </div>
          <div class="w-72">
            <component
              :is="k.Select"
              v-model="states[id].sel.value"
              label="Station"
              placeholder="Choose a station"
              :options="[
                { value: 'a', label: 'Pallet loading', hint: 'Taktwin' },
                { value: 'b', label: 'Two magazines', hint: 'Your own' },
              ]"
            />
          </div>
          <div class="h-48 rounded-card border border-line">
            <component
              :is="k.Tabs"
              v-model="states[id].tab.value"
              label="Panel"
              :tabs="[
                { value: 'check', label: 'Check', count: 5 },
                { value: 'io', label: 'Inputs' },
              ]"
            >
              <template #check><p class="text-muted">The check's lines.</p></template>
              <template #io><p class="text-muted">The inputs.</p></template>
            </component>
          </div>
          <div>
            <h3 class="t-label mb-2">I/O list: drag a row, or focus it and press Space, the arrows, Space</h3>
            <component :is="k.IoList" v-model="states[id].order.value" :signals="signals" />
          </div>
          <component :is="k.Dialog" v-model:open="states[id].open.value" title="A dialog" description="With a title, a description and buttons.">
            <p>Esc closes it, and focus returns to the button that opened it.</p>
            <template #footer>
              <button class="t-btn" @click="states[id].open.value = false">Cancel</button>
              <button class="t-btn t-btn-primary" @click="states[id].open.value = false">OK</button>
            </template>
          </component>
        </section>
      </div>
    </div>
  </div>
</template>
