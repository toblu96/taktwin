<script setup lang="ts">
import UiSegmented from '../../ui/UiSegmented.vue'
import UiTabs from '../../ui/UiTabs.vue'
import UiIoList from '../../ui/UiIoList.vue'
import { ref } from 'vue'
import { checks, modules, signals } from '../data'
import ActivityBar from './ActivityBar.vue'
import StatusBar from './StatusBar.vue'
import IconBack from '~icons/fluent/arrow-left-20-regular'
import IconPlay from '~icons/fluent/play-20-regular'
import IconWrench from '~icons/fluent/wrench-20-regular'
import IconCode from '~icons/fluent/code-20-regular'
import IconPlug from '~icons/fluent/plug-connected-20-regular'
import IconTask from '~icons/fluent/clipboard-task-20-regular'
import IconPause from '~icons/fluent/pause-20-filled'
import IconRestart from '~icons/fluent/arrow-counterclockwise-20-regular'
import IconChevron from '~icons/fluent/chevron-right-16-regular'
import IconCube from '~icons/fluent/cube-16-regular'
import IconWarning from '~icons/fluent/warning-16-filled'
import IconPassed from '~icons/fluent/checkmark-circle-16-filled'
import IconFailed from '~icons/fluent/dismiss-circle-16-filled'
import IconPanel from '~icons/fluent/panel-bottom-20-regular'

// A project open in the new look: what you do with it in the activity bar, the station in the
// middle, its parts at the side, and what runs and how a check went in a panel below. The top
// bar keeps only the project and how the simulation runs; warnings that need an answer sit on
// the 3D view.
const view = ref('simulate')
const speed = ref('1')
const panel = ref('check')
const order = ref(signals.map((s) => s.key))
const open = ref<Record<string, boolean>>({ conveyor: true })
const selected = ref('B2 Pallet at the stop')
</script>

<template>
  <div class="flex h-full flex-col">
    <div class="flex min-h-0 flex-1">
      <ActivityBar
        v-model="view"
        :items="[
          { id: 'projects', label: 'Back to your projects', icon: IconBack, back: true },
          { id: 'simulate', label: 'Simulate', icon: IconPlay },
          { id: 'build', label: 'Build', icon: IconWrench },
          { id: 'program', label: 'Program', icon: IconCode },
          { id: 'io', label: 'I/O and connection', icon: IconPlug },
          { id: 'exercise', label: 'Exercise', icon: IconTask },
        ]"
      />

      <aside class="flex w-64 shrink-0 flex-col border-r border-line bg-s1">
        <div class="flex h-10 items-center px-3 font-semibold">Station</div>
        <ul class="min-h-0 flex-1 overflow-auto px-1.5 pb-2" role="tree" aria-label="Modules of the station">
          <li v-for="m in modules" :key="m.id" role="treeitem" :aria-expanded="m.children.length ? !!open[m.id] : undefined">
            <button class="flex h-7 w-full items-center gap-1 rounded-[5px] pr-2 pl-1 text-left hover:bg-hover" @click="open[m.id] = !open[m.id]">
              <IconChevron class="size-3.5 text-faint transition-transform" :class="[open[m.id] ? 'rotate-90' : '', m.children.length ? '' : 'invisible']" />
              <IconCube class="size-4 text-muted" />
              <span class="truncate">{{ m.name }}</span>
            </button>
            <ul v-if="open[m.id]" role="group">
              <li v-for="c in m.children" :key="c" role="treeitem" :aria-selected="selected === c">
                <button
                  class="flex h-7 w-full items-center rounded-[5px] pr-2 pl-10.5 text-left text-muted hover:bg-hover"
                  :class="selected === c ? 'bg-select text-fg' : ''"
                  @click="selected = c"
                >
                  <span class="truncate">{{ c }}</span>
                </button>
              </li>
            </ul>
          </li>
        </ul>
        <section class="border-t border-line p-3">
          <h3 class="mb-2 font-semibold">PLC</h3>
          <div class="flex items-center gap-2 rounded-control border border-line bg-s0 px-2.5 py-2">
            <span class="size-2 rounded-full bg-ok" />
            <span class="flex-1">
              <span class="block">Soft PLC</span>
              <span class="text-xs text-muted">Running · step 30</span>
            </span>
            <button class="t-btn">Program</button>
          </div>
        </section>
      </aside>

      <div class="flex min-w-0 flex-1 flex-col">
        <header class="flex h-10 shrink-0 items-center gap-2 border-b border-line bg-s1 px-3">
          <span class="text-muted">Projects</span>
          <IconChevron class="size-3.5 text-faint" />
          <span class="font-semibold">Load a pallet</span>
          <span class="rounded-sm bg-accent-soft px-1.5 text-[11px] leading-4.5 font-medium text-accent-ink">Exercise</span>
          <span class="flex-1" />
          <span class="font-mono text-xs text-muted">00:42.3</span>
          <UiSegmented
            v-model="speed"
            label="Speed"
            :options="[
              { value: '1', label: '1×' },
              { value: '2', label: '2×' },
              { value: '5', label: '5×' },
            ]"
          />
          <button class="t-icon-btn" title="Pause"><IconPause class="size-4.5" /></button>
          <button class="t-icon-btn" title="Restart"><IconRestart class="size-4.5" /></button>
        </header>

        <main class="relative min-h-0 flex-1 overflow-hidden bg-[#d7dedf]">
          <img src="/lab/canvas.jpg" alt="The station in 3D" class="size-full object-cover" />
          <div class="absolute top-3 left-1/2 flex -translate-x-1/2 items-center gap-2 rounded-card border border-danger/40 bg-s2 py-1.5 pr-1.5 pl-3 shadow-pop" role="alert">
            <IconWarning class="size-4 text-danger" />
            <span>The gripper hit a carton on the belt.</span>
            <button class="t-btn h-6 px-2 text-xs">Acknowledge</button>
          </div>
        </main>

        <section class="flex h-60 shrink-0 flex-col border-t border-line bg-s0">
          <UiTabs
            v-model="panel"
            label="Panel"
            :tabs="[
              { value: 'check', label: 'Check', count: 5 },
              { value: 'io', label: 'Inputs' },
              { value: 'output', label: 'Output' },
            ]"
          >
            <template #check>
              <ol class="flex flex-col gap-1">
                <li v-for="(c, i) in checks" :key="i" class="flex items-center gap-2">
                  <component :is="c.ok ? IconPassed : IconFailed" class="size-4" :class="c.ok ? 'text-ok' : 'text-danger'" />
                  <span :class="c.ok ? 'text-muted' : ''">{{ c.line }}</span>
                </li>
              </ol>
            </template>
            <template #io>
              <UiIoList v-model="order" :signals="signals" class="max-w-2xl" />
            </template>
            <template #output><p class="font-mono text-xs text-muted">Downloaded to the soft PLC · 3 POUs · built in 42 ms</p></template>
          </UiTabs>
        </section>
      </div>
    </div>
    <StatusBar>
      <span class="mx-1 h-3 w-px bg-line" />
      <span class="flex items-center gap-1.5 px-1.5"><span class="size-1.5 rounded-full bg-ok" />Soft PLC connected</span>
      <span class="flex-1" />
      <span class="flex items-center gap-1 px-1.5"><IconPanel class="size-3.5" />Panel</span>
      <span class="px-1.5">Saved</span>
    </StatusBar>
  </div>
</template>
