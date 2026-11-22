<script setup lang="ts">
import { computed } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { bit, simulation, useSim } from '../sim'

// The signals of the PLC that is shown, live: a lamp for each digital input and output, the
// value of each analogue one. During an I/O test the outputs are switched here by hand, and a
// sensor's threshold sits next to its input, to teach it with a workpiece in front of it. Wide,
// as in the panel below the 3D view, inputs and outputs stand side by side with what each is
// for and the module it belongs to.
defineProps<{ wide?: boolean }>()

const sim = useSim()
const { parameterValue, plcId, plcScene, plcView, scene } = sim

const testing = computed(() => sim === simulation && !!plcView.value?.ioTest)
const switchOutput = (variable: string, on: boolean) => void Service.SetTestOutputFor(plcId.value, variable, on)

// The module behind a signal. Pins name their PLC only where the station has several.
const pinPlc = computed(() => ((scene.value?.plcs?.length ?? 0) > 1 ? (plcScene.value?.module ?? '') : ''))
const instanceOf = (address: string) =>
  scene.value?.instances?.find((i) => i.connectors?.some((c) => c.type === 'wires' && c.pins?.some((p) => p.address === address && (p.plc ?? '') === pinPlc.value)))
const settingOf = (address: string) => {
  const id = instanceOf(address)?.id
  return id ? scene.value?.parameters?.find((p) => p.module === id) : undefined
}

function changeSetting(address: string, event: Event) {
  const setting = settingOf(address)
  const value = Number((event.target as HTMLInputElement).value)
  if (setting && value >= setting.min && value <= setting.max) void sim.setParameter(setting.id, value)
}

// An analogue output set by hand during the I/O test, within its range.
function setAnalog(address: string, range: number[], event: Event) {
  const value = Number((event.target as HTMLInputElement).value)
  if (Number.isFinite(value)) void Service.SetTestAnalogFor(plcId.value, address, Math.min(range[1], Math.max(range[0], value)))
}

const groups = computed(() =>
  [
    { title: 'Inputs', list: plcScene.value?.inputs ?? [], image: plcView.value?.inputs, outputs: false },
    { title: 'Outputs', list: plcScene.value?.outputs ?? [], image: plcView.value?.outputs, outputs: true },
  ].filter((g) => g.list.length),
)
const analogGroups = computed(() =>
  [
    { title: 'Analogue inputs', list: plcScene.value?.analog ?? [], outputs: false },
    { title: 'Analogue outputs', list: plcScene.value?.analogOutputs ?? [], outputs: true },
  ].filter((g) => g.list.length),
)

const lamp = (on: boolean) => (on ? 'bg-ok shadow-[0_0_6px_var(--t-ok)]' : 'bg-s3 shadow-[inset_0_0_0_1px_var(--t-control)]')
const cell = 'border-0 border-b border-solid border-line py-1 pr-3 align-middle'
const number = 't-input h-6 w-16 px-1.5 text-right font-mono text-xs'
</script>

<template>
  <div class="grid gap-x-6 gap-y-4" :class="wide ? 'grid-cols-[repeat(auto-fit,minmax(460px,1fr))]' : ''">
    <section v-for="group in groups" :key="group.title" class="min-w-0">
      <h3 class="m-0 mb-1 text-xs font-semibold text-muted">{{ group.title }}</h3>
      <table class="w-full table-fixed border-collapse">
        <colgroup>
          <col class="w-6" />
          <col class="w-20" />
          <col v-if="wide || testing" />
          <col v-if="testing && !group.outputs" class="w-32" />
          <template v-if="!testing">
            <col class="w-28" />
            <col class="w-20" />
          </template>
        </colgroup>
        <tbody>
          <tr v-for="signal in group.list" :key="signal.address" :title="signal.description">
            <td :class="cell">
              <button
                v-if="testing && group.outputs"
                class="-m-1 grid size-5 cursor-default place-items-center rounded-full border-0 bg-transparent p-0 hover:bg-hover"
                :aria-pressed="bit(group.image, signal.byte, signal.bit)"
                :aria-label="`Switch ${signal.name} ${bit(group.image, signal.byte, signal.bit) ? 'off' : 'on'}`"
                @click="switchOutput(signal.address, !bit(group.image, signal.byte, signal.bit))"
              >
                <span class="block size-2.5 rounded-full" :class="lamp(bit(group.image, signal.byte, signal.bit))" />
              </button>
              <span v-else class="block size-2.5 rounded-full" :class="lamp(bit(group.image, signal.byte, signal.bit))" />
            </td>
            <td :class="cell" class="truncate font-semibold">{{ signal.name }}</td>
            <td v-if="wide || testing" :class="cell" class="text-muted">
              <span class="block truncate">{{ signal.description }}</span>
            </td>
            <td v-if="testing && !group.outputs" :class="cell" class="pr-0 text-right whitespace-nowrap">
              <label
                v-if="settingOf(signal.address)"
                class="inline-flex items-center gap-1 text-xs text-muted"
                :title="`${settingOf(signal.address)?.label} of ${instanceOf(signal.address)?.name}`"
              >
                <input
                  type="number"
                  :class="number"
                  :min="settingOf(signal.address)?.min"
                  :max="settingOf(signal.address)?.max"
                  :step="settingOf(signal.address)?.step || 1"
                  :value="parameterValue(settingOf(signal.address)?.id ?? '', settingOf(signal.address)?.value ?? 0)"
                  :aria-label="`${settingOf(signal.address)?.label} of ${signal.name}`"
                  @change="changeSetting(signal.address, $event)"
                />{{ settingOf(signal.address)?.unit }}
              </label>
            </td>
            <template v-if="!testing">
              <td :class="cell" class="truncate font-mono text-xs">{{ signal.variable }}</td>
              <td :class="cell" class="pr-0 text-right font-mono text-xs whitespace-nowrap text-muted" :title="signal.address">{{ signal.shown || signal.address }}</td>
            </template>
          </tr>
        </tbody>
      </table>
    </section>
    <section v-for="group in analogGroups" :key="group.title" class="min-w-0">
      <h3 class="m-0 mb-1 text-xs font-semibold text-muted">{{ group.title }}</h3>
      <table class="w-full border-collapse">
        <tbody>
          <tr v-for="signal in group.list" :key="signal.address" :title="signal.description">
            <td :class="cell" class="w-24 font-mono text-xs whitespace-nowrap text-ok">
              <input
                v-if="group.outputs && testing"
                type="number"
                :class="number"
                :min="signal.range[0]"
                :max="signal.range[1]"
                :step="(signal.range[1] - signal.range[0]) / 100"
                :value="(plcView?.analog?.[signal.address] ?? 0).toFixed(2)"
                :aria-label="`Set ${signal.name}`"
                @change="setAnalog(signal.address, signal.range, $event)"
              />
              <template v-else>{{ (plcView?.analog?.[signal.address] ?? 0).toFixed(2) }}</template>
              {{ signal.unit }}
            </td>
            <td :class="cell" class="w-16 font-semibold whitespace-nowrap">{{ signal.name }}</td>
            <td v-if="wide" :class="cell" class="text-muted">
              <span class="block truncate">{{ signal.description }}</span>
            </td>
            <td :class="cell" class="pr-0 text-right font-mono text-xs whitespace-nowrap text-muted" :title="`${signal.address}, ${signal.type}`">
              {{ signal.shown || signal.address }}
            </td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>
