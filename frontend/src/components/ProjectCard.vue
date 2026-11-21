<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref } from 'vue'
import type { ProjectInfo } from '../../bindings/github.com/toblu96/taktwin/internal/simapp/models'

// A project on the projects page: a picture of its station, its name, what it is made from,
// when it last changed and how its last check went. Clicking it opens it; its menu has the
// rest.
const props = defineProps<{ project: ProjectInfo; busy?: boolean }>()
const emit = defineEmits<{
  open: []
  rename: [to: string]
  duplicate: []
  reveal: []
  export: []
  remove: []
}>()

const p = computed(() => props.project)

const menu = ref(false)
function closeMenu() {
  menu.value = false
  window.removeEventListener('click', closeMenu)
}
function toggleMenu() {
  if (menu.value) return closeMenu()
  menu.value = true
  // A click anywhere else closes it; this one is still on its way up.
  setTimeout(() => window.addEventListener('click', closeMenu))
}
onUnmounted(closeMenu)

function act(what: () => void) {
  closeMenu()
  what()
}

const renaming = ref(false)
const newName = ref('')
const input = ref<HTMLInputElement | null>(null)
async function startRename() {
  newName.value = p.value.name
  renaming.value = true
  await nextTick()
  input.value?.select()
}
function finishRename() {
  if (!renaming.value) return
  renaming.value = false
  const to = newName.value.trim()
  if (to && to !== p.value.name) emit('rename', to)
}

const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })
const units: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 31536000],
  ['month', 2592000],
  ['week', 604800],
  ['day', 86400],
  ['hour', 3600],
  ['minute', 60],
]
function ago(iso?: string) {
  if (!iso) return ''
  const seconds = (Date.parse(iso) - Date.now()) / 1000
  for (const [unit, size] of units) if (Math.abs(seconds) >= size) return rtf.format(Math.round(seconds / size), unit)
  return 'just now'
}
const exact = (iso?: string) => (iso ? new Date(iso).toLocaleString() : '')
</script>

<template>
  <article class="card" :class="{ broken: p.problem, menu }">
    <button class="open" :disabled="busy || !!p.problem" :title="p.problem || `Open ${p.name}`" @click="emit('open')">
      <span class="picture">
        <img v-if="p.thumbnail" :src="p.thumbnail" alt="" />
        <img v-else class="placeholder" src="/logo.svg" alt="" />
        <span v-if="p.exercise" class="kind">Exercise</span>
      </span>
    </button>
    <div class="body">
      <div class="title">
        <input
          v-if="renaming"
          ref="input"
          v-model="newName"
          class="rename"
          type="text"
          maxlength="80"
          aria-label="New name"
          @keydown.enter="finishRename"
          @keydown.esc="renaming = false"
          @blur="finishRename"
        />
        <button v-else class="name" :disabled="busy || !!p.problem" @click="emit('open')">{{ p.name }}</button>
        <button class="more" :aria-expanded="menu" aria-haspopup="menu" :aria-label="`More for ${p.name}`" @click.stop="toggleMenu">⋯</button>
      </div>
      <p class="what">{{ p.exerciseName ? `${p.exerciseName} · ${p.stationName}` : p.stationName }}</p>
      <p v-if="p.problem" class="problem">{{ p.problem }}</p>
      <p v-else class="meta">
        <span :title="exact(p.modified)">{{ ago(p.modified) }}</span>
        <span v-if="p.lastCheck" class="check" :class="p.lastCheck.passed ? 'passed' : 'failed'" :title="`Checked ${exact(p.lastCheck.at)}`">
          {{ p.lastCheck.passed ? '✓ Check passed' : '✗ Check failed' }}
        </span>
      </p>
    </div>
    <div v-if="menu" class="popup" role="menu" @click.stop>
      <button role="menuitem" @click="act(startRename)">Rename</button>
      <button role="menuitem" @click="act(() => emit('duplicate'))">Duplicate</button>
      <button role="menuitem" @click="act(() => emit('reveal'))">Show in folder</button>
      <button role="menuitem" @click="act(() => emit('export'))">Export…</button>
      <hr />
      <button role="menuitem" class="danger" @click="act(() => emit('remove'))">Delete…</button>
    </div>
  </article>
</template>

<style scoped>
.card {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 6px;
  display: flex;
  flex-direction: column;
  min-width: 0;
  position: relative;
  transition: border-color 0.12s;
}

.card:hover:not(.broken),
.card.menu {
  border-color: var(--text-dim);
}

.open {
  background: none;
  border: 0;
  border-radius: 6px 6px 0 0;
  padding: 0;
}

.picture {
  align-items: center;
  aspect-ratio: 16 / 10;
  background: #c9d6d6;
  border-bottom: 1px solid var(--line);
  border-radius: 5px 5px 0 0;
  display: flex;
  justify-content: center;
  overflow: hidden;
  position: relative;
}

.picture img {
  display: block;
  height: 100%;
  object-fit: cover;
  width: 100%;
}

.picture .placeholder {
  height: 56px;
  opacity: 0.5;
  width: 56px;
}

.kind {
  background: var(--accent);
  border-radius: 3px;
  color: var(--bg);
  font-size: 11px;
  font-weight: 700;
  left: 8px;
  padding: 1px 6px;
  position: absolute;
  top: 8px;
}

.broken .picture {
  filter: grayscale(1);
  opacity: 0.5;
}

.body {
  display: grid;
  gap: 3px;
  padding: 10px 12px 12px;
}

.title {
  align-items: center;
  display: flex;
  gap: 6px;
  min-width: 0;
}

.name {
  background: none;
  border: 0;
  flex: 1;
  font-size: 14px;
  font-weight: 600;
  min-width: 0;
  overflow: hidden;
  padding: 0;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.name:disabled {
  opacity: 1;
}

.rename {
  background: var(--bg);
  border: 1px solid var(--accent);
  color: var(--text);
  flex: 1;
  font-size: 14px;
  min-width: 0;
  padding: 1px 4px;
}

.more {
  background: none;
  border: 1px solid transparent;
  color: var(--text-dim);
  font-size: 16px;
  line-height: 1;
  padding: 1px 7px 5px;
}

.more:hover,
.menu .more {
  background: var(--panel-raised);
  border-color: var(--line);
  color: var(--text);
}

p {
  margin: 0;
}

.what {
  color: var(--text-dim);
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.meta {
  align-items: center;
  color: var(--text-dim);
  display: flex;
  font-size: 12px;
  gap: 8px;
  justify-content: space-between;
}

.check {
  font-weight: 600;
}

.check.passed {
  color: var(--signal);
}

.check.failed {
  color: var(--danger);
}

.problem {
  color: var(--danger);
  font-size: 12px;
}

.popup {
  background: var(--panel-raised);
  border: 1px solid var(--line);
  border-radius: 4px;
  box-shadow: 0 8px 24px rgb(0 0 0 / 40%);
  display: flex;
  flex-direction: column;
  min-width: 160px;
  padding: 4px;
  position: absolute;
  right: 8px;
  top: calc(100% - 44px);
  z-index: 10;
}

.popup button {
  background: none;
  border: 0;
  padding: 5px 10px;
  text-align: left;
}

.popup button:hover {
  background: var(--bg);
}

.popup .danger {
  color: var(--danger);
}

hr {
  border: 0;
  border-top: 1px solid var(--line);
  margin: 4px 0;
  width: 100%;
}
</style>
