<script setup lang="ts">
import { computed } from 'vue'
import { panelOpen, panelTab } from '../project-view'
import UiTabs from '../ui/UiTabs.vue'
import EventsPanel from './EventsPanel.vue'
import { events } from '../events'
import IconClose from '~icons/fluent/dismiss-16-regular'

// The panel below the 3D view, as VS Code's: what happened while the station ran. It closes here, and opens again from the status
// bar or with ⌘J or Ctrl+J.

const tabs = computed(() => [{ value: 'events', label: 'Events', count: events.value.filter((e) => e.kind !== 'restart').length }])
const tab = computed({ get: () => panelTab.value as string, set: (v: string) => (panelTab.value = v as typeof panelTab.value) })
</script>

<template>
  <UiTabs v-model="tab" label="Panel" :tabs="tabs" class="h-full">
    <template #actions>
      <button class="t-icon-btn size-6" title="Close the panel" @click="panelOpen = false"><IconClose class="size-4" /></button>
    </template>
    <template #events><EventsPanel /></template>
  </UiTabs>
</template>
