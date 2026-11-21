<script setup lang="ts">
import { computed, ref } from 'vue'
import { Service as service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { edits } from '../builder'
import { closeProject, page, project, refreshProject, reloadStation, simulation, view } from '../sim'
import { updateAvailable } from '../update'

const { scene, snapshot } = simulation

const speeds = [1, 2, 5]

// The simulation's controls show while the open project's station runs.
const running = computed(() => page.value === 'simulation' && !!project.value && view.value === 'simulate')
const problem = ref('')

// Building is on a station of the project's own: a collection's station becomes a copy that
// the project keeps, after asking. An exercise's station stays as the exercise has it.
async function build() {
  const p = project.value
  if (!p || view.value === 'build') return
  problem.value = ''
  if (!p.own) {
    const ok = confirm(`Build on a copy of ${p.stationName} that belongs to this project? The project then no longer follows updates of the station's collection.`)
    if (!ok) return
    try {
      await service.CustomizeStation()
      await refreshProject()
    } catch (e) {
      problem.value = (e as Error).message ?? String(e)
      return
    }
  }
  view.value = 'build'
}

// Back from building, the simulation runs the station as it was built.
async function simulate() {
  if (view.value === 'simulate') return
  view.value = 'simulate'
  problem.value = ''
  if (!edits.value) return
  try {
    await reloadStation()
  } catch (e) {
    problem.value = `The station does not load: ${(e as Error).message ?? e}`
  }
}

// The gear shows the settings, and a second click goes back to where the app was.
let before: typeof page.value = 'simulation'
function toggleSettings() {
  if (page.value === 'settings') {
    page.value = before
    return
  }
  before = page.value
  page.value = 'settings'
}

const time = computed(() => {
  const total = snapshot.value?.time ?? 0
  const minutes = Math.floor(total / 60)
  const seconds = (total % 60).toFixed(1).padStart(4, '0')
  return `${String(minutes).padStart(2, '0')}:${seconds}`
})
const paused = computed(() => snapshot.value?.paused ?? false)
</script>

<template>
  <header class="bar">
    <img src="/logo.svg" alt="" width="26" height="26" />
    <strong class="name">Taktwin</strong>
    <nav v-if="!project" class="tabs" aria-label="Pages">
      <button :class="{ active: page === 'simulation' }" @click="page = 'simulation'">Projects</button>
      <button :class="{ active: page === 'library' }" @click="page = 'library'">Library</button>
    </nav>
    <template v-else>
      <button class="back" title="Close the project and go back to your projects" @click="closeProject">‹ Projects</button>
      <span class="project" :title="project.folder">
        {{ project.name }}
        <small>{{ project.exerciseName ? `Exercise: ${project.exerciseName}` : project.stationName }}</small>
      </span>
      <div v-if="page === 'simulation'" class="segmented view" role="group" aria-label="View">
        <button :class="{ active: view === 'simulate' }" :aria-pressed="view === 'simulate'" @click="simulate">Simulate</button>
        <button
          :class="{ active: view === 'build' }"
          :aria-pressed="view === 'build'"
          :disabled="!!project.exercise"
          :title="project.exercise ? 'The station of an exercise stays as the exercise has it' : 'Build on the station'"
          @click="build"
        >
          Build
        </button>
      </div>
    </template>
    <span v-if="problem" class="collision">{{ problem }}</span>

    <div v-if="running" class="status">
      <button class="source" title="PLC connection" @click="simulation.selection.value = scene?.plc.module ?? null">
        <span v-if="snapshot?.link" class="lamp" :class="snapshot.link.state" />{{ snapshot?.plc ?? 'PLC' }}
      </button>
      <span v-if="snapshot?.ioTest" class="test">
        I/O test: outputs by hand
        <button @click="service.SetIOTest(false)">End</button>
      </span>
      <span v-for="text in snapshot?.collisions ?? []" :key="text" class="collision">{{ text }}</span>
      <span v-if="snapshot?.fellOff" class="collision fell">
        {{ snapshot.fellOff === 1 ? 'A workpiece' : `${snapshot.fellOff} workpieces` }} fell off the station
        <button @click="service.AcknowledgeFellOff()">OK</button>
      </span>
      <span v-if="snapshot?.step" class="step mono">
        <b>S{{ snapshot.step.number }}</b> {{ snapshot.step.label }}
      </span>
    </div>

    <div v-if="running" class="controls">
      <span class="time mono" title="Simulated time">{{ time }}</span>
      <div class="segmented" role="group" aria-label="Simulation speed">
        <button
          v-for="speed in speeds"
          :key="speed"
          :class="{ active: snapshot?.speed === speed }"
          :disabled="!!snapshot?.link && speed !== 1"
          :title="snapshot?.link && speed !== 1 ? 'A PLC over OPC UA runs in real time' : undefined"
          @click="service.SetSpeed(speed)"
        >
          {{ speed }}×
        </button>
      </div>
      <button class="fixed" @click="service.SetPaused(!paused)">{{ paused ? 'Resume' : 'Pause' }}</button>
      <button @click="service.Reset()">Restart</button>
    </div>
    <span v-if="!running" class="spacer" />
    <button class="gear" :class="{ active: page === 'settings' }" :aria-pressed="page === 'settings'" title="Settings" aria-label="Settings" @click="toggleSettings">
      <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
        <path
          fill="currentColor"
          d="M19.4 13a7.6 7.6 0 0 0 0-2l2.1-1.6-2-3.5-2.5 1a7.4 7.4 0 0 0-1.7-1L15 3.3h-4l-.4 2.6a7.4 7.4 0 0 0-1.7 1l-2.5-1-2 3.5L6.6 11a7.6 7.6 0 0 0 0 2l-2.1 1.6 2 3.5 2.5-1a7.4 7.4 0 0 0 1.7 1l.4 2.6h4l.4-2.6a7.4 7.4 0 0 0 1.7-1l2.5 1 2-3.5zM13 15.5a3.5 3.5 0 1 1 0-7 3.5 3.5 0 0 1 0 7z"
          transform="translate(-1 0)"
        />
      </svg>
      <span v-if="updateAvailable" class="dot" title="An update is available"></span>
    </button>
  </header>
</template>

<style scoped>
.bar {
  align-items: center;
  background: var(--panel);
  border-bottom: 1px solid var(--line);
  display: flex;
  gap: 10px;
  grid-column: 1 / -1;
  padding: 0 12px;
}

img {
  border-radius: 2px;
}

.name {
  font-size: 14px;
}

.back {
  padding: 3px 8px;
}

.spacer {
  flex: 1;
}

.view button {
  min-width: 76px;
}

.gear {
  align-items: center;
  background: none;
  border-color: transparent;
  color: var(--text-dim);
  display: flex;
  padding: 4px 6px;
  position: relative;
}

.gear:hover,
.gear.active {
  background: var(--panel-raised);
  border-color: var(--line);
  color: var(--text);
}

.gear .dot {
  position: absolute;
  right: 1px;
  top: 1px;
}

.project {
  display: flex;
  flex-direction: column;
  line-height: 1.15;
  max-width: 260px;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.project small {
  color: var(--text-dim);
  font-size: 11px;
}

.status {
  align-items: center;
  display: flex;
  flex: 1;
  gap: 10px;
  justify-content: center;
  min-width: 0;
  white-space: nowrap;
}

.source {
  align-items: center;
  background: none;
  border: 1px solid var(--line);
  border-radius: 2px;
  color: var(--text-dim);
  display: flex;
  font-size: 12px;
  gap: 6px;
  padding: 2px 10px;
}

.test {
  align-items: center;
  background: var(--accent);
  color: #1b1405;
  display: flex;
  font-size: 12px;
  font-weight: 600;
  gap: 8px;
  padding: 2px 3px 2px 8px;
}

.collision {
  background: var(--red);
  color: #fff;
  font-size: 12px;
  font-weight: 600;
  padding: 2px 8px;
  white-space: nowrap;
}

.fell {
  align-items: center;
  display: flex;
  gap: 8px;
  padding: 2px 3px 2px 8px;
}

.fell button {
  background: #fff;
  border: 0;
  color: var(--red);
  font-size: 11px;
  font-weight: 700;
  padding: 1px 7px;
}

.test button {
  background: #1b1405;
  border: 0;
  color: var(--accent);
  font-size: 11px;
  padding: 1px 7px;
}

.lamp {
  background: var(--accent);
  border-radius: 50%;
  height: 8px;
  width: 8px;
}

.lamp.connected {
  background: #4ee67c;
}

.lamp.failed {
  background: var(--danger);
}

.step b {
  color: var(--accent);
  font-weight: 600;
}

.controls {
  align-items: center;
  display: flex;
  gap: 8px;
}

.tabs {
  align-self: stretch;
  border-left: 1px solid var(--line);
  display: flex;
  margin-left: 6px;
}

.tabs button {
  background: none;
  border: 0;
  border-radius: 0;
  border-right: 1px solid var(--line);
  color: var(--text-dim);
  letter-spacing: 0.02em;
  padding: 0 16px;
}

.tabs button:hover:not(.active) {
  background: var(--panel-raised);
  color: var(--text);
}

.gear .dot {
  background: var(--accent);
  border-radius: 50%;
  height: 7px;
  width: 7px;
}

.tabs .active {
  background: var(--accent);
  color: #1b1405;
  font-weight: 600;
}

.time {
  color: var(--text-dim);
  margin-right: 4px;
}

.fixed {
  min-width: 68px;
}

.segmented {
  display: flex;
}

.segmented button {
  border-radius: 0;
  margin-left: -1px;
}

.segmented button:first-child {
  border-radius: 2px 0 0 6px;
  margin-left: 0;
}

.segmented button:last-child {
  border-radius: 0 2px 2px 0;
}

.segmented .active {
  background: var(--accent);
  border-color: var(--accent);
  color: #1b1405;
  position: relative;
}
</style>
