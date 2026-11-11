<script setup lang="ts">
import { computed } from 'vue'
import { ProgramService, Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { bit, programming, simulation, useSim } from '../sim'
import PlcConnection from './PlcConnection.vue'
import PlcTest from './PlcTest.vue'

const sim = useSim()
const { parameterValue, plcId, plcScene, plcView, scene } = sim

const testing = computed(() => sim === simulation && !!plcView.value?.ioTest)
const switchOutput = (variable: string, on: boolean) => void Service.SetTestOutputFor(plcId.value, variable, on)

// The module behind an input, to teach its sensor right next to its live signal.
// Pins name their PLC only where the station has several.
const pinPlc = computed(() => ((scene.value?.plcs?.length ?? 0) > 1 ? (plcScene.value?.module ?? '') : ''))
const instanceOf = (address: string) =>
  scene.value?.instances?.find((i) => i.connectors?.some((c) => c.type === 'wires' && c.pins?.some((p) => p.address === address && (p.plc ?? '') === pinPlc.value)))
const settingOf = (address: string) => {
  const id = instanceOf(address)?.id
  return id ? scene.value?.parameters?.find((p) => p.module === id) : undefined
}
const ownerOf = (address: string) => instanceOf(address)?.name

function changeSetting(address: string, event: Event) {
  const setting = settingOf(address)
  const value = Number((event.target as HTMLInputElement).value)
  if (setting && value >= setting.min && value <= setting.max) void sim.setParameter(setting.id, value)
}

const groups = computed(() => [
  { title: 'Inputs', list: plcScene.value?.inputs ?? [], image: plcView.value?.inputs, outputs: false },
  { title: 'Outputs', list: plcScene.value?.outputs ?? [], image: plcView.value?.outputs, outputs: true },
])
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
  <PlcTest v-if="sim === simulation" />
  <section class="section">
    <p class="source">
      <span class="section-title">Program</span>
      <span>{{ plcView?.plc }}</span>
    </p>
    <p v-if="plcView?.step" class="source">
      <span class="section-title">Step</span>
      <span class="mono"
        ><b>S{{ plcView.step.number }}</b> {{ plcView.step.label }}</span
      >
    </p>
  </section>
  <section v-for="group in groups" :key="group.title" class="section">
    <h3 class="section-title">{{ group.title }}</h3>
    <table>
      <tbody>
        <tr v-for="signal in group.list" :key="signal.address" :title="signal.description">
          <td class="dot-cell">
            <button
              v-if="testing && group.outputs"
              class="force"
              :aria-pressed="bit(group.image, signal.byte, signal.bit)"
              :title="`Switch ${signal.name} ${bit(group.image, signal.byte, signal.bit) ? 'off' : 'on'}`"
              @click="switchOutput(signal.address, !bit(group.image, signal.byte, signal.bit))"
            >
              <span class="dot" :class="{ on: bit(group.image, signal.byte, signal.bit) }" />
            </button>
            <span v-else class="dot" :class="{ on: bit(group.image, signal.byte, signal.bit) }" />
          </td>
          <td class="name">{{ signal.name }}</td>
          <template v-if="testing">
            <td class="what">{{ signal.description }}</td>
            <td class="setting">
              <label v-if="!group.outputs && settingOf(signal.address)" :title="`${settingOf(signal.address)?.label} of ${ownerOf(signal.address)}`">
                <input
                  type="number"
                  :min="settingOf(signal.address)?.min"
                  :max="settingOf(signal.address)?.max"
                  :step="settingOf(signal.address)?.step || 1"
                  :value="parameterValue(settingOf(signal.address)?.id ?? '', settingOf(signal.address)?.value ?? 0)"
                  :aria-label="`${settingOf(signal.address)?.label} of ${signal.name}`"
                  @change="changeSetting(signal.address, $event)"
                />{{ settingOf(signal.address)?.unit }}
              </label>
            </td>
          </template>
          <template v-else>
            <td class="mono variable">{{ signal.variable }}</td>
            <td class="mono address">{{ signal.address }}</td>
          </template>
        </tr>
      </tbody>
    </table>
  </section>
  <section v-if="plcScene?.analog?.length" class="section">
    <h3 class="section-title">Analogue inputs</h3>
    <table>
      <tbody>
        <tr v-for="signal in plcScene.analog" :key="signal.address" :title="signal.description">
          <td class="dot-cell volts mono">{{ (plcView?.analog?.[signal.channel] ?? 0).toFixed(2) }} V</td>
          <td class="name">{{ signal.name }}</td>
          <td class="mono address">{{ signal.address }}</td>
        </tr>
      </tbody>
    </table>
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

.volts {
  color: var(--signal);
  white-space: nowrap;
}

.force {
  background: none;
  border: 1px solid var(--line);
  border-radius: 50%;
  display: grid;
  height: 17px;
  margin-left: -4px;
  padding: 0;
  place-items: center;
  width: 17px;
}

.force:hover {
  background: var(--panel-raised);
  border-color: var(--text-dim);
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

table {
  border-collapse: collapse;
  width: 100%;
}

td {
  border-bottom: 1px solid #1a3a3f;
  padding: 2px 6px 2px 0;
}

.dot-cell {
  width: 18px;
}

.dot {
  background: #24464b;
  border-radius: 50%;
  display: block;
  height: 9px;
  width: 9px;
}

.dot.on {
  background: var(--signal);
  box-shadow: 0 0 6px var(--signal);
}

.name {
  font-weight: 600;
  width: 66px;
}

.what {
  color: var(--text-dim);
  font-size: 12px;
}

.setting {
  padding-right: 0;
  text-align: right;
  white-space: nowrap;
}

.setting label {
  color: var(--text-dim);
  font-size: 11px;
}

.setting input {
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: 2px;
  color: var(--text);
  margin-right: 3px;
  padding: 1px 3px;
  text-align: right;
  width: 42px;
}

.variable {
  font-size: 12px;
}

.address {
  color: var(--text-dim);
  font-size: 11px;
  padding-right: 0;
  text-align: right;
}
</style>
