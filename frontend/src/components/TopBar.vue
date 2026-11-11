<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { LibraryService, Service as service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { grouped } from '../collections'
import { filesRead, loadStation, page, simulation } from '../sim'

const { scene, snapshot } = simulation

const speeds = [1, 2, 5]

const stations = ref<{ id: string; name: string; collection?: string }[]>([])
// Read again on each opening, so that a station saved in the user folder shows up.
const refresh = async () => {
  stations.value = ((await LibraryService.Entries()) ?? []).filter((e) => e.level === 'station' && !e.error)
}
onMounted(refresh)
watch(filesRead, refresh)
const choose = (event: Event) => void loadStation((event.target as HTMLSelectElement).value)

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
    <nav class="tabs" aria-label="Pages">
      <button :class="{ active: page === 'simulation' }" @click="page = 'simulation'">Simulation</button>
      <button :class="{ active: page === 'library' }" @click="page = 'library'">Library</button>
      <button :class="{ active: page === 'builder' }" @click="page = 'builder'">Builder</button>
      <button :class="{ active: page === 'settings' }" @click="page = 'settings'">Settings</button>
    </nav>
    <select v-if="page === 'simulation'" class="station" :value="scene?.id" aria-label="Station" @change="choose" @focus="refresh">
      <optgroup v-for="g in grouped(stations)" :key="g.id" :label="g.name">
        <option v-for="s in g.items" :key="s.id" :value="s.id">{{ s.name }}</option>
      </optgroup>
    </select>

    <div v-if="page === 'simulation'" class="status">
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

    <div v-if="page === 'simulation'" class="controls">
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

.station {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 2px;
  color: var(--text);
  padding: 3px 6px;
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
