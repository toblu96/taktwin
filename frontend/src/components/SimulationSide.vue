<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { ask } from '../ask'
import { programming, project, refreshProject, simulation } from '../sim'
import StationDock from './StationDock.vue'
import IconCode from '~icons/fluent/code-16-regular'
import IconTest from '~icons/fluent/toggle-left-16-regular'

// The side bar of the simulation as it runs: what the project is about, the station's PLCs, each
// with how it is connected and what it runs, its program and its I/O test, and the station's own
// controls, such as putting workpieces in. How a PLC is connected is set up in the wiring.
const { scene, snapshot, selection } = simulation

// The exercise the project is on, or else its station, with what it is about.
const exerciseText = ref('')
watch(
  () => [project.value?.exercise, scene.value?.id],
  async () => {
    const id = project.value?.exercise
    exerciseText.value = id ? (((await Service.Exercises()) ?? []).find((e) => e.id === id)?.description ?? '') : ''
  },
  { immediate: true },
)
const title = computed(() => project.value?.exerciseName || scene.value?.name || '')
const about = computed(() => exerciseText.value || scene.value?.description || '')

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

// Newer versions of the collections the project carries, installed here, which it takes only
// when asked: its station follows them unless the project changed it.
const updates = computed(() => project.value?.updates ?? [])
const updateText = computed(() => updates.value.map((u) => `${u.name || u.collection} ${u.installed}`).join(', '))
const updating = ref(false)
const updateProblem = ref('')
async function update() {
  const was = updates.value.map((u) => `${u.name || u.collection} ${u.version}`).join(', ')
  const ok = await ask(
    `Update this project to ${updateText.value}? It has ${was} now. The station follows the new version unless you changed it in this project; what you changed stays.`,
    'Update',
  )
  if (!ok) return
  updating.value = true
  updateProblem.value = ''
  try {
    simulation.scene.value = await Service.UpdateCollections()
    await refreshProject()
  } catch (e) {
    updateProblem.value = (e as Error).message ?? String(e)
  } finally {
    updating.value = false
  }
}

const hasControls = computed(() => (scene.value?.docks ?? []).some((d) => d.widgets?.length))
</script>

<template>
  <aside class="flex h-full min-w-0 flex-col bg-s1" aria-label="Station">
    <div class="min-h-0 flex-1 overflow-auto">
      <section class="t-section">
        <p class="m-0 text-[11px] font-semibold tracking-wide text-muted uppercase">{{ project?.exercise ? 'Exercise' : 'Station' }}</p>
        <h2 class="m-0 mt-1 text-[15px] leading-snug font-semibold">{{ title }}</h2>
        <p v-if="about" class="m-0 mt-1.5 line-clamp-4 text-muted" :title="about">{{ about }}</p>
        <div v-if="updates.length" class="mt-3 rounded-card border border-line bg-s0 px-2.5 py-2">
          <p class="m-0 text-xs text-muted">A newer version is installed: {{ updateText }}. This project keeps the one it was made with until you update it.</p>
          <button class="t-btn mt-2 h-6 px-2 text-xs" :disabled="updating" @click="update">Update…</button>
          <p v-if="updateProblem" class="m-0 mt-2 text-xs text-danger" role="alert">{{ updateProblem }}</p>
        </div>
      </section>
      <section v-if="plcs.length" class="t-section">
        <h3 class="t-section-title">{{ plcs.length > 1 ? 'PLCs' : 'PLC' }}</h3>
        <div class="flex flex-col gap-2">
          <div v-for="p in plcs" :key="p.id" class="rounded-card border bg-s0" :class="p.module && selection === p.module ? 'border-focus' : 'border-line'">
            <button
              class="flex w-full min-w-0 cursor-default items-start gap-2.5 rounded-t-card border-0 bg-transparent px-2.5 pt-2 pb-1.5 text-left text-fg hover:bg-hover focus-visible:-outline-offset-2"
              :disabled="!p.module"
              :title="p.module ? `Show ${p.name}` : undefined"
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
