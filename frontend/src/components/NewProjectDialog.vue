<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { LibraryService, Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { ExerciseInfo } from '../../bindings/github.com/toblu96/taktwin/internal/simapp/models'
import { grouped } from '../collections'
import { newProject } from '../sim'

// Makes a project from a station or an exercise of the collections, named after it unless
// it is given a name, and opens it.
const props = defineProps<{ station?: string }>()
const emit = defineEmits<{ close: [] }>()

type Choice = { id: string; name: string; description?: string; collection?: string; on?: string }

const from = ref<'station' | 'exercise'>('station')
const stations = ref<Choice[]>([])
const exercises = ref<Choice[]>([])
const chosen = ref({ station: props.station ?? '', exercise: '' })
const name = ref('')
const busy = ref(false)
const error = ref('')
const nameInput = ref<HTMLInputElement | null>(null)

onMounted(async () => {
  const [entries, all] = await Promise.all([LibraryService.Entries(), Service.AllExercises()])
  stations.value = (entries ?? []).filter((e) => e.level === 'station' && !e.error)
  exercises.value = (all ?? []).map((e: ExerciseInfo) => ({ id: e.id, name: e.name, description: e.description, collection: e.collection, on: e.stationName }))
  chosen.value.station ||= stations.value[0]?.id ?? ''
  chosen.value.exercise ||= exercises.value[0]?.id ?? ''
  await nextTick()
  nameInput.value?.focus()
})

const choices = computed(() => (from.value === 'station' ? stations.value : exercises.value))
const current = computed(() => choices.value.find((c) => c.id === chosen.value[from.value]))

async function create() {
  if (!current.value || busy.value) return
  busy.value = true
  error.value = ''
  try {
    await newProject(from.value === 'station' ? { station: current.value.id, exercise: '' } : { station: '', exercise: current.value.id }, name.value.trim())
    emit('close')
  } catch (e) {
    error.value = (e as Error).message ?? String(e)
  } finally {
    busy.value = false
  }
}

function pick(c: Choice) {
  chosen.value[from.value] = c.id
}

function key(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}
onMounted(() => window.addEventListener('keydown', key))
onUnmounted(() => window.removeEventListener('keydown', key))
</script>

<template>
  <div class="backdrop" @click.self="emit('close')">
    <section class="dialog" role="dialog" aria-modal="true" aria-labelledby="new-project-title">
      <header>
        <h2 id="new-project-title">New project</h2>
        <div class="segmented" role="tablist" aria-label="Start from">
          <button role="tab" :aria-selected="from === 'station'" :class="{ active: from === 'station' }" @click="from = 'station'">Station</button>
          <button role="tab" :aria-selected="from === 'exercise'" :class="{ active: from === 'exercise' }" :disabled="!exercises.length" @click="from = 'exercise'">
            Exercise
          </button>
        </div>
      </header>

      <div class="choices">
        <template v-for="g in grouped(choices)" :key="g.id">
          <h3 class="section-title">{{ g.name }}</h3>
          <div class="tiles">
            <button
              v-for="c in g.items"
              :key="c.id"
              class="tile"
              :class="{ active: chosen[from] === c.id }"
              :aria-pressed="chosen[from] === c.id"
              @click="pick(c)"
              @dblclick="create"
            >
              <strong>{{ c.name }}</strong>
              <small v-if="c.on">On {{ c.on }}</small>
              <span v-if="c.description" class="description">{{ c.description }}</span>
            </button>
          </div>
        </template>
        <p v-if="!choices.length" class="empty">Your collections have no {{ from === 'station' ? 'stations' : 'exercises' }}.</p>
      </div>

      <footer>
        <label class="field">
          <span>Name</span>
          <input ref="nameInput" v-model="name" type="text" maxlength="80" :placeholder="current?.name ?? ''" @keydown.enter="create" />
        </label>
        <p v-if="error" class="error" role="alert">{{ error }}</p>
        <div class="actions">
          <button @click="emit('close')">Cancel</button>
          <button class="primary" :disabled="busy || !current" @click="create">Create</button>
        </div>
      </footer>
    </section>
  </div>
</template>

<style scoped>
.backdrop {
  align-items: center;
  background: rgb(0 0 0 / 45%);
  display: flex;
  inset: 0;
  justify-content: center;
  padding: 24px;
  position: fixed;
  z-index: 40;
}

.dialog {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 6px;
  box-shadow: 0 16px 48px rgb(0 0 0 / 45%);
  display: grid;
  grid-template-rows: auto minmax(0, 1fr) auto;
  max-height: min(640px, 100%);
  width: min(720px, 100%);
}

header {
  align-items: center;
  border-bottom: 1px solid var(--line);
  display: flex;
  justify-content: space-between;
  padding: 14px 18px;
}

h2 {
  font-size: 16px;
  margin: 0;
}

.segmented {
  display: flex;
}

.segmented button {
  border-radius: 0;
  margin-left: -1px;
  min-width: 84px;
}

.segmented button:first-child {
  border-radius: 3px 0 0 3px;
}

.segmented button:last-child {
  border-radius: 0 3px 3px 0;
}

.segmented .active {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--bg);
  font-weight: 600;
  position: relative;
}

.choices {
  overflow: auto;
  padding: 14px 18px;
}

.section-title {
  margin-top: 4px;
}

.tiles {
  display: grid;
  gap: 8px;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  margin-bottom: 16px;
}

.tile {
  align-items: flex-start;
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: 4px;
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 10px 12px;
  text-align: left;
}

.tile.active {
  border-color: var(--accent);
  box-shadow: inset 0 0 0 1px var(--accent);
}

.tile small {
  color: var(--text-dim);
  font-size: 11px;
}

.description {
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
  color: var(--text-dim);
  display: -webkit-box;
  font-size: 12px;
  line-height: 1.4;
  overflow: hidden;
}

.empty {
  color: var(--text-dim);
}

footer {
  align-items: flex-end;
  border-top: 1px solid var(--line);
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  padding: 14px 18px;
}

.field {
  display: grid;
  flex: 1;
  gap: 3px;
  min-width: 220px;
}

.field span {
  color: var(--text-dim);
  font-size: 12px;
}

.field input {
  background: var(--bg);
  border: 1px solid var(--line);
  color: var(--text);
  padding: 5px 8px;
}

.actions {
  display: flex;
  gap: 6px;
}

.primary {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--bg);
  font-weight: 600;
}

.error {
  color: var(--danger);
  flex-basis: 100%;
  font-size: 12px;
  margin: 0;
  order: -1;
}
</style>
