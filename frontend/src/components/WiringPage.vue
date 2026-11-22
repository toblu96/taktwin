<script setup lang="ts">
import { computed } from 'vue'
import { simulation } from '../sim'
import UiSegmented from '../ui/UiSegmented.vue'
import IoTable from './IoTable.vue'
import PlcConnection from './PlcConnection.vue'

// The wiring view of an open project, as it runs: how each PLC is connected, a controller outside
// the app or the soft PLC, and the signals between the station and the PLC, with what each one
// is for, its variable and its address. Until the wiring is drawn here, this is its overview.
const { scene, selection, plcId, plcScene } = simulation

const plcs = computed(() => (scene.value?.plcs ?? []).map((p) => ({ value: p.module ?? '', label: scene.value?.instances?.find((i) => i.id === p.module)?.name ?? 'PLC' })))
const shown = computed({
  get: () => plcScene.value?.module ?? '',
  set: (module: string) => (selection.value = module),
})
</script>

<template>
  <main class="min-h-0 overflow-auto bg-s0">
    <div class="mx-auto flex max-w-7xl flex-col gap-5 px-8 py-6">
      <div class="flex items-center gap-3">
        <h2 class="m-0 flex-1 text-[15px] font-semibold">Wiring</h2>
        <UiSegmented v-if="plcs.length > 1" v-model="shown" label="PLC" :options="plcs" />
      </div>
      <div class="grid items-start gap-6 lg:grid-cols-[minmax(300px,380px)_minmax(0,1fr)]">
        <div class="rounded-card border border-line bg-s1">
          <PlcConnection :key="`${scene?.id}:${plcId}`" />
        </div>
        <section class="min-w-0">
          <h3 class="t-section-title">Signals</h3>
          <IoTable wide />
        </section>
      </div>
    </div>
  </main>
</template>
