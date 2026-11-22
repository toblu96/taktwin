<script setup lang="ts">
import { computed } from 'vue'
import { ProgramService } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { programming, simulation, useSim } from '../sim'
import IoTable from './IoTable.vue'
import PlcConnection from './PlcConnection.vue'
import PlcTest from './PlcTest.vue'

// A PLC: its program and connection in the simulation, and its signals. In an open project the
// panel below the 3D view shows the signals and their test, so the PLC's own view leaves them out.
withDefaults(defineProps<{ signals?: boolean }>(), { signals: true })

const sim = useSim()
const { plcId, plcScene, plcView, scene } = sim
const hasSignals = computed(() => {
  const p = plcScene.value
  return !!(p?.inputs?.length || p?.outputs?.length || p?.analog?.length || p?.analogOutputs?.length)
})
</script>

<template>
  <section v-if="sim === simulation" class="section">
    <h3 class="section-title">Program</h3>
    <p class="help">Write this PLC's program in Structured Text, run it in the app's soft PLC, and watch and force its variables.</p>
    <div class="program-actions">
      <button class="start" @click="programming = plcId">Edit program</button>
      <button title="Beside the simulation, in a window of its own" @click="ProgramService.OpenWindow(plcId)">Open in window</button>
    </div>
  </section>
  <PlcConnection v-if="sim === simulation" :key="`${scene?.id}:${plcId}`" />
  <PlcTest v-if="sim === simulation && signals" />
  <section class="section">
    <p class="source">
      <span class="section-title">Program</span>
      <span>{{ plcView?.plc }}</span>
    </p>
    <p v-if="plcView?.step" class="source">
      <span class="section-title">Step</span>
      <span class="mono"><b>S{{ plcView.step.number }}</b> {{ plcView.step.label }}</span>
    </p>
  </section>
  <section v-if="signals && hasSignals" class="section">
    <IoTable />
  </section>
</template>

<style scoped>
.help {
  color: var(--text-dim);
  margin: 0 0 8px;
}
.program-actions {
  display: flex;
  gap: 6px;
}
.program-actions .start {
  background: var(--accent);
  border-color: var(--accent);
  color: #1b1405;
  font-weight: 600;
}
.source {
  align-items: baseline;
  display: flex;
  justify-content: space-between;
  margin: 0 0 6px;
}
.source .section-title {
  margin: 0;
}
b {
  color: var(--accent);
  font-weight: 600;
}
</style>
