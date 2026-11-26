<script setup lang="ts">
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { simulation, useSim } from '../sim'
import UiLamp from '../ui/UiLamp.vue'
import type { SceneConnector, ScenePin } from '../../bindings/github.com/toblu96/taktwin/internal/model'

// The pins of a module, connector by connector, with their live state, what each is for and
// which PLC signal is wired to it. In the library any free signal can be switched by its lamp,
// in a running station a PLC output during the I/O test, which the top bar starts.
defineProps<{ connectors: SceneConnector[] }>()

const sim = useSim()
const { service, snapshot, stateOn } = sim

// A pin that a PLC output drives, at its address in the PLC's image.
const plcOutput = (pin: ScenePin) => pin.output && !!pin.address

const testOutput = (pin: ScenePin) => sim === simulation && !!snapshot.value?.ioTest && plcOutput(pin)
const canSwitch = (pin: ScenePin) => (pin.forcible && !!service.SetPin) || testOutput(pin)

function switchPin(pin: ScenePin) {
  const on = !stateOn(pin.state)
  if (testOutput(pin)) void Service.SetTestOutputFor(pin.plc ?? '', pin.address ?? '', on)
  else void service.SetPin?.(pin.state, on)
}

const row = 'flex min-h-8 items-center gap-2.5 border-0 border-b border-solid border-line py-1 last:border-b-0'
</script>

<template>
  <section v-for="connector in connectors" :key="connector.id" class="t-section">
    <h3 class="t-section-title">
      {{ connector.type === 'wires' ? 'Signals' : connector.id }}
      <span v-if="connector.type !== 'wires'" class="ml-1 font-normal tracking-normal normal-case">{{ connector.label }}</span>
    </h3>
    <ul class="m-0 list-none p-0">
      <li v-for="pin in connector.pins" :key="pin.pin" :class="row">
        <span class="grid w-4 shrink-0 place-items-center" :class="pin.wired ? '' : 'opacity-50'">
          <span v-if="pin.analog" class="text-sm leading-none text-muted" title="Analogue signal">~</span>
          <button
            v-else-if="canSwitch(pin)"
            class="-m-1 grid size-5 cursor-default place-items-center rounded-full border-0 bg-transparent p-0 hover:bg-hover"
            :aria-pressed="stateOn(pin.state)"
            :aria-label="`Switch ${pin.function || pin.pin} ${stateOn(pin.state) ? 'off' : 'on'}`"
            @click="switchPin(pin)"
          >
            <UiLamp :on="stateOn(pin.state)" :out="pin.output" />
          </button>
          <UiLamp v-else :on="stateOn(pin.state)" :out="pin.output" />
        </span>
        <span v-if="connector.type !== 'wires'" class="w-6 shrink-0 font-mono text-xs font-semibold" :class="pin.wired ? '' : 'text-faint'">{{ pin.pin }}</span>
        <span class="min-w-0 flex-1">
          <template v-if="pin.function">
            <span class="block truncate">{{ pin.function }}</span>
            <span v-if="pin.source" class="block truncate text-xs text-muted">{{ pin.source }}</span>
          </template>
          <span v-else class="text-xs text-faint">{{ pin.spare ? 'Spare' : 'Not used' }}</span>
        </span>
        <span class="shrink-0 text-right font-mono text-xs leading-tight" :title="pin.invert ? 'Inverted at the source' : undefined">
          <span v-if="pin.analog && pin.wired" class="mr-2 text-lamp-in">{{ (snapshot?.states?.[pin.state] ?? 0).toFixed(2) }} V</span>
          <template v-if="pin.plcName">
            <span class="block">{{ pin.invert ? '¬' : '' }}{{ pin.plcName }}</span>
            <span class="block text-[11px] text-muted" :title="pin.address">{{ pin.shown || pin.address }}</span>
          </template>
          <span v-else-if="pin.wired && !pin.address && !service.SetPin && !pin.analog" class="text-[11px] text-muted">not wired to the PLC</span>
        </span>
      </li>
    </ul>
  </section>
</template>
