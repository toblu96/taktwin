<script setup lang="ts">
import { computed } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { simulation } from '../sim'

// The I/O test from a module's pins: start it here to switch the outputs this module has, end it
// or switch all outputs off. The panel below the 3D view has the same for the whole PLC.
const props = defineProps<{ plc?: string }>()

const { plcId, snapshot } = simulation

// The PLC it tests: the one given, as that of a module's pins, else the one the panel shows.
const id = computed(() => props.plc ?? plcId.value)
const view = computed(() => snapshot.value?.plcs?.find((p, i) => (id.value ? p.id === id.value : i === 0)) ?? null)
const testing = computed(() => !!view.value?.ioTest)
const external = computed(() => !!view.value?.link && !view.value?.step && !testing.value)
const setTest = (on: boolean) => void Service.SetIOTestFor(id.value, on)
</script>

<template>
  <section class="t-section">
    <div class="flex items-center gap-2">
      <h3 class="t-section-title mb-0 flex-1">I/O test</h3>
      <template v-if="testing">
        <button class="t-btn h-6 px-2 text-xs" @click="Service.ClearTestOutputsFor(id)">All outputs off</button>
        <button class="t-btn t-btn-primary h-6 px-2 text-xs" @click="setTest(false)">End</button>
      </template>
      <button v-else class="t-btn h-6 px-2 text-xs" title="Switch this module's outputs by hand, without the program" @click="setTest(true)">Start</button>
    </div>
    <p v-if="testing" class="m-0 mt-2 text-xs text-muted">Click an output's lamp below to switch it.</p>
    <p v-if="external" class="m-0 mt-2 text-xs text-accent-ink">The PLC keeps running on its own during a test, with its inputs frozen.</p>
  </section>
</template>
