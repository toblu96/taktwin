<script setup lang="ts">
import { computed, ref } from 'vue'
import { simulation } from '../sim'
import UiSegmented from '../ui/UiSegmented.vue'
import IoTable from './IoTable.vue'

// The inputs and outputs of a running station, in the side bar while no module is selected: the
// signals of a PLC, live. During the I/O test, started in the top bar, the outputs are switched
// here by their lamps. With several PLCs, the one shown is picked here.
const { scene, snapshot } = simulation

const index = ref(0)
const plcs = computed(() => (scene.value?.plcs ?? []).map((p, i) => ({ value: String(i), label: scene.value?.instances?.find((m) => m.id === p.module)?.name ?? 'PLC' })))
const picked = computed({ get: () => String(index.value), set: (v: string) => (index.value = Number(v)) })

const testing = computed(() => !!snapshot.value?.ioTest)
const external = computed(() => !!snapshot.value?.plcs?.[index.value]?.link)
</script>

<template>
  <div>
    <section v-if="plcs.length > 1 || testing" class="t-section flex flex-col gap-2">
      <UiSegmented v-if="plcs.length > 1" v-model="picked" label="PLC" :options="plcs" class="self-start" />
      <template v-if="testing">
        <p class="m-0 text-xs text-muted">The programs are cut off. Click an output's lamp to switch it; set a sensor's threshold next to its input.</p>
        <p v-if="external" class="m-0 text-xs text-accent-ink">The PLC keeps running on its own during the test, with its inputs frozen.</p>
      </template>
    </section>
    <section class="t-section">
      <IoTable :index="index" />
    </section>
  </div>
</template>
