<script setup lang="ts">
import { computed } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { programming, simulation } from '../sim'
import StationDock from './StationDock.vue'
import IconCode from '~icons/fluent/code-16-regular'
import IconTest from '~icons/fluent/toggle-left-16-regular'

// The side bar of the simulation as it runs: the station's PLCs, each with how it is connected
// and what it runs, its program and its I/O test, and the station's own controls, such as
// putting workpieces in. Clicking a PLC shows its connection on the right.
const { scene, snapshot, selection } = simulation

// The engine calls a PLC with nothing to run "No PLC".
const idle = 'No PLC'
const STATES: Record<string, string> = { connected: 'Connected', connecting: 'Connecting…', failed: 'Not connected' }
const LAMPS: Record<string, string> = { connected: 'bg-ok', connecting: 'bg-accent', failed: 'bg-danger' }

const plcs = computed(() => {
  const list = scene.value?.plcs?.length ? scene.value.plcs : scene.value?.plc ? [scene.value.plc] : []
  return list.map((p, i) => {
    const view = snapshot.value?.plcs?.[i] ?? null
    const link = view?.link ?? null
    const runs = view?.plc && view.plc !== idle ? view.plc : ''
    return {
      id: i > 0 ? (p.module ?? '') : '',
      module: p.module ?? null,
      name: scene.value?.instances?.find((m) => m.id === p.module)?.name ?? 'PLC',
      runs,
      state: link ? (STATES[link.state] ?? link.state) : runs ? 'Running' : 'No PLC connected',
      lamp: link ? (LAMPS[link.state] ?? 'bg-faint') : runs ? 'bg-ok' : 'bg-faint',
      roundTrip: link?.state === 'connected' ? `${link.roundTrip.toFixed(1)} ms` : '',
      step: view?.step ?? null,
      testing: !!view?.ioTest,
    }
  })
})

const hasControls = computed(() => (scene.value?.docks ?? []).some((d) => d.widgets?.length))
</script>

<template>
  <aside class="flex h-full min-w-0 flex-col bg-s1" aria-label="Station">
    <h2 class="m-0 flex h-9 shrink-0 items-center px-3 text-xs font-semibold tracking-wide text-muted uppercase">Station</h2>
    <div class="min-h-0 flex-1 overflow-auto">
      <section v-if="plcs.length" class="t-section pt-1">
        <h3 class="t-section-title">{{ plcs.length > 1 ? 'PLCs' : 'PLC' }}</h3>
        <div class="flex flex-col gap-2">
          <div v-for="p in plcs" :key="p.id" class="rounded-card border bg-s0" :class="p.module && selection === p.module ? 'border-focus' : 'border-line'">
            <button
              class="flex w-full min-w-0 cursor-default items-start gap-2.5 rounded-t-card border-0 bg-transparent px-2.5 pt-2 pb-1.5 text-left text-fg hover:bg-hover focus-visible:-outline-offset-2"
              :disabled="!p.module"
              :title="p.module ? `Show how ${p.name} is connected` : undefined"
              @click="selection = p.module"
            >
              <span class="mt-1.5 size-2 shrink-0 rounded-full" :class="p.lamp" />
              <span class="min-w-0 flex-1">
                <span class="block truncate font-medium">{{ p.name }}</span>
                <span class="flex gap-1.5 text-xs text-muted">
                  <span class="truncate">{{ p.testing ? 'I/O test: outputs by hand' : p.state }}</span>
                  <span v-if="p.roundTrip && !p.testing" class="shrink-0 font-mono">{{ p.roundTrip }}</span>
                </span>
                <span v-if="p.runs" class="block truncate text-xs text-muted">{{ p.runs }}</span>
                <span v-if="p.step" class="mt-0.5 block truncate font-mono text-xs"><b class="font-semibold text-accent-ink">S{{ p.step.number }}</b> {{ p.step.label }}</span>
              </span>
            </button>
            <div class="flex gap-1.5 border-0 border-t border-solid border-line px-2.5 py-2">
              <button class="t-btn h-6 px-2 text-xs" :title="`Write the program of ${p.name}`" @click="programming = p.id"><IconCode class="size-3.5" />Program</button>
              <button
                class="t-btn h-6 px-2 text-xs"
                :class="p.testing ? 't-btn-primary' : ''"
                :aria-pressed="p.testing"
                :title="p.testing ? 'Give the outputs back to the program' : 'Switch the outputs by hand, without the program'"
                @click="Service.SetIOTestFor(p.id, !p.testing)"
              >
                <IconTest class="size-3.5" />{{ p.testing ? 'End I/O test' : 'I/O test' }}
              </button>
            </div>
          </div>
        </div>
      </section>
      <section v-if="hasControls" class="t-section">
        <StationDock />
      </section>
    </div>
  </aside>
</template>
