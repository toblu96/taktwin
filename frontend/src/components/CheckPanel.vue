<script setup lang="ts">
import { Events } from '@wailsio/runtime'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { Service, type CheckState, type ExerciseInfo } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { newProject, project, simulation } from '../sim'
import MarkdownText from './MarkdownText.vue'
import IconPassed from '~icons/fluent/checkmark-circle-16-filled'
import IconFailed from '~icons/fluent/dismiss-circle-16-filled'
import IconWaiting from '~icons/fluent/circle-16-regular'
import IconTask from '~icons/fluent/clipboard-task-16-regular'
import IconWarning from '~icons/fluent/warning-16-regular'

// The check tab of the panel. In an exercise's project: what the exercise asks for, and its
// check against whatever PLC runs the station, line by line as on a grading sheet. In a
// station's project: the exercises written for the station, each to start a project on.
const { scene } = simulation
const exercises = ref<ExerciseInfo[]>([])
const check = ref<CheckState | null>(null)
const error = ref('')
// What the project's exercise asks for, in Markdown, if it says more than its description.
const instructions = ref('')

async function load() {
  exercises.value = (await Service.Exercises()) ?? []
  check.value = await Service.Check()
  instructions.value = await Service.Instructions()
}

async function run(job: () => Promise<unknown>) {
  error.value = ''
  try {
    await job()
  } catch (e) {
    error.value = (e as Error).message
  }
}

// A teacher's check may load the station as their collections have it: the view follows.
async function startCheck(id: string) {
  await Service.StartCheck(id)
  if ((await Service.Check()).note) {
    scene.value = await Service.Scene()
    exercises.value = (await Service.Exercises()) ?? []
  }
}

let off: (() => void) | undefined
onMounted(() => {
  off = Events.On('sim:check', (event) => {
    check.value = event.data as CheckState
  })
  void load()
})
onUnmounted(() => off?.())
watch(() => scene.value?.id, load)

const current = computed(() => exercises.value.find((e) => e.open && e.id === project.value?.exercise) ?? exercises.value.find((e) => e.open) ?? null)
const result = computed(() => (check.value && check.value.exercise === current.value?.id ? check.value : null))
const lines = computed(() => result.value?.lines ?? [])
const passed = computed(() => !!result.value && !result.value.running && !result.value.error && lines.value.length > 0 && lines.value.every((l) => l.ok))
const status = computed(() => {
  if (!result.value) return { text: 'Not checked yet', class: 'text-muted' }
  if (result.value.running) return { text: 'Checking…', class: 'text-fg' }
  return passed.value ? { text: 'Passed', class: 'text-ok' } : { text: 'Failed', class: 'text-danger' }
})
</script>

<template>
  <div v-if="current" class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(300px,420px)]">
    <section class="min-w-0">
      <h3 class="m-0 flex items-center gap-1.5 text-[13px] font-semibold"><IconTask class="size-4 text-muted" />{{ current.name }}</h3>
      <p v-if="project?.exerciseChanged" class="m-0 mt-2 flex max-w-[70ch] items-start gap-1.5 rounded-control border border-danger/40 bg-s1 px-2.5 py-2 text-xs" role="alert">
        <IconWarning class="mt-px size-4 shrink-0 text-danger" />
        <span>This exercise or its station is not as it was handed out. To start again from there, put everything back with ⋯ beside the project's name.</span>
      </p>
      <p v-if="current.description" class="m-0 mt-1 max-w-[70ch] text-muted">{{ current.description }}</p>
      <MarkdownText v-if="instructions" class="mt-2 max-w-[70ch]" :source="instructions" />
      <div v-if="current.changes?.length" class="mt-3 text-xs text-muted">
        <span class="font-medium">Set up for this exercise</span>
        <ul class="m-0 mt-1 pl-4">
          <li v-for="change in current.changes" :key="change">{{ change }}</li>
        </ul>
      </div>
    </section>

    <section class="min-w-0" aria-label="Check">
      <div class="flex items-center gap-2">
        <span class="font-semibold" :class="status.class" role="status">{{ status.text }}</span>
        <span class="flex-1" />
        <button v-if="result?.running" class="t-btn" @click="run(() => Service.StopCheck())">Stop</button>
        <button v-else class="t-btn t-btn-primary" title="Reset the station and check it line by line" @click="run(() => startCheck(current!.id))">
          {{ result ? 'Check again' : 'Check' }}
        </button>
      </div>
      <p v-if="result?.note" class="m-0 mt-2 flex items-start gap-1.5 text-xs text-muted" role="status">
        <IconWarning class="mt-px size-4 shrink-0 text-accent-ink" />{{ result.note }}
      </p>
      <p v-if="!result" class="m-0 mt-2 text-xs text-muted">The check resets the station, runs it with your PLC, and goes through what the exercise asks for, line by line.</p>
      <ol class="m-0 mt-2 flex list-none flex-col gap-1 p-0">
        <li v-for="(line, i) in lines" :key="i" class="flex items-start gap-2">
          <component :is="line.ok ? IconPassed : IconFailed" class="mt-0.5 size-4 shrink-0" :class="line.ok ? 'text-ok' : 'text-danger'" />
          <span :class="line.ok ? 'text-muted' : 'text-fg'">{{ line.line }}</span>
        </li>
        <li v-if="result?.running" class="flex items-center gap-2 text-muted"><IconWaiting class="size-4 shrink-0" />…</li>
      </ol>
      <p v-if="result?.error || error" class="m-0 mt-2 text-danger" role="alert">{{ result?.error || error }}</p>
    </section>
  </div>

  <div v-else-if="exercises.length" class="flex flex-col gap-2">
    <p class="m-0 text-muted">Exercises written for this station. Each starts a project of its own, with the station set up as the exercise says.</p>
    <ul class="m-0 flex list-none flex-col gap-1 p-0">
      <li v-for="e in exercises" :key="e.id" class="flex items-start gap-3 rounded-control border border-line bg-s1 px-3 py-2">
        <IconTask class="mt-0.5 size-4 shrink-0 text-muted" />
        <span class="min-w-0 flex-1">
          <span class="block font-medium">{{ e.name }}</span>
          <span v-if="e.description" class="line-clamp-2 text-xs text-muted">{{ e.description }}</span>
        </span>
        <button class="t-btn" @click="run(() => newProject({ station: '', exercise: e.id }))">New project</button>
      </li>
    </ul>
    <p v-if="error" class="m-0 text-danger" role="alert">{{ error }}</p>
  </div>

  <p v-else class="m-0 text-muted">No exercises are written for this station yet.</p>
</template>
