<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { signals } from './data'
import UiDialog from '../ui/UiDialog.vue'
import UiIoList from '../ui/UiIoList.vue'
import UiMenu from '../ui/UiMenu.vue'
import UiSegmented from '../ui/UiSegmented.vue'
import UiSelect from '../ui/UiSelect.vue'
import UiSwitch from '../ui/UiSwitch.vue'
import UiCheckbox from '../ui/UiCheckbox.vue'
import UiModeToggle from '../ui/UiModeToggle.vue'
import UiField from '../ui/UiField.vue'
import IconHand from '~icons/fluent/hand-point-16-filled'
import IconRun from '~icons/fluent/play-16-regular'
import IconEdit from '~icons/fluent/edit-16-regular'
import UiTabs from '../ui/UiTabs.vue'
import UiTree, { type TreeHeading, type TreeRow } from '../ui/UiTree.vue'
import IconCube from '~icons/fluent/cube-16-regular'
import IconPlay from '~icons/fluent/play-20-regular'
import IconWrench from '~icons/fluent/wrench-20-regular'
import IconRename from '~icons/fluent/rename-20-regular'
import IconDelete from '~icons/fluent/delete-20-regular'

// The lab: the app's components from src/ui on their own, in dark and light, to see and try a
// control before and while it is used in the app. The query string picks the theme, such as
// ?theme=light, for screenshots.
const params = new URLSearchParams(location.search)
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
const editMode = ref(false)
const testing = ref(false)
// A value that changes ten times a second, as a live one does, for the field that keeps what is typed.
const tick = ref(0)
window.setInterval(() => tick.value++, 100)
const ticked = ref(true)
const treeOpen = ref(true)
const picked = ref('b2')
const tree = computed<(TreeRow | TreeHeading)[]>(() => [
  { key: 'h', heading: 'Modules' },
  { key: 'conveyor', label: 'Pallet conveyor', icon: IconCube, children: true, open: treeOpen.value, selected: picked.value === 'conveyor' },
  ...(treeOpen.value
    ? [
        { key: 'b1', label: 'B1 Pallet at start', depth: 1, parent: 'conveyor', selected: picked.value === 'b1' },
        { key: 'b2', label: 'B2 Pallet at the stop', depth: 1, parent: 'conveyor', selected: picked.value === 'b2' },
      ]
    : []),
  { key: 'broken', label: 'A module that does not load', icon: IconCube, danger: true, selected: picked.value === 'broken' },
])
const order = ref(signals.map((s) => s.key))
const menu = [{ label: 'Rename', icon: IconRename, shortcut: 'F2' }, {}, { label: 'Delete…', icon: IconDelete, danger: true }]
</script>

<template>
  <div class="flex h-full flex-col">
    <div class="flex h-9 shrink-0 items-center gap-3 border-b border-line bg-s2 px-3 text-xs">
      <strong class="text-[13px]">UI lab</strong>
      <select v-model="theme" class="t-input h-6 w-auto py-0 text-xs" aria-label="Theme">
        <option value="system">System</option>
        <option value="dark">Dark</option>
        <option value="light">Light</option>
      </select>
    </div>

    <div class="min-h-0 flex-1">
      <div class="flex h-full flex-col gap-7 overflow-auto p-6">
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
          <input class="t-input w-56 font-mono text-xs select-text" value="opc.tcp://127.0.0.1:62555" aria-label="A disabled input" disabled />
          <UiField class="t-input w-32 font-mono text-xs select-text" :value="tick" aria-label="A field over a changing value" />
          <UiSwitch v-model="on" label="A switch" />
          <UiSwitch v-model="testing" label="A switch with an icon" :icon="IconHand" hide-label />
          <UiSwitch v-model="testing" label="An amber switch with an icon" :icon="IconHand" hide-label accent />
          <UiCheckbox v-model="ticked" label="A checkbox" />
          <UiModeToggle v-model="editMode" label="Edit" :off="{ icon: IconRun, label: 'Running' }" :on="{ icon: IconEdit, label: 'Editing' }" />
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
        <section class="w-72">
          <UiSelect model-value="a" label="A disabled select" disabled :options="[{ value: 'a', label: 'Pallet loading' }]" />
        </section>
        <section class="w-72 rounded-card border border-line bg-s1 p-1.5">
          <UiTree :items="tree" label="A tree" @select="(k) => (picked = k)" @toggle="(_, to) => (treeOpen = to)" />
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
