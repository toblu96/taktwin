<script setup lang="ts">
import { computed, ref } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { simulation } from '../sim'
import UiSegmented from '../ui/UiSegmented.vue'
import IoTable from './IoTable.vue'
import IconTest from '~icons/fluent/toggle-left-16-regular'

// The inputs and outputs of a running station, in the side bar while no module is selected: the
// signals of a PLC, live, and the I/O test that switches its outputs by hand. With several PLCs,
// the one shown is picked here.
const { scene, snapshot } = simulation

const index = ref(0)
const plcs = computed(() => (scene.value?.plcs ?? []).map((p, i) => ({ value: String(i), label: scene.value?.instances?.find((m) => m.id === p.module)?.name ?? 'PLC' })))
const picked = computed({ get: () => String(index.value), set: (v: string) => (index.value = Number(v)) })

const id = computed(() => (index.value > 0 ? (scene.value?.plcs?.[index.value]?.module ?? '') : ''))
const view = computed(() => snapshot.value?.plcs?.[index.value] ?? null)
const testing = computed(() => !!view.value?.ioTest)
const external = computed(() => !!view.value?.link && !view.value?.step)
const setTest = (on: boolean) => void Service.SetIOTestFor(id.value, on)
</script>

<template>
  <div>
    <section class="t-section flex flex-col gap-2">
      <UiSegmented v-if="plcs.length > 1" v-model="picked" label="PLC" :options="plcs" class="self-start" />
      <div class="flex flex-wrap items-center gap-2">
        <template v-if="testing">
          <button class="t-btn t-btn-primary" @click="setTest(false)">End I/O test</button>
          <button class="t-btn" @click="Service.ClearTestOutputsFor(id)">All outputs off</button>
        </template>
        <button v-else class="t-btn" title="Switch the outputs by hand, without the program" @click="setTest(true)"><IconTest class="size-4" />Start I/O test</button>
      </div>
      <p class="m-0 text-xs text-muted">
        {{
          testing
            ? "The program is cut off. Click an output's lamp to switch it; set a sensor's threshold next to its input."
            : 'Test sensors and actuators by hand. Workpieces stay where they are, and all outputs start off.'
        }}
      </p>
      <p v-if="external" class="m-0 text-xs text-accent-ink">The PLC keeps running on its own during a test, with its inputs frozen.</p>
    </section>
    <section class="t-section">
      <IoTable :index="index" />
    </section>
  </div>
</template>
