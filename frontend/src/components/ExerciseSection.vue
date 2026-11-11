<script setup lang="ts">
import { Events } from '@wailsio/runtime'
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { Service, type CheckState, type ExerciseInfo } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { openExercise, useSim } from '../sim'

// The exercises of the station, and the check of one of them against whatever PLC runs it,
// line by line as on a grading sheet.
const { scene } = useSim()
const exercises = ref<ExerciseInfo[]>([])
const check = ref<CheckState | null>(null)
const error = ref('')

async function load() {
  exercises.value = (await Service.Exercises()) ?? []
  check.value = await Service.Check()
}

async function open(id: string) {
  error.value = ''
  try {
    await openExercise(id)
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function start(id: string) {
  error.value = ''
  try {
    await Service.StartCheck(id)
  } catch (e) {
    error.value = (e as Error).message
  }
}

let off: (() => void) | undefined
onMounted(() => {
  off = Events.On('sim:check', (event) => {
    check.value = event.data as CheckState
  })
  load()
})
onUnmounted(() => off?.())
watch(() => scene.value?.id, load)

const name = (id: string) => exercises.value.find((e) => e.id === id)?.name ?? id
const passed = (c: CheckState) => !c.running && !c.error && (c.lines ?? []).length > 0 && (c.lines ?? []).every((l) => l.ok)
</script>

<template>
  <section v-if="exercises.length" class="section">
    <h3 class="section-title">Exercises</h3>
    <div v-for="exercise in exercises" :key="exercise.id" class="exercise">
      <div class="row">
        <strong>{{ exercise.name }}</strong>
        <button v-if="check?.running && check.exercise === exercise.id" @click="Service.StopCheck()">Stop</button>
        <button v-else-if="!exercise.open" :disabled="check?.running" title="Put the station into the simulation as this exercise sets it up" @click="open(exercise.id)">
          Open
        </button>
        <button v-else :disabled="check?.running" title="Reset the station and check it line by line" @click="start(exercise.id)">Check</button>
      </div>
      <p class="description">{{ exercise.description }}</p>
      <div v-if="exercise.changes?.length" class="changes">
        <span class="changes-title">{{ exercise.open ? 'Set up for this exercise' : 'Sets the station up' }}</span>
        <ul>
          <li v-for="change in exercise.changes" :key="change">{{ change }}</li>
        </ul>
      </div>
    </div>
    <p v-if="error" class="error">{{ error }}</p>
    <div v-if="check?.exercise" class="result">
      <p class="status" :class="{ ok: passed(check), bad: !check.running && !passed(check) }">
        {{ name(check.exercise) }}: {{ check.running ? 'checking…' : passed(check) ? 'passed' : 'failed' }}
      </p>
      <ol class="lines">
        <li v-for="(line, index) in check.lines ?? []" :key="index" :class="line.ok ? 'ok' : 'bad'">
          <span class="mark">{{ line.ok ? '✓' : '✗' }}</span
          >{{ line.line }}
        </li>
      </ol>
      <p v-if="check.error" class="error">{{ check.error }}</p>
    </div>
  </section>
</template>

<style scoped>
.exercise + .exercise {
  margin-top: 10px;
}

.row {
  align-items: center;
  display: flex;
  gap: 8px;
  justify-content: space-between;
}

.description {
  color: var(--text-dim);
  font-size: 12px;
  line-height: 1.4;
  margin: 4px 0 0;
}

.result {
  margin-top: 12px;
}

.changes {
  font-size: 12px;
  margin-top: 6px;
}

.changes-title {
  color: var(--text-dim);
}

.changes ul {
  margin: 2px 0 0;
  padding-left: 16px;
}

.status {
  font-weight: 600;
  margin: 0 0 6px;
}

.lines {
  list-style: none;
  margin: 0;
  padding: 0;
}

.lines li {
  font-size: 12px;
  line-height: 1.5;
}

.mark {
  display: inline-block;
  width: 16px;
}

.ok {
  color: var(--signal);
}

.bad,
.error {
  color: var(--danger);
}

.lines li.ok {
  color: var(--text);
}

.lines li.ok .mark {
  color: var(--signal);
}
</style>
