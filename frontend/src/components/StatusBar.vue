<script setup lang="ts">
import { computed, ref } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { edits } from '../builder'
import { openSettings, page, project, simulation, view } from '../sim'
import { update, updateAvailable } from '../update'
import IconSaved from '~icons/fluent/checkmark-16-regular'

// The status bar, quiet as VS Code's is now: the app and its version on the left, then how the
// open project's station runs, and on the right where the project is kept. Each item shows a
// state, and a click goes to where it is changed.
const { scene, snapshot } = simulation

const running = computed(() => page.value === 'simulation' && !!project.value && view.value === 'simulate')
const building = computed(() => page.value === 'simulation' && !!project.value && view.value === 'build')

const link = computed(() => snapshot.value?.link?.state ?? '')
const linkText: Record<string, string> = { connected: 'connected', failed: 'not connected', connecting: 'connecting' }
const lamp: Record<string, string> = { connected: 'bg-ok', failed: 'bg-danger', connecting: 'bg-accent' }

const problem = ref('')
async function reveal() {
  problem.value = ''
  try {
    if (project.value) await Service.RevealProject(project.value.name)
  } catch (e) {
    problem.value = (e as Error).message ?? String(e)
  }
}

const item = 'flex h-6 shrink-0 cursor-default items-center gap-1.5 rounded-sm border-0 bg-transparent px-1.5 text-xs text-muted'
const button = `${item} hover:bg-hover hover:text-fg`
</script>

<template>
  <footer class="flex h-6 min-w-0 shrink-0 items-center gap-0.5 overflow-hidden border-t border-line bg-s1 px-1.5 whitespace-nowrap">
    <button :class="button" :title="updateAvailable ? 'An update is available' : 'Updates'" @click="openSettings('updates')">
      <img src="/logo.svg" alt="" class="size-3.5" />
      Taktwin {{ update?.version ?? '' }}
      <span v-if="updateAvailable" class="size-1.5 rounded-full bg-accent" />
    </button>

    <template v-if="running">
      <span class="mx-1 h-3 w-px bg-line" />
      <button :class="button" title="The PLC's connection: click to show it" @click="simulation.selection.value = scene?.plc.module ?? null">
        <span class="size-1.5 rounded-full" :class="lamp[link] ?? 'bg-faint'" />
        {{ snapshot?.plc ?? 'No PLC' }}<template v-if="link"> · {{ linkText[link] ?? link }}</template>
      </button>
      <span v-if="snapshot?.ioTest" :class="item" class="bg-accent font-medium text-on-accent">
        I/O test: outputs by hand
        <button class="t-btn h-4.5 border-0 bg-on-accent px-1.5 text-[11px] text-accent hover:bg-on-accent" @click="Service.SetIOTest(false)">End</button>
      </span>
      <span v-if="snapshot?.step" :class="item" class="font-mono" title="The step the PLC's program is in">
        <b class="font-semibold text-accent-ink">S{{ snapshot.step.number }}</b>{{ snapshot.step.label }}
      </span>
    </template>
    <template v-else-if="building">
      <span class="mx-1 h-3 w-px bg-line" />
      <span :class="item">Building{{ edits ? ` · ${edits} ${edits === 1 ? 'change' : 'changes'}` : '' }}</span>
    </template>

    <span class="flex-1" />
    <span v-if="problem" :class="item" class="text-danger">{{ problem }}</span>
    <button v-if="project" :class="button" :title="`Kept as it changes, in ${project.folder}: click to show the folder`" @click="reveal">
      <IconSaved class="size-3.5" />Saved
    </button>
  </footer>
</template>
