<script setup lang="ts">
import { computed } from 'vue'
import { ProgramService } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { programming, simulation, useSim } from '../sim'
import IoTable from './IoTable.vue'
import PlcConnection from './PlcConnection.vue'
import IconCode from '~icons/fluent/code-16-regular'
import IconWindow from '~icons/fluent/window-new-16-regular'

// A PLC: in a running station its program, what runs and its step, and how it is connected; its
// signals in the library. In an open project the panel below the 3D view shows the signals and
// their test, so the PLC's own view leaves them out.
withDefaults(defineProps<{ signals?: boolean; connection?: boolean }>(), { signals: true, connection: true })

const sim = useSim()
const { plcId, plcScene, plcView, scene } = sim
const hasSignals = computed(() => {
  const p = plcScene.value
  return !!(p?.inputs?.length || p?.outputs?.length || p?.analog?.length || p?.analogOutputs?.length)
})
</script>

<template>
  <section v-if="sim === simulation" class="t-section">
    <h3 class="t-section-title">Program</h3>
    <dl class="m-0 mb-3 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1">
      <dt class="text-muted">Runs</dt>
      <dd class="m-0 truncate">{{ plcView?.plc || 'No PLC' }}</dd>
      <template v-if="plcView?.step">
        <dt class="text-muted">Step</dt>
        <dd class="m-0 truncate font-mono text-xs leading-5"><b class="font-semibold text-accent-ink">S{{ plcView.step.number }}</b> {{ plcView.step.label }}</dd>
      </template>
    </dl>
    <p class="m-0 mb-2.5 text-xs text-muted">Write the program in Structured Text, run it in the app's soft PLC, and watch and force its variables.</p>
    <div class="flex flex-wrap gap-2">
      <button class="t-btn" @click="programming = plcId"><IconCode class="size-4" />Edit program</button>
      <button class="t-btn" title="Beside the simulation, in a window of its own" @click="ProgramService.OpenWindow(plcId)"><IconWindow class="size-4" />Open in window</button>
    </div>
  </section>
  <PlcConnection v-if="sim === simulation && connection" :key="`${scene?.id}:${plcId}`" />
  <section v-if="signals && hasSignals" class="t-section">
    <IoTable />
  </section>
</template>
