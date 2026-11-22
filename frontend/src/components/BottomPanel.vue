<script setup lang="ts">
import { computed } from 'vue'
import { panelOpen, panelTab } from '../project-view'
import { simulation } from '../sim'
import UiTabs from '../ui/UiTabs.vue'
import IoPanel from './IoPanel.vue'
import ProblemsPanel from './ProblemsPanel.vue'
import IconClose from '~icons/fluent/dismiss-16-regular'

// The panel below the 3D view, as VS Code's: the PLC's inputs and outputs, and what is wrong
// with the station. It closes here, and opens again from the status
// bar or with ⌘J or Ctrl+J.
const { scene } = simulation

const tabs = computed(() => [
  { value: 'io', label: 'Inputs and outputs' },
  { value: 'problems', label: 'Problems', count: scene.value?.checks?.length ?? 0 },
])
const tab = computed({ get: () => panelTab.value as string, set: (v: string) => (panelTab.value = v as typeof panelTab.value) })
</script>

<template>
  <UiTabs v-model="tab" label="Panel" :tabs="tabs" class="h-full">
    <template #actions>
      <button class="t-icon-btn size-6" title="Close the panel" @click="panelOpen = false"><IconClose class="size-4" /></button>
    </template>
    <template #io><IoPanel /></template>
    <template #problems><ProblemsPanel /></template>
  </UiTabs>
</template>
