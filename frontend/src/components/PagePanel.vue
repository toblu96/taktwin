<script setup lang="ts">
import { onActivated, onBeforeUnmount, onDeactivated, onMounted, ref, watch } from 'vue'
import { panelHeight } from '../layout'
import { panelArrived, panelLeft, panelOpen } from '../panel'
import UiSash from '../ui/UiSash.vue'
import UiTabs from '../ui/UiTabs.vue'
import IconClose from '~icons/fluent/dismiss-16-regular'

// The panel below a page's middle, with the tabs the page has for it, such as the events of a
// running station or the problems of one being built. Each tab's content goes into the slot of
// its value. It is open or closed, and as high, on every page that has one (panel.ts); its top
// edge drags. A page in a kept-alive view counts as gone while another shows.
// Covered, as by the program's editor over the middle, its edge does not catch the pointer.
const props = defineProps<{ tabs: { value: string; label: string; count?: number }[]; covered?: boolean }>()

const tab = ref(props.tabs[0]?.value ?? '')
watch(
  () => props.tabs.map((t) => t.value),
  (values) => {
    if (!values.includes(tab.value)) tab.value = values[0] ?? ''
  },
)

let here = false
const arrive = () => {
  if (!here) panelArrived()
  here = true
}
const leave = () => {
  if (here) panelLeft()
  here = false
}
onMounted(arrive)
onActivated(arrive)
onDeactivated(leave)
onBeforeUnmount(leave)
</script>

<template>
  <div v-if="panelOpen" class="relative max-h-[70vh] shrink-0 border-0 border-t border-solid border-line" :style="{ height: `${panelHeight}px` }">
    <UiSash v-if="!covered" v-model="panelHeight" side="bottom" :min="120" :max="720" label="Height of the panel" />
    <UiTabs v-model="tab" label="Panel" :tabs="tabs" class="h-full">
      <template #actions>
        <button class="t-icon-btn size-6" title="Close the panel" @click="panelOpen = false"><IconClose class="size-4" /></button>
      </template>
      <template v-for="t in tabs" :key="t.value" #[t.value]><slot :name="t.value" /></template>
    </UiTabs>
  </div>
</template>
