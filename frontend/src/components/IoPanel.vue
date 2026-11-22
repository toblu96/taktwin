<script setup lang="ts">
import { computed } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { simulation } from '../sim'
import UiSegmented from '../ui/UiSegmented.vue'
import IoTable from './IoTable.vue'
import IconTest from '~icons/fluent/toggle-left-16-regular'

// The inputs and outputs tab of the panel: the signals of a PLC, live, and the I/O test that
// switches its outputs by hand. With several PLCs, the one shown is picked here or selected.
const { scene, selection, plcId, plcScene, plcView } = simulation

const plcs = computed(() => (scene.value?.plcs ?? []).map((p) => ({ value: p.module ?? '', label: scene.value?.instances?.find((i) => i.id === p.module)?.name ?? 'PLC' })))
const shown = computed({
  get: () => plcScene.value?.module ?? '',
  set: (module: string) => (selection.value = module),
})

const testing = computed(() => !!plcView.value?.ioTest)
const external = computed(() => !!plcView.value?.link && !plcView.value?.step)
const setTest = (on: boolean) => void Service.SetIOTestFor(plcId.value, on)
</script>

<template>
  <div class="flex flex-col gap-3">
    <div class="flex flex-wrap items-center gap-2">
      <UiSegmented v-if="plcs.length > 1" v-model="shown" label="PLC" :options="plcs" />
      <template v-if="testing">
        <button class="t-btn t-btn-primary" @click="setTest(false)">End I/O test</button>
        <button class="t-btn" @click="Service.ClearTestOutputsFor(plcId)">All outputs off</button>
        <span class="text-xs text-muted">The program is cut off. Switch outputs here or on a module's pins; set a sensor's threshold next to its input.</span>
      </template>
      <template v-else>
        <button class="t-btn" title="Switch the outputs by hand, without the program" @click="setTest(true)"><IconTest class="size-4" />Start I/O test</button>
        <span class="text-xs text-muted">Test sensors and actuators by hand. Workpieces stay where they are, and all outputs start off.</span>
      </template>
      <span v-if="external" class="text-xs text-accent-ink">The PLC keeps running on its own during a test, with its inputs frozen.</span>
    </div>
    <IoTable wide />
  </div>
</template>
