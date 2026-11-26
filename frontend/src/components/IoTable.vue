<script setup lang="ts">
import { computed } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { bit, simulation, useSim } from '../sim'
import UiLamp from '../ui/UiLamp.vue'

// The signals of the PLC that is shown, live: a lamp for each digital input and output, the
// value of each analogue one. During an I/O test the outputs are switched here by hand, and a
// sensor's threshold sits next to its input, to teach it with a workpiece in front of it. Wide,
// on a page of its own, inputs and outputs stand side by side with their addresses; narrow, as
// in a side bar, each signal shows what it is for and its variable, its address in its tooltip.
// It shows the PLC given by its index, or else the one the selection picks. Pointing at a signal,
// with the mouse or the keyboard, marks the device it belongs to in the 3D view.
const props = defineProps<{ wide?: boolean; index?: number }>()

const sim = useSim()
const { parameterValue, scene } = sim
const plcScene = computed(() => (props.index === undefined ? sim.plcScene.value : (scene.value?.plcs?.[props.index] ?? scene.value?.plc ?? null)))
const plcView = computed(() => (props.index === undefined ? sim.plcView.value : (sim.snapshot.value?.plcs?.[props.index] ?? null)))
const plcId = computed(() => (props.index === undefined ? sim.plcId.value : props.index > 0 ? (scene.value?.plcs?.[props.index]?.module ?? '') : ''))

const testing = computed(() => sim === simulation && !!sim.snapshot.value?.ioTest)
const switchOutput = (variable: string, on: boolean) => void Service.SetTestOutputFor(plcId.value, variable, on)

// The module behind a signal. Pins name their PLC only where the station has several.
const pinPlc = computed(() => ((scene.value?.plcs?.length ?? 0) > 1 ? (plcScene.value?.module ?? '') : ''))
const instanceOf = (address: string) =>
  scene.value?.instances?.find((i) => i.connectors?.some((c) => c.type === 'wires' && c.pins?.some((p) => p.address === address && (p.plc ?? '') === pinPlc.value)))
// The module and device a signal belongs to, as its pins say.
function placeOf(address: string) {
  for (const i of scene.value?.instances ?? [])
    for (const c of i.connectors ?? [])
      for (const p of c.pins ?? []) if (p.address === address && (p.plc ?? '') === pinPlc.value && p.owner) return { instance: p.owner, device: p.device ?? '' }
  return null
}
const point = (address: string | null) => (sim.focus.value = address ? placeOf(address) : null)

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
          <col />
          <template v-if="!testing">
            <col :class="wide ? 'w-28' : 'w-24'" />
            <col v-if="wide" class="w-20" />
          </template>
        </colgroup>
        <tbody>
          <tr
            v-for="signal in group.list"
            :key="signal.address"
            :title="signal.description"
            class="hover:bg-hover"
            @mouseenter="point(signal.address)"
            @mouseleave="point(null)"
            @focusin="point(signal.address)"
            @focusout="point(null)"
          >
            <td :class="cell">
              <button
                v-if="testing && group.outputs"
                class="-m-1 grid size-5 cursor-default place-items-center rounded-full border-0 bg-transparent p-0 hover:bg-hover"
                :aria-pressed="bit(group.image, signal.byte, signal.bit)"
                :aria-label="`Switch ${signal.name} ${bit(group.image, signal.byte, signal.bit) ? 'off' : 'on'}`"
                @click="switchOutput(signal.address, !bit(group.image, signal.byte, signal.bit))"
              >
                <UiLamp :on="bit(group.image, signal.byte, signal.bit)" :out="group.outputs" />
              </button>
              <UiLamp v-else :on="bit(group.image, signal.byte, signal.bit)" :out="group.outputs" />
            </td>
            <td :class="cell" class="truncate font-semibold">{{ signal.name }}</td>
            <td :class="cell" class="text-muted">
              <span class="flex items-center gap-2">
                <span class="min-w-0 flex-1 truncate">{{ signal.description }}</span>
                <template v-if="testing && !group.outputs">
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
                </template>
              </span>
            </td>
            <template v-if="!testing">
              <td :class="[cell, wide ? '' : 'pr-0']" class="truncate font-mono text-xs" :title="wide ? undefined : signal.shown || signal.address">{{ signal.variable }}</td>
              <td v-if="wide" :class="cell" class="pr-0 text-right font-mono text-xs whitespace-nowrap text-muted" :title="signal.address">{{ signal.shown || signal.address }}</td>
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
