<script setup lang="ts">
import { computed, ref } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { edits } from '../builder'
import { openSettings, page, project, simulation, view } from '../sim'
import { update, updateAvailable } from '../update'

// The status bar, as a desktop tool has one: the app and its version on the left, then how
// the open project's station runs, and on the right where the project is kept. Everything in it shows a state; a click goes to where it is changed.
const { scene, snapshot } = simulation

const running = computed(() => page.value === 'simulation' && !!project.value && view.value === 'simulate')
const building = computed(() => page.value === 'simulation' && !!project.value && view.value === 'build')

const time = computed(() => {
  const total = snapshot.value?.time ?? 0
  const minutes = Math.floor(total / 60)
  const seconds = (total % 60).toFixed(1).padStart(4, '0')
  return `${String(minutes).padStart(2, '0')}:${seconds}`
})

const link = computed(() => snapshot.value?.link?.state ?? '')
const linkText: Record<string, string> = { connected: 'connected', failed: 'not connected', connecting: 'connecting' }

const problem = ref('')
async function reveal() {
  problem.value = ''
  try {
    if (project.value) await Service.RevealProject(project.value.name)
  } catch (e) {
    problem.value = (e as Error).message ?? String(e)
  }
}
</script>

<template>
  <footer class="status-bar">
    <button class="item app" :title="updateAvailable ? 'An update is available' : 'Updates'" @click="openSettings('updates')">
      <img src="/logo.svg" alt="" width="14" height="14" />
      Taktwin {{ update?.version ?? '' }}
      <span v-if="updateAvailable" class="dot" />
    </button>

    <template v-if="running">
      <span class="sep" />
      <button class="item" title="The PLC's connection: click to show it" @click="simulation.selection.value = scene?.plc.module ?? null">
        <span class="lamp" :class="link" />
        {{ snapshot?.plc ?? 'No PLC' }}<template v-if="link"> · {{ linkText[link] ?? link }}</template>
      </button>
      <span v-if="snapshot?.ioTest" class="item test">
        I/O test: outputs by hand
        <button @click="Service.SetIOTest(false)">End</button>
      </span>
      <span v-if="snapshot?.step" class="item mono" title="The step the PLC's program is in">
        <b>S{{ snapshot.step.number }}</b
        >&nbsp;{{ snapshot.step.label }}
      </span>
      <span class="item mono" title="Simulated time and speed">{{ time }} · {{ snapshot?.speed ?? 1 }}×{{ snapshot?.paused ? ' · paused' : '' }}</span>
    </template>
    <template v-else-if="building">
      <span class="sep" />
      <span class="item">Building{{ edits ? ` · ${edits} ${edits === 1 ? 'change' : 'changes'}` : '' }}</span>
    </template>

    <span class="spacer" />
    <span v-if="problem" class="item problem">{{ problem }}</span>
    <button v-if="project" class="item" :title="`Kept as it changes, in ${project.folder}: click to show the folder`" @click="reveal">✓ Saved</button>
  </footer>
</template>

<style scoped>
.status-bar {
  align-items: center;
  background: var(--panel);
  border-top: 1px solid var(--line);
  color: var(--text-dim);
  display: flex;
  font-size: 12px;
  gap: 2px;
  min-width: 0;
  overflow: hidden;
  padding: 0 6px;
  white-space: nowrap;
}

.item {
  align-items: center;
  background: none;
  border: 0;
  border-radius: 3px;
  color: inherit;
  display: flex;
  font-size: 12px;
  gap: 6px;
  height: 22px;
  padding: 0 7px;
}

button.item:hover {
  background: var(--panel-raised);
  color: var(--text);
}

.app {
  color: var(--text);
}

.dot {
  background: var(--accent);
  border-radius: 50%;
  height: 7px;
  width: 7px;
}

.sep {
  border-left: 1px solid var(--line);
  height: 14px;
  margin: 0 4px;
}

.spacer {
  flex: 1;
}

.lamp {
  background: var(--text-dim);
  border-radius: 50%;
  height: 8px;
  width: 8px;
}

.lamp.connecting {
  background: var(--accent);
}

.lamp.connected {
  background: #4ee67c;
}

.lamp.failed {
  background: var(--danger);
}

.test {
  background: var(--accent);
  color: #1b1405;
  font-weight: 600;
}

.test button {
  background: #1b1405;
  border: 0;
  color: var(--accent);
  font-size: 11px;
  padding: 0 6px;
}

b {
  color: var(--accent);
  font-weight: 600;
}

.problem {
  color: var(--danger);
}
</style>
