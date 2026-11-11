<script setup lang="ts">
import { computed } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { simulation } from '../sim'

// Compact is for the panel of a module, where the pins are what matters.

const props = defineProps<{ compact?: boolean; plc?: string }>()

const { plcId, snapshot } = simulation

// The PLC it tests: the one given, as that of a module's pins, else the one the panel shows.
const id = computed(() => props.plc ?? plcId.value)
const view = computed(() => snapshot.value?.plcs?.find((p, i) => (id.value ? p.id === id.value : i === 0)) ?? null)
const testing = computed(() => !!view.value?.ioTest)
const external = computed(() => !!view.value?.link && !view.value?.step && !testing.value)
const setTest = (on: boolean) => void Service.SetIOTestFor(id.value, on)
const clear = () => void Service.ClearTestOutputsFor(id.value)
</script>

<template>
  <section v-if="compact" class="section compact">
    <h3 class="section-title">I/O test</h3>
    <div class="actions">
      <template v-if="testing">
        <button @click="clear()">All outputs off</button>
        <button class="stop" @click="setTest(false)">End</button>
      </template>
      <button v-else class="start" @click="setTest(true)">Start</button>
    </div>
    <p v-if="external" class="help note">The PLC keeps running on its own during a test, with its inputs frozen.</p>
  </section>
  <section v-else class="section">
    <h3 class="section-title">I/O test</h3>
    <template v-if="testing">
      <p class="help">
        The program is cut off. Switch outputs below or on a module's pins, and inputs show live. To teach a sensor, run a workpiece to it and change its threshold next to its
        input.
      </p>
      <div class="actions">
        <button class="stop" @click="setTest(false)">End I/O test</button>
        <button @click="clear()">All outputs off</button>
      </div>
    </template>
    <template v-else>
      <p class="help">Test sensors and actuators by hand, without the program interfering. Workpieces stay where they are, and all outputs start off.</p>
      <p v-if="external" class="help note">The PLC keeps running on its own during a test, with its inputs frozen. Restart its program afterwards.</p>
      <button class="start" @click="setTest(true)">Start I/O test</button>
    </template>
  </section>
</template>

<style scoped>
.compact {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: 6px 10px;
  justify-content: space-between;
}

.compact .section-title {
  margin: 0;
}

.compact .help {
  margin: 0;
  width: 100%;
}

.compact .start,
.compact .stop {
  flex: none;
  width: auto;
}

.help {
  color: var(--text-dim);
  margin: 0 0 8px;
}

.note {
  color: var(--accent);
}

.actions {
  display: flex;
  gap: 6px;
}

.start,
.stop {
  flex: 1;
  width: 100%;
}

.stop {
  background: var(--accent);
  border-color: var(--accent);
  color: #1b1405;
  font-weight: 600;
}
</style>
