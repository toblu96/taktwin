<script setup lang="ts">
import { computed, type Component } from 'vue'
import { useRoute } from 'vue-router'
import { closeProject, project } from '../sim'
import { showView } from '../project-view'
import { go, toggleSettings } from '../router'
import { updateAvailable } from '../update'
import IconHome from '~icons/fluent/home-20-regular'
import IconLibrary from '~icons/fluent/library-20-regular'
import IconBack from '~icons/fluent/arrow-left-20-regular'
import IconPlay from '~icons/fluent/play-20-regular'
import IconWiring from '~icons/fluent/plug-connected-20-regular'
import IconExercise from '~icons/fluent/clipboard-task-20-regular'
import IconSettings from '~icons/fluent/settings-20-regular'

// The bar along the left edge, as in VS Code: where you are, one icon each, with the active one
// marked by an amber line. Without a project it leads to the projects and the library; in a
// project, back to them, and to its views: the simulation, the wiring and the exercise, each run
// or edited as the switch in the top bar says. Settings sit at the foot.
type Item = { id: string; label: string; icon: Component; active: boolean; disabled?: string; run: () => unknown; back?: boolean }

const route = useRoute()
const items = computed<Item[]>(() => {
  if (!project.value)
    return [
      { id: 'projects', label: 'Projects', icon: IconHome, active: route.name === 'projects', run: () => go({ name: 'projects' }) },
      { id: 'library', label: 'Library', icon: IconLibrary, active: route.name === 'library', run: () => go({ name: 'library' }) },
    ]
  return [
    { id: 'back', label: 'Close the project and go back to your projects', icon: IconBack, active: false, run: closeProject, back: true },
    { id: 'simulation', label: 'Simulation', icon: IconPlay, active: route.meta.view === 'simulation', run: () => showView('simulation') },
    { id: 'wiring', label: 'Wiring', icon: IconWiring, active: route.meta.view === 'wiring', run: () => showView('wiring') },
    { id: 'exercise', label: 'Exercise', icon: IconExercise, active: route.meta.view === 'exercise', run: () => showView('exercise') },
  ]
})
</script>

<template>
  <nav class="flex w-12 shrink-0 flex-col items-center gap-1 border-r border-line bg-s1 py-2" aria-label="Views">
    <template v-for="item in items" :key="item.id">
      <button
        class="relative flex size-9 cursor-default items-center justify-center rounded-control border-0 bg-transparent p-0 text-muted hover:bg-hover hover:text-fg disabled:opacity-40 disabled:hover:bg-transparent"
        :class="item.active ? 'bg-press text-fg' : ''"
        :aria-current="item.active ? 'page' : undefined"
        :aria-label="item.disabled ?? item.label"
        :title="item.disabled ?? item.label"
        :disabled="!!item.disabled"
        @click="item.run()"
      >
        <span v-if="item.active" class="absolute top-2 bottom-2 -left-1.5 w-0.5 rounded-full bg-accent" />
        <component :is="item.icon" class="size-5" />
      </button>
      <span v-if="item.back" class="my-1 h-px w-6 bg-line" />
    </template>
    <span class="flex-1" />
    <button
      class="relative flex size-9 cursor-default items-center justify-center rounded-control border-0 bg-transparent p-0 text-muted hover:bg-hover hover:text-fg"
      :class="route.name === 'settings' ? 'bg-press text-fg' : ''"
      :aria-current="route.name === 'settings' ? 'page' : undefined"
      aria-label="Settings"
      title="Settings"
      @click="toggleSettings"
    >
      <span v-if="route.name === 'settings'" class="absolute top-2 bottom-2 -left-1.5 w-0.5 rounded-full bg-accent" />
      <IconSettings class="size-5" />
      <span v-if="updateAvailable" class="absolute top-1.5 right-1.5 size-2 rounded-full bg-accent" title="An update is available" />
    </button>
  </nav>
</template>
