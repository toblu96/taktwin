<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { LibraryService, Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { ExerciseInfo } from '../../bindings/github.com/toblu96/taktwin/internal/simapp/models'
import { grouped } from '../collections'
import { newEmptyProject, newProject } from '../sim'
import UiDialog from '../ui/UiDialog.vue'
import UiSegmented from '../ui/UiSegmented.vue'
import IconCube from '~icons/fluent/cube-16-regular'
import IconTask from '~icons/fluent/clipboard-task-20-regular'
import IconWrench from '~icons/fluent/wrench-20-regular'
import IconCheck from '~icons/fluent/checkmark-circle-16-filled'

// Makes a project from a station or an exercise of the collections, named after it unless it is
// given a name, and opens it; or one with an empty station of its own, opened to be built on.
const props = defineProps<{ start?: 'station' | 'exercise' | 'empty' }>()
const open = defineModel<boolean>('open', { required: true })

type Choice = { id: string; name: string; description?: string; collection?: string; on?: string }

const from = ref<'station' | 'exercise' | 'empty'>(props.start ?? 'station')
const stations = ref<Choice[]>([])
const exercises = ref<Choice[]>([])
// One empty station for each collection that has something to stand on, by its id.
const empties = ref<Choice[]>([])
const chosen = ref({ station: '', exercise: '', empty: '' })
const name = ref('')
const busy = ref(false)
const error = ref('')
const nameInput = ref<HTMLInputElement | null>(null)

async function load() {
  const [entries, all] = await Promise.all([LibraryService.Entries(), Service.AllExercises()])
  stations.value = (entries ?? []).filter((e) => e.level === 'station' && !e.error)
  const stands = (entries ?? []).filter((e) => e.role === 'station' && !e.error)
  empties.value = [...new Set(stands.map((e) => e.collection ?? ''))].map((c) => ({
    id: c,
    name: 'Empty station',
    collection: c,
    description: `Build your own, starting on ${stands.find((e) => (e.collection ?? '') === c)?.name ?? 'a stand'}.`,
  }))
  exercises.value = (all ?? []).map((e: ExerciseInfo) => ({ id: e.id, name: e.name, description: e.description, collection: e.collection, on: e.stationName }))
  chosen.value.station ||= stations.value[0]?.id ?? ''
  chosen.value.exercise ||= exercises.value[0]?.id ?? ''
  chosen.value.empty ||= empties.value[0]?.id ?? ''
}
onMounted(load)
watch(open, async (o) => {
  if (!o) return
  from.value = props.start ?? from.value
  error.value = ''
  name.value = ''
  await load()
  await nextTick()
  nameInput.value?.focus()
})

const choices = computed(() => ({ station: stations.value, exercise: exercises.value, empty: empties.value })[from.value])
const current = computed(() => choices.value.find((c) => c.id === chosen.value[from.value]))
const kinds = computed(() => [
  { value: 'station', label: 'Station', icon: IconCube },
  { value: 'exercise', label: 'Exercise', icon: IconTask, disabled: !exercises.value.length },
  { value: 'empty', label: 'Empty', icon: IconWrench, disabled: !empties.value.length },
])

async function create() {
  if (!current.value || busy.value) return
  busy.value = true
  error.value = ''
  try {
    if (from.value === 'empty') await newEmptyProject(current.value.id, name.value.trim())
    else await newProject(from.value === 'station' ? { station: current.value.id, exercise: '' } : { station: '', exercise: current.value.id }, name.value.trim())
    open.value = false
  } catch (e) {
    error.value = (e as Error).message ?? String(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <UiDialog v-model:open="open" title="New project" description="Start from a station, an exercise, or an empty station to build.">
    <div class="flex flex-col gap-4">
      <UiSegmented v-model="from" label="Start from" :options="kinds" class="self-start" />

      <div class="flex max-h-80 flex-col gap-4 overflow-auto">
        <section v-for="g in grouped(choices)" :key="g.id">
          <h3 class="t-label m-0 mb-2">{{ g.name }}</h3>
          <div class="grid grid-cols-[repeat(auto-fill,minmax(200px,1fr))] gap-2">
            <button
              v-for="c in g.items"
              :key="c.id"
              class="relative flex cursor-default flex-col items-start gap-0.5 rounded-card border bg-s1 px-3 py-2.5 text-left text-fg hover:bg-s3"
              :class="chosen[from] === c.id ? 'border-focus bg-select shadow-[inset_0_0_0_1px_var(--t-focus)]' : 'border-line'"
              :aria-pressed="chosen[from] === c.id"
              @click="chosen[from] = c.id"
              @dblclick="create"
            >
              <IconCheck v-if="chosen[from] === c.id" class="absolute top-2 right-2 size-4 text-focus" />
              <strong class="pr-5 font-semibold">{{ c.name }}</strong>
              <small v-if="c.on" class="text-xs text-muted">On {{ c.on }}</small>
              <span v-if="c.description" class="line-clamp-2 text-xs leading-snug text-muted">{{ c.description }}</span>
            </button>
          </div>
        </section>
        <p v-if="!choices.length" class="m-0 text-muted">Your collections have no {{ from === 'exercise' ? 'exercises' : 'stations' }}.</p>
      </div>

      <label class="flex flex-col gap-1">
        <span class="t-label">Name</span>
        <input
          ref="nameInput"
          v-model="name"
          class="t-input"
          type="text"
          maxlength="80"
          :placeholder="from === 'empty' ? 'New station' : (current?.name ?? '')"
          @keydown.enter="create"
        />
      </label>
      <p v-if="error" class="m-0 text-danger" role="alert">{{ error }}</p>
    </div>
    <template #footer>
      <button class="t-btn" @click="open = false">Cancel</button>
      <button class="t-btn t-btn-primary" :disabled="busy || !current" @click="create">Create</button>
    </template>
  </UiDialog>
</template>
