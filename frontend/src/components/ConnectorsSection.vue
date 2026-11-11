<script setup lang="ts">
import { computed } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { simulation, useSim } from '../sim'
import PlcTest from './PlcTest.vue'
import type { SceneConnector, ScenePin } from '../../bindings/github.com/toblu96/taktwin/internal/model'

// The pins of a module with their live state and what is behind them.
const props = defineProps<{ connectors: SceneConnector[] }>()

const sim = useSim()
const { service, snapshot, stateOn } = sim

// A pin that a PLC output drives, at its address in the PLC's image.
const plcOutput = (pin: ScenePin) => pin.output && !!pin.address

// Whoever is looking at a PLC output's pin can start the I/O test right here.
const hasOutputs = computed(() => props.connectors.some((c) => c.pins?.some(plcOutput)))
// The PLC its outputs are of, where the station has several.
const plc = computed(() => props.connectors.flatMap((c) => c.pins ?? []).find(plcOutput)?.plc ?? '')
const plcOf = (pin: ScenePin) => snapshot.value?.plcs?.find((p, i) => (pin.plc ? p.id === pin.plc : i === 0))

// In a preview any free signal can be switched, and in an I/O test any PLC output.
const testOutput = (pin: ScenePin) => sim === simulation && !!plcOf(pin)?.ioTest && plcOutput(pin)
const canSwitch = (pin: ScenePin) => (pin.forcible && !!service.SetPin) || testOutput(pin)

function switchPin(pin: ScenePin) {
  const on = !stateOn(pin.state)
  if (testOutput(pin)) void Service.SetTestOutputFor(pin.plc ?? '', pin.address ?? '', on)
  else void service.SetPin?.(pin.state, on)
}
</script>

<template>
  <PlcTest v-if="sim === simulation && hasOutputs" compact :plc="plc" />
  <section v-for="connector in connectors" :key="connector.id" class="section">
    <h3 class="section-title">
      {{ connector.type === 'wires' ? 'Signals' : connector.id }} <span class="kind">{{ connector.type === 'wires' ? '' : connector.label }}</span>
    </h3>
    <table>
      <tbody>
        <tr v-for="pin in connector.pins" :key="pin.pin" :class="{ unused: !pin.wired }">
          <td class="dot-cell">
            <span v-if="pin.analog" class="wave" title="Analogue signal">~</span>
            <button
              v-else-if="canSwitch(pin)"
              class="force"
              :aria-pressed="stateOn(pin.state)"
              :title="`Switch ${pin.pin} ${stateOn(pin.state) ? 'off' : 'on'}`"
              @click="switchPin(pin)"
            >
              <span class="dot" :class="[pin.output ? 'out' : 'in', { on: stateOn(pin.state) }]" />
            </button>
            <span v-else class="dot" :class="[pin.output ? 'out' : 'in', { on: stateOn(pin.state) }]" />
          </td>
          <td v-if="connector.type !== 'wires'" class="pin mono">{{ pin.pin }}</td>
          <td class="function">
            <template v-if="pin.function"
              >{{ pin.function }}<small v-if="pin.source"><br />{{ pin.source }}</small></template
            >
            <small v-else>{{ pin.spare ? 'Spare' : 'Not used' }}</small>
          </td>
          <td class="wire mono" :title="pin.invert ? 'Inverted at the source' : ''">
            <span v-if="pin.analog && pin.wired" class="volts">{{ (snapshot?.states?.[pin.state] ?? 0).toFixed(2) }} V</span>
            <template v-if="pin.plcName"
              >{{ pin.invert ? '¬' : '' }}{{ pin.plcName }}<br /><small>{{ pin.address }}</small></template
            >
            <small v-else-if="pin.wired && !pin.address && !service.SetPin && !pin.analog">not wired to PLC</small>
          </td>
        </tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
.kind {
  font-weight: 400;
  letter-spacing: 0;
  margin-left: 4px;
  text-transform: none;
}

table {
  border-collapse: collapse;
  width: 100%;
}

td {
  border-bottom: 1px solid #1a3a3f;
  padding: 3px 6px 3px 0;
}

.unused {
  opacity: 0.55;
}

.dot-cell {
  width: 18px;
}

.volts {
  color: var(--signal);
  margin-right: 8px;
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

.dot {
  background: #24464b;
  border-radius: 50%;
  display: block;
  height: 9px;
  width: 9px;
}

.wave {
  color: var(--text-dim);
  display: block;
  font-size: 13px;
  line-height: 9px;
}

.dot.in.on {
  background: #4ee67c;
  box-shadow: 0 0 6px #4ee67c;
}

.dot.out.on {
  background: #ff9d2e;
  box-shadow: 0 0 6px #ff9d2e;
}

.pin {
  font-size: 12px;
  font-weight: 600;
  width: 34px;
}

.wire {
  font-size: 12px;
  line-height: 1.25;
  padding-right: 0;
  text-align: right;
  white-space: nowrap;
}

small {
  color: var(--text-dim);
  font-size: 11px;
}
</style>
