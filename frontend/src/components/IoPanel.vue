<script setup lang="ts">
import { computed, ref } from 'vue'
import { connectAsPlanned, plans, simulation } from '../sim'
import UiSegmented from '../ui/UiSegmented.vue'
import IoTable from './IoTable.vue'
import IconConnect from '~icons/fluent/plug-connected-16-regular'

// The inputs and outputs of a running station, in the side bar while no module is selected: the
// signals of a PLC, live. During the I/O test, started in the top bar, the outputs are switched
// here by their lamps. With several PLCs, the one shown is picked here. While it is not connected
// it says so above its lamps, with Connect, or the way to set the connection up.
const { scene, snapshot, selection } = simulation

const index = ref(0)
const plcs = computed(() => (scene.value?.plcs ?? []).map((p, i) => ({ value: String(i), label: scene.value?.instances?.find((m) => m.id === p.module)?.name ?? 'PLC' })))
const picked = computed({ get: () => String(index.value), set: (v: string) => (index.value = Number(v)) })

const testing = computed(() => !!snapshot.value?.ioTest)
const external = computed(() => !!snapshot.value?.plcs?.[index.value]?.link)

// The PLC shown: its id, "" for the first, its module, and whether anything runs it.
const id = computed(() => (index.value > 0 ? (scene.value?.plcs?.[index.value]?.module ?? '') : ''))
const module = computed(() => scene.value?.plcs?.[index.value]?.module ?? null)
const idle = computed(() => {
  const view = snapshot.value?.plcs?.[index.value]
  return !!snapshot.value && !!scene.value?.plcs?.length && !view?.link && (!view?.plc || view.plc === 'No PLC')
})
const planned = computed(() => plans.value[id.value]?.mode ?? 'none')
const busy = ref(false)
const problem = ref('')
async function connect() {
  busy.value = true
  problem.value = ''
  try {
    await connectAsPlanned(id.value)
  } catch (e) {
    problem.value = (e as Error).message ?? String(e)
  } finally {
    busy.value = false
  }
}
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
    <section v-if="idle && !testing" class="t-section">
      <div class="flex items-center gap-2 rounded-control border border-line bg-s0 px-2.5 py-2" role="status">
        <span class="size-2 shrink-0 rounded-full bg-faint" />
        <span class="min-w-0 flex-1">
          <span class="block font-medium">Not connected</span>
          <span class="block text-xs text-muted">No PLC runs: every output stays off.</span>
        </span>
        <button
          v-if="planned !== 'none'"
          class="t-btn t-btn-primary w-28 shrink-0 justify-center"
          :disabled="busy"
          title="Connect as the project has it, and restart the station"
          @click="connect"
        >
          <IconConnect class="size-4" />Connect
        </button>
        <button v-else-if="module" class="t-btn shrink-0" title="Choose how the PLC connects" @click="selection = module">Set up…</button>
      </div>
      <p v-if="problem" class="m-0 mt-2 text-xs text-danger" role="alert">{{ problem }}</p>
    </section>
    <section class="t-section">
      <IoTable :index="index" />
    </section>
  </div>
</template>
